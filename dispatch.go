package rls

import (
	"math/bits"
	"regexp"
	"sync"
	"unsafe"

	"github.com/autobrr/rls/taginfo"
)

// A lexGate is what a lexer needs at a position to have any chance of a
// match. It restates the tests the lexer itself begins with, so that the
// parser can make them once for all its lexers: a lexer whose gate fails at a
// position returns false there without having touched the tags, and so is not
// called. A gate may let through more than the lexer does, never less.
type lexGate struct {
	// always is set for a lexer with no bound, which is called everywhere.
	always bool
	// runs are the runs of letters and digits, in upper case, that let the
	// lexer through when the position begins with one of them.
	runs map[string]struct{}
	// fold lets the lexer through when a rune outside ASCII that a letter
	// folds to follows the run, see foldRune.
	fold bool
	// noRun lets the lexer through when the position begins with such a rune.
	noRun bool
	// lead are the first bytes that let the lexer through whatever the run.
	lead [256]bool
}

// prefilterGate is the gate of a lexer that returns false when none of pfs
// lets the position through.
func prefilterGate(pfs ...*prefilter) *lexGate {
	g := &lexGate{runs: make(map[string]struct{}), noRun: true}
	for _, pf := range pfs {
		if pf == nil {
			return &lexGate{always: true}
		}
		for k := range pf.runs {
			g.runs[k] = struct{}{}
		}
		g.fold = g.fold || pf.fold
		for c := range pf.nonRun {
			g.lead[c] = g.lead[c] || pf.nonRun[c]
		}
		if pf.hasLead {
			for c := range pf.lead {
				g.lead[c] = g.lead[c] || pf.lead[c]
			}
		}
	}
	return g
}

// leadGate is the gate of a lexer that returns false when the position does
// not begin with one of lead. ok is whether the set is bounded.
func leadGate(lead [256]bool, ok bool) *lexGate {
	if !ok {
		return &lexGate{always: true}
	}
	return &lexGate{lead: lead}
}

// bytesGate is the gate of a lexer that returns false when the position does
// not begin with one of the bytes of set.
func bytesGate(set string) *lexGate {
	g := &lexGate{}
	for i := range len(set) {
		g.lead[set[i]] = true
	}
	return g
}

// gateOffers holds, for each parser that is being built, the gates its lexers
// have offered it. A parser is known to its lexers by the delimiter regexp it
// hands them, which it makes for itself.
var gateOffers sync.Map // *regexp.Regexp -> *gateOffer

// gateOffer are the gates offered to one parser, by the lex func each is for.
// A lex func that is in it is one of this package's, with or without a gate.
type gateOffer struct {
	mu    sync.Mutex
	gates map[unsafe.Pointer]*lexGate
}

// funcID tells one lex func from another: a func value is a pointer to its
// closure, and two closures are never at one address.
func funcID(f LexFunc) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&f))
}

// withGate returns lexer with an Init that offers gate to the parser that is
// being built, after the Init the lexer has. The gate is read after that
// Init, since it is made of what the Init builds. A lexer that runs once has
// no use for a gate and offers none, which still tells the parser that the
// lexer is one of this package's.
//
// The offer is for the lex func of the lexer and no other. So a caller that
// puts a lex func of its own in the place of this one, or wraps it, has a
// lexer without a gate, which is called everywhere, as is any lexer that is
// not made here.
func withGate(lexer TagLexer, gate func() *lexGate) TagLexer {
	init, lex := lexer.Init, lexer.Lex
	lexer.Init = func(infos map[string][]*taginfo.Taginfo, delim *regexp.Regexp, short map[string]bool) {
		if init != nil {
			init(infos, delim, short)
		}
		if v, ok := gateOffers.Load(delim); ok {
			var g *lexGate
			if gate != nil {
				g = gate()
			}
			o := v.(*gateOffer)
			o.mu.Lock()
			o.gates[funcID(lex)] = g
			o.mu.Unlock()
		}
	}
	return lexer
}

// noGates makes a parser go without what only saves it work, for a test to
// compare a parser built that way with one that has it all: every lexer is
// tried at every position, no automaton stands in front of a regexp, and the
// lexers that run once search the whole of what they are given.
var noGates bool

// maxDispatch is how many lexers a dispatch can tell apart. The lexers past
// them are tried at every position.
const maxDispatch = 64

// A dispatch gives, for a position, the lexers that can match there as a set
// of bits in lexer order. It looks at the position once: the first byte, the
// run of letters and digits it begins, and the rune after the run.
type dispatch struct {
	runs  map[string]uint64
	max   int
	lead  [256]uint64
	fold  uint64
	noRun uint64
}

// newDispatch builds the dispatch for the gates of the lexers, in lexer
// order. A nil gate is a lexer that is called everywhere.
func newDispatch(gates []*lexGate) *dispatch {
	d := &dispatch{runs: make(map[string]uint64)}
	for a, g := range gates[:min(len(gates), maxDispatch)] {
		bit := uint64(1) << a
		if g == nil || g.always || noGates {
			d.noRun |= bit
			for c := range d.lead {
				d.lead[c] |= bit
			}
			continue
		}
		for c := range g.lead {
			if g.lead[c] {
				d.lead[c] |= bit
			}
		}
		if g.fold {
			d.fold |= bit
		}
		if g.noRun {
			d.noRun |= bit
		}
		for k := range g.runs {
			d.runs[k] |= bit
			d.max = max(d.max, len(k))
		}
	}
	return d
}

// mask returns the lexers that can match at i, which is before n. The
// position is the first byte of a token, which is no delimiter, so it is the
// same byte in the name and in its working copy, and the run it begins is the
// same in both.
func (d *dispatch) mask(buf []byte, i, n int) uint64 {
	m := d.lead[buf[i]]
	// the run at i, upper cased, as the prefilters key it
	var key [64]byte
	j := 0
	for i+j < n && j < len(key) && runByte(buf[i+j]) {
		c := buf[i+j]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		key[j] = c
		j++
	}
	if j == 0 {
		// a byte that is no letter or digit begins what the lead bytes say
		// and, when it is a rune a letter folds to, what a run can begin
		if foldRune(buf, i, n) {
			m |= d.noRun
		}
		return m
	}
	if foldRune(buf, i+j, n) {
		m |= d.fold
	}
	if j <= d.max {
		m |= d.runs[string(key[:j])]
	}
	return m
}

// nextLexer returns the first lexer from a on that is in m. The lexers past
// the bits of a mask are all in it.
func nextLexer(m uint64, a int) int {
	if a >= maxDispatch {
		return a
	}
	// 64 when no bit is left, which is past the last of them
	return min(a+bits.TrailingZeros64(m>>a), maxDispatch)
}
