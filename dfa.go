package rls

import (
	"regexp/syntax"
)

// A prefixDFA answers, for a set of regexps anchored at the start, which of
// them can match at a position, so that a regexp is only run where it can.
//
// It is the automaton of the regexps with every zero width assertion taken to
// hold ('$', '\b'), built ahead over the ASCII bytes. Running it from a
// position gives the regexps for which some prefix of the input is a match
// when the assertions are ignored. A regexp that matches at the position also
// matches there without its assertions, so one the automaton leaves out cannot
// match and need not run; one it reports may still fail on an assertion, which
// the regexp itself then decides.
//
// A byte outside ASCII ends the run with every regexp that is still alive
// reported: the automaton does not decode runes, and a case insensitive
// letter matches a rune or two outside ASCII.
type prefixDFA struct {
	// next is the state after a byte, or 0 when no regexp is left.
	next [][128]uint16
	// match are the regexps that have a match on reaching the state, and
	// alive those with a thread in it.
	match []uint32
	alive []uint32
	start uint16
}

// maxDFAStates bounds the automaton. A set of regexps that needs more gets
// none.
const maxDFAStates = 3000

// maxDFARegexps is how many regexps an automaton can tell apart.
const maxDFARegexps = 32

// newPrefixDFA builds the automaton for res, or returns nil when there is
// none to build: when one of the regexps does not compile or has a match
// that does not begin at the start of the text, when there are more than
// maxDFARegexps, or when the automaton grows past maxDFAStates.
func newPrefixDFA(res ...string) *prefixDFA {
	if noGates || len(res) == 0 || len(res) > maxDFARegexps {
		return nil
	}
	progs := make([]*syntax.Prog, len(res))
	for l, s := range res {
		r, err := syntax.Parse(s, syntax.Perl)
		if err != nil {
			return nil
		}
		if progs[l], err = syntax.Compile(r.Simplify()); err != nil {
			return nil
		}
		// a regexp that can match further on is found there by a search, of
		// which the automaton knows nothing
		if progs[l].StartCond()&syntax.EmptyBeginText == 0 {
			return nil
		}
	}
	type thread struct {
		l  uint8
		pc uint32
	}
	// closure expands threads through everything that consumes no input, and
	// returns those that consume with the regexps that reached a match
	marks, gen := make([][]uint32, len(progs)), uint32(0)
	for l, p := range progs {
		marks[l] = make([]uint32, len(p.Inst))
	}
	var out []thread
	var matched uint32
	var add func(thread)
	add = func(t thread) {
		if marks[t.l][t.pc] == gen {
			return
		}
		marks[t.l][t.pc] = gen
		switch inst := &progs[t.l].Inst[t.pc]; inst.Op {
		case syntax.InstAlt, syntax.InstAltMatch:
			add(thread{t.l, inst.Out})
			add(thread{t.l, inst.Arg})
		case syntax.InstCapture, syntax.InstNop, syntax.InstEmptyWidth:
			add(thread{t.l, inst.Out})
		case syntax.InstMatch:
			matched |= 1 << t.l
		case syntax.InstFail:
		default:
			out = append(out, t)
		}
	}
	closure := func(ts []thread) ([]thread, uint32) {
		gen++
		out, matched = nil, 0
		for _, t := range ts {
			add(t)
		}
		return out, matched
	}
	d := &prefixDFA{}
	ids := make(map[string]uint16)
	var sets [][]thread
	// state returns the id of the state for a closed set of threads
	state := func(ts []thread, m uint32) (uint16, bool) {
		key := make([]byte, 0, 4+len(ts)*5)
		key = append(key, byte(m), byte(m>>8), byte(m>>16), byte(m>>24))
		var alive uint32
		for _, t := range ts {
			key = append(key, t.l, byte(t.pc), byte(t.pc>>8), byte(t.pc>>16), byte(t.pc>>24))
			alive |= 1 << t.l
		}
		if id, ok := ids[string(key)]; ok {
			return id, true
		}
		if len(d.next) >= maxDFAStates {
			return 0, false
		}
		id := uint16(len(d.next))
		ids[string(key)] = id
		d.next, d.match, d.alive, sets = append(d.next, [128]uint16{}), append(d.match, m), append(d.alive, alive), append(sets, ts)
		return id, true
	}
	// state 0 is the dead state
	d.next, d.match, d.alive, sets = append(d.next, [128]uint16{}), append(d.match, 0), append(d.alive, 0), append(sets, nil)
	first := make([]thread, len(progs))
	for l, p := range progs {
		first[l] = thread{uint8(l), uint32(p.Start)}
	}
	ts, m := closure(first)
	var ok bool
	if d.start, ok = state(ts, m); !ok {
		return nil
	}
	var step []thread
	for s := 1; s < len(sets); s++ {
		for c := range 128 {
			step = step[:0]
			for _, t := range sets[s] {
				if inst := &progs[t.l].Inst[t.pc]; inst.MatchRune(rune(c)) {
					step = append(step, thread{t.l, inst.Out})
				}
			}
			if len(step) == 0 {
				continue
			}
			ts, m := closure(step)
			if len(ts) == 0 && m == 0 {
				continue
			}
			id, ok := state(ts, m)
			if !ok {
				return nil
			}
			d.next[s][c] = id
		}
	}
	return d
}

// mask returns the regexps that can match b[i:n], as bits in their order.
func (d *prefixDFA) mask(b []byte, i, n int) uint32 {
	s := d.start
	m := d.match[s]
	for ; i < n; i++ {
		c := b[i]
		if c >= 0x80 {
			return m | d.alive[s]
		}
		if s = d.next[s][c]; s == 0 {
			return m
		}
		// nothing more to learn once every regexp that is alive has matched
		if m |= d.match[s]; d.alive[s]&^m == 0 {
			return m
		}
	}
	return m
}
