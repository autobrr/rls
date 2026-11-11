package rls

import (
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/autobrr/rls/taginfo"
)

// A matcher finds the row of a tag type that matches at a position without
// running the type's combined alternation. Most rows are a short list of
// spellings, so the row is found by the word at the position and confirmed by
// comparing bytes.
//
// It gives what the alternation gives. The alternation takes the first row,
// in row order, one of whose spellings matches and ends where the lexer's
// terminator allows, trying the spellings of a row in the order a
// backtracking matcher would. So the rows that can match the word at the
// position are tried in row order, each with its spellings in that order. A
// row that is not a plain list of spellings, or that captures a part of its
// match, is confirmed with a regexp of its own.
//
// That rests on three things, which newMatcher checks and without which a
// type keeps its alternation alone: each row means the same on its own as it
// does between its neighbours, no row matches the empty string, and the
// spellings expandRE gives for a row begin with every run of letters and
// digits a match of the row can begin with.
//
// The bytes are compared as ASCII. A case insensitive regexp also matches
// 'k' and 's' with the runes U+212A and U+017F, so where the matcher would
// have to judge a byte outside ASCII -- right after the run, inside a
// spelling it compares, or as the terminator of a source that does not end in
// a word character -- it does not decide, and the lexer runs the alternation
// as before.
type matcher struct {
	rows []matchRow
	// runs gives the rows that have a spelling whose first run of letters and
	// digits is the key, in row order. The key is upper case. '_' is no part
	// of a run, as in the prefilter.
	runs map[string][]int32
	// scan are the rows expandRE gives no spellings for, in row order. Each
	// is tried at every position its gate lets through.
	scan []int32
	// classes are the sets of bytes a position of a spelling can hold.
	classes []byteClass
	// caps is how many capture groups the rows have between them, which the
	// tag of any row carries as values.
	caps int
	// source is whether the terminator is that of the source lexer.
	source bool
}

// matchRow is one row of a matcher.
type matchRow struct {
	// spellings are the spellings of the row in the order a backtracking
	// matcher tries them, each a string of indexes into the classes of the
	// matcher. It is nil for a row that is confirmed by re.
	spellings []string
	// re matches the row alone, anchored and terminated as the alternation.
	re func() *regexp.Regexp
	// offset is where the captures of the row start among those of the type.
	offset int
	// gate is what a row in scan needs at a position: one of its first bytes,
	// and the literal every match of it holds.
	gate patterned
}

// byteClass is a set of ASCII bytes.
type byteClass [2]uint64

func (c *byteClass) add(r rune) { c[r/64] |= 1 << (r % 64) }

func (c *byteClass) has(b byte) bool { return c[b/64]&(1<<(b%64)) != 0 }

// maxSpellings is how many spellings a row may have and still be compared
// byte by byte.
const maxSpellings = 64

// noMatcher makes every lexer run its alternation, for a test to compare a
// parser built that way with one that has the matchers.
var noMatcher bool

// bareFlags matches a group that sets flags for the rest of its regexp, ie
// '(?-i)'. In the alternation it also changes the rows after its own.
var bareFlags = regexp.MustCompile(`\(\?[a-zA-Z\-]+\)`)

// newMatcher builds the matcher for infos. ci is whether the rows match
// without regard to case, and source whether the terminator is the source
// lexer's. It returns nil, which leaves the lexer with its alternation, when a
// row is not what match takes rows to be: one that does not parse on its own
// or sets flags for the rows after it, one that matches the empty string, or
// one whose first bytes cannot be bounded.
func newMatcher(infos []*taginfo.Taginfo, ci, source bool) *matcher {
	if noMatcher {
		return nil
	}
	m := &matcher{
		rows:   make([]matchRow, len(infos)),
		runs:   make(map[string][]int32),
		source: source,
	}
	classes := make(map[byteClass]byte)
	for i, info := range infos {
		// the alternation joins the rows with no group around each, so a row
		// has to be a whole regexp by itself ('a)|(b' is not) and keep its
		// flags to itself
		if _, err := syntax.Parse(info.RE(), syntax.Perl); err != nil || bareFlags.MatchString(info.RE()) {
			return nil
		}
		pattern := `(` + info.RE() + `)`
		if ci {
			pattern = `(?i)` + pattern
		}
		tree, err := syntax.Parse(pattern, syntax.Perl)
		if err != nil || nullable(tree) {
			return nil
		}
		row := &m.rows[i]
		row.offset = m.caps
		m.caps += tree.MaxCap() - 1
		end := `\b`
		if source {
			end = `(?:\b|[\-\._ ]` + plusEnd + `)`
		}
		row.re = sync.OnceValue(func() *regexp.Regexp {
			return regexp.MustCompile(`^` + pattern + end)
		})
		// what the row can be written as, for the runs it can begin with
		spellings, err := expandRE(info.RE(), ci)
		if err != nil {
			if !firstBytes(info.RE(), &row.gate.lead) {
				return nil
			}
			// a rune outside ASCII in the row may fold to a letter, which
			// the first bytes do not show ('ſtuff' matches 'stuff'): such a
			// row is tried everywhere
			if strings.IndexFunc(info.RE(), func(r rune) bool { return r >= 0x80 }) != -1 {
				for c := range row.gate.lead {
					row.gate.lead[c] = true
				}
				m.scan = append(m.scan, int32(i))
				continue
			}
			// either case, as the prefilter has it: a row may fold a part of
			// itself ('(?i:multi)')
			for c := 'a'; c <= 'z'; c++ {
				if row.gate.lead[c] || row.gate.lead[c-32] {
					row.gate.lead[c], row.gate.lead[c-32] = true, true
				}
			}
			row.gate.lit, row.gate.fold = requiredLiteral(info.RE(), ci)
			m.scan = append(m.scan, int32(i))
			continue
		}
		var keyed bool
		for _, s := range spellings {
			run := firstRun(s)
			if run == "" {
				// begins with a byte that is no part of a run, where match
				// does not decide
				continue
			}
			if v := m.runs[run]; len(v) == 0 || v[len(v)-1] != int32(i) {
				m.runs[run] = append(v, int32(i))
			}
			keyed = true
		}
		if !keyed {
			continue
		}
		// a row with no group of its own, as a list of spellings
		if tree.MaxCap() == 1 {
			if v, ok := m.spell(tree, classes); ok && len(v) != 0 && !slices.Contains(v, "") {
				row.spellings = v
			}
		}
	}
	return m
}

// nullable reports whether re can match the empty string.
func nullable(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpEmptyMatch, syntax.OpStar, syntax.OpQuest,
		syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return true
	case syntax.OpLiteral:
		return len(re.Rune) == 0
	case syntax.OpRepeat:
		return re.Min == 0 || nullable(re.Sub[0])
	case syntax.OpCapture, syntax.OpPlus:
		return nullable(re.Sub[0])
	case syntax.OpConcat:
		return !slices.ContainsFunc(re.Sub, func(sub *syntax.Regexp) bool { return !nullable(sub) })
	case syntax.OpAlternate:
		return slices.ContainsFunc(re.Sub, nullable)
	}
	return false
}

// spell returns the strings re matches as strings of class indexes, in the
// order a backtracking matcher tries them. It returns false when re is not a
// list of at most maxSpellings ASCII strings, when a letter or class of it
// folds to a rune outside ASCII, and when the type runs out of classes.
func (m *matcher) spell(re *syntax.Regexp, classes map[byteClass]byte) ([]string, bool) {
	class := func(c byteClass) (byte, bool) {
		if i, ok := classes[c]; ok {
			return i, true
		}
		if len(m.classes) == 256 {
			return 0, false
		}
		i := byte(len(m.classes))
		m.classes, classes[c] = append(m.classes, c), i
		return i, true
	}
	switch re.Op {
	case syntax.OpEmptyMatch:
		return []string{""}, true
	case syntax.OpCapture:
		return m.spell(re.Sub[0], classes)
	case syntax.OpLiteral:
		b := make([]byte, len(re.Rune))
		for i, r := range re.Rune {
			if r >= 0x80 {
				return nil, false
			}
			var c byteClass
			c.add(r)
			if re.Flags&syntax.FoldCase != 0 {
				for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
					if f < 0x80 {
						c.add(f)
					}
				}
			}
			var ok bool
			if b[i], ok = class(c); !ok {
				return nil, false
			}
		}
		return []string{string(b)}, true
	case syntax.OpCharClass:
		var c byteClass
		for i := 0; i < len(re.Rune); i += 2 {
			if re.Rune[i+1] >= 0x80 {
				return nil, false
			}
			for r := re.Rune[i]; r <= re.Rune[i+1]; r++ {
				c.add(r)
			}
		}
		i, ok := class(c)
		return []string{string([]byte{i})}, ok
	case syntax.OpConcat:
		v := []string{""}
		for _, sub := range re.Sub {
			w, ok := m.spell(sub, classes)
			if !ok || len(v)*len(w) > maxSpellings {
				return nil, false
			}
			next := make([]string, 0, len(v)*len(w))
			for _, a := range v {
				for _, b := range w {
					next = append(next, a+b)
				}
			}
			v = next
		}
		return v, true
	case syntax.OpAlternate:
		var v []string
		for _, sub := range re.Sub {
			w, ok := m.spell(sub, classes)
			if !ok || len(v)+len(w) > maxSpellings {
				return nil, false
			}
			v = append(v, w...)
		}
		return v, true
	case syntax.OpQuest:
		v, ok := m.spell(re.Sub[0], classes)
		if !ok || len(v) >= maxSpellings {
			return nil, false
		}
		if re.Flags&syntax.NonGreedy != 0 {
			return append([]string{""}, v...), true
		}
		return append(v, ""), true
	}
	return nil, false
}

// wordByte reports whether c is a word character to the regexp \b.
func wordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// match finds the row that matches b at i, where b ends at n. It returns the
// index of the row, or -1 when none matches, the end of the value and the end
// of the match. The match ends past the value when the source lexer's
// terminator took a byte: a delimiter for a row of spellings, and whatever the
// terminator took for a row confirmed by its regexp, which the lexer hands
// back when it is no delimiter. For a row confirmed by its regexp match
// returns the submatches too. decided is false when the text has to go to the
// alternation.
func (m *matcher) match(b []byte, i, n int) (row, value, end int, sub [][]byte, decided bool) {
	if m == nil {
		return 0, 0, 0, nil, false
	}
	// the run at i, upper cased
	var key [64]byte
	j := 0
	for i+j < n && j < len(key) && runByte(b[i+j]) {
		c := b[i+j]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		key[j] = c
		j++
	}
	if j == 0 || j == len(key) || i+j < n && b[i+j] >= 0x80 {
		return 0, 0, 0, nil, false
	}
	keyed, scan := m.runs[string(key[:j])], m.scan
	for len(keyed) != 0 || len(scan) != 0 {
		// the next row in row order
		var k int32
		if len(scan) == 0 || len(keyed) != 0 && keyed[0] < scan[0] {
			k, keyed = keyed[0], keyed[1:]
		} else {
			k, scan = scan[0], scan[1:]
			if g := &m.rows[k].gate; !g.lead[b[i]] || len(g.lit) != 0 && !hasLiteral(b[i:n], g.lit, g.fold) {
				continue
			}
		}
		r := &m.rows[k]
		if r.spellings == nil {
			if sub := r.re().FindSubmatch(b[i:n]); sub != nil {
				return int(k), i + len(sub[1]), i + len(sub[0]), sub, true
			}
			continue
		}
		for _, s := range r.spellings {
			if len(s) > n-i {
				continue
			}
			l := 0
			for ; l < len(s); l++ {
				c := b[i+l]
				if c >= 0x80 {
					return 0, 0, 0, nil, false
				}
				if !m.classes[s[l]].has(c) {
					break
				}
			}
			if l != len(s) {
				continue
			}
			// the terminator
			e := i + l
			prev := wordByte(b[e-1])
			switch {
			case e == n:
				if prev || m.source {
					return int(k), e, e, nil, true
				}
			case prev != wordByte(b[e]):
				return int(k), e, e, nil, true
			case !m.source:
			case prev:
				// between two word characters only '_' ends a source
				if b[e] == '_' {
					return int(k), e, e + 1, nil, true
				}
			case tagDelim(b[e]):
				return int(k), e, e + 1, nil, true
			case b[e] >= 0x80:
				// \W under (?i) does not take the runes that fold to a
				// letter
				return 0, 0, 0, nil, false
			default:
				// another byte that is no word character ends it. The
				// alternation takes that byte and the lexer hands it back
				return int(k), e, e, nil, true
			}
		}
	}
	return -1, 0, 0, nil, true
}

// values returns the values of the tag for a match of row: the text, the
// value and the captures of every row of the type, of which only those of
// row can be set. buf is room for them, and a type with more groups than it
// holds gets a slice of its own.
func (m *matcher) values(buf [][]byte, text, value []byte, row int, sub [][]byte) [][]byte {
	v := append(buf[:0], text, value)
	for range m.caps {
		v = append(v, nil)
	}
	if len(sub) > 2 {
		copy(v[2+m.rows[row].offset:], sub[2:])
	}
	return v
}
