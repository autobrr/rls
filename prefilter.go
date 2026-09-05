package rls

import (
	"errors"
	"strings"

	"github.com/autobrr/rls/taginfo"
)

// A prefilter answers, cheaply, whether a tag type could possibly match at a
// position, so the type's combined alternation only runs when it might.
//
// Roughly three in four tokens in a release name match no tag at all, and each
// one was previously tested against every type's alternation. Those are the
// calls this removes.
//
// Soundness. Both regexp lexers anchor the alternation so that a match ends on
// a word boundary or a delimiter, which means a match can never end part way
// through a run of word characters. So if a type matches at i, the first
// word-run of the matched text is exactly the word-run at i, and testing that
// run against the set of first runs the type can produce never rejects a
// position the alternation would have matched. A type whose rows cannot all be
// expanded gets no prefilter and is always run.
type prefilter struct {
	runs map[string]struct{}
	max  int
	// lead holds the possible first bytes of the rows that could not be
	// expanded to literals (mostly patterned rows such as ([123]\d{3})p, which
	// can only start with a digit). A position whose run is not in runs may
	// still be matched by one of those, so it is only rejected when its first
	// byte is not a possible lead either.
	lead    [256]bool
	hasLead bool
}

// runByte reports whether c continues a run for prefilter purposes.
//
// Note '_' is excluded even though Go's \b counts it as a word character: it is
// also a member of the [\-\._ ] class the source lexer accepts as a terminator,
// so a match may end just before one ('xvid_iso' matches the XViD codec). A run
// that swallowed '_' would key on 'XVIDISO' and wrongly reject the position.
func runByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// newPrefilter builds the prefilter for infos, reporting false when any row
// cannot be reduced to literal spellings.
func newPrefilter(infos []*taginfo.Taginfo, ci bool) (*prefilter, bool) {
	pf := &prefilter{runs: make(map[string]struct{})}
	for _, info := range infos {
		v, err := expandRE(info.RE(), ci)
		if err != nil {
			// the row cannot be reduced to literals. Fall back to bounding its
			// first byte, which is enough for the patterned rows; if even that
			// is unbounded the type gets no prefilter at all.
			var lead [256]bool
			if !firstBytes(info.RE(), &lead) {
				return nil, false
			}
			for c := 0; c < 256; c++ {
				if lead[c] {
					pf.lead[c], pf.hasLead = true, true
					// a lead byte is only consulted for runs, so fold case
					if c >= 'a' && c <= 'z' {
						pf.lead[c-32] = true
					} else if c >= 'A' && c <= 'Z' {
						pf.lead[c+32] = true
					}
				}
			}
			continue
		}
		for _, s := range v {
			run := firstRun(s)
			if run == "" {
				// an alternative that opens with a delimiter has no run to key
				// on; fall back to always running the type
				return nil, false
			}
			if len(run) > pf.max {
				pf.max = len(run)
			}
			pf.runs[run] = struct{}{}
		}
	}
	if len(pf.runs) == 0 {
		return nil, false
	}
	return pf, true
}

// firstRun returns the leading run of word characters in s, upper cased.
func firstRun(s string) string {
	i := 0
	for i < len(s) && runByte(s[i]) {
		i++
	}
	return strings.ToUpper(s[:i])
}

// maybe reports whether the type could match at i. A false result means the
// alternation cannot match and does not need to run.
func (pf *prefilter) maybe(buf []byte, i, n int) bool {
	if pf == nil {
		return true
	}
	// the run at i, upper cased into a stack buffer so the map lookup below
	// does not allocate
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
		// not on a word character: only reachable for alternations that begin
		// with a delimiter, which newPrefilter rejects, so let it run
		return true
	}
	if j <= pf.max {
		if _, ok := pf.runs[string(key[:j])]; ok {
			return true
		}
	}
	// no literal run matched; an unexpanded row may still match here
	return pf.hasLead && pf.lead[buf[i]]
}

// firstBytes fills lead with the bytes re can begin with, reporting false when
// that set is unbounded (an unescaped '.', a negated class, and so on). It is a
// deliberately shallow analysis: it only has to be an over-approximation.
func firstBytes(re string, lead *[256]bool) bool {
	f := &leader{src: re, lead: lead}
	return f.alt() && f.pos == len(f.src)
}

type leader struct {
	src  string
	pos  int
	lead *[256]bool
}

func (f *leader) set(c byte) { f.lead[c] = true }

func (f *leader) setRange(lo, hi byte) {
	for x := lo; x <= hi; x++ {
		f.lead[x] = true
	}
}

// alt walks each branch, taking the first bytes of every one.
func (f *leader) alt() bool {
	for {
		if !f.seq() {
			return false
		}
		if f.pos < len(f.src) && f.src[f.pos] == '|' {
			f.pos++
			continue
		}
		return true
	}
}

// seq contributes the first bytes of the leading atoms, continuing past an atom
// only while it is optional (and so may contribute nothing).
func (f *leader) seq() bool {
	for f.pos < len(f.src) {
		c := f.src[f.pos]
		if c == '|' || c == ')' {
			return true
		}
		start := f.pos
		// every atom up to and including the first mandatory one can begin the
		// match, because an optional atom may match nothing. In
		// '(?:(?:incl?|and)[\-\._ ])?key...' both {i,a} and {k} are leads.
		optional, ok := f.atom(true)
		if !ok {
			return false
		}
		if !optional {
			return f.skipBranch()
		}
		if f.pos == start {
			return false // no progress; bail rather than spin
		}
	}
	return true
}

// skipBranch consumes to the end of the current alternation branch.
func (f *leader) skipBranch() bool {
	depth := 0
	for f.pos < len(f.src) {
		switch f.src[f.pos] {
		case '\\':
			f.pos++
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return true
			}
			depth--
		case '|':
			if depth == 0 {
				return true
			}
		}
		f.pos++
	}
	return true
}

// atom records the first bytes of one atom, reporting whether it is optional.
func (f *leader) atom(contribute bool) (optional, ok bool) {
	switch c := f.src[f.pos]; c {
	case '(':
		f.pos++
		// inline flag group, eg (?i) -- zero width, contributes nothing
		if j := strings.IndexByte(f.src[f.pos:], ')'); j != -1 && j > 0 && f.src[f.pos] == '?' {
			if flags := f.src[f.pos : f.pos+j]; !strings.ContainsAny(flags, ":<=!P") {
				f.pos += j + 1
				return true, true
			}
		}
		for _, p := range []string{"?:", "?i:", "?-i:"} {
			if strings.HasPrefix(f.src[f.pos:], p) {
				f.pos += len(p)
				break
			}
		}
		// named capture, eg (?P<s>...)
		if strings.HasPrefix(f.src[f.pos:], "?P<") {
			j := strings.IndexByte(f.src[f.pos:], '>')
			if j == -1 {
				return false, false
			}
			f.pos += j + 1
		}
		if f.pos < len(f.src) && f.src[f.pos] == '?' {
			return false, false
		}
		sub := &leader{src: f.src, pos: f.pos, lead: f.lead}
		if contribute {
			if !sub.alt() {
				return false, false
			}
		}
		// walk to the matching ')'
		depth := 0
		for f.pos < len(f.src) {
			switch f.src[f.pos] {
			case '\\':
				f.pos++
			case '(':
				depth++
			case ')':
				if depth == 0 {
					f.pos++
					return f.quant(), true
				}
				depth--
			}
			f.pos++
		}
		return false, false
	case '[':
		f.pos++
		neg := f.pos < len(f.src) && f.src[f.pos] == '^'
		if neg {
			return false, false
		}
		for f.pos < len(f.src) && f.src[f.pos] != ']' {
			ch := f.src[f.pos]
			if ch == '\\' {
				f.pos++
				if f.pos >= len(f.src) {
					return false, false
				}
				ch = f.src[f.pos]
			}
			f.pos++
			if f.pos+1 < len(f.src) && f.src[f.pos] == '-' && f.src[f.pos+1] != ']' {
				f.pos++
				hi := f.src[f.pos]
				f.pos++
				if contribute {
					f.setRange(ch, hi)
				}
				continue
			}
			if contribute {
				f.set(ch)
			}
		}
		if f.pos >= len(f.src) {
			return false, false
		}
		f.pos++ // ]
		return f.quant(), true
	case '\\':
		f.pos++
		if f.pos >= len(f.src) {
			return false, false
		}
		e := f.src[f.pos]
		f.pos++
		if contribute {
			switch e {
			case 'd':
				f.setRange('0', '9')
			case 'w':
				f.setRange('0', '9')
				f.setRange('a', 'z')
				f.setRange('A', 'Z')
				f.set('_')
			case 'Q':
				j := strings.Index(f.src[f.pos:], `\E`)
				if j == -1 || j == 0 {
					return false, false
				}
				f.set(f.src[f.pos])
				f.pos += j + 2
				return f.quant(), true
			default:
				if !literalEscape(e) {
					return false, false
				}
				f.set(e)
			}
		}
		return f.quant(), true
	case '^', '$':
		// zero width: contributes nothing and does not end the scan
		f.pos++
		return true, true
	case '.':
		return false, false
	default:
		f.pos++
		if contribute {
			f.set(c)
		}
		return f.quant(), true
	}
}

// quant consumes a trailing quantifier, reporting whether it makes the atom
// optional.
func (f *leader) quant() bool {
	if f.pos >= len(f.src) {
		return false
	}
	switch f.src[f.pos] {
	case '?', '*':
		f.pos++
		return true
	case '+':
		f.pos++
		return false
	case '{':
		// {n}, {n,}, {n,m}: optional only when n is 0
		j := strings.IndexByte(f.src[f.pos:], '}')
		if j == -1 {
			return false
		}
		spec := f.src[f.pos+1 : f.pos+j]
		f.pos += j + 1
		return strings.HasPrefix(spec, "0")
	}
	return false
}

// leadSet returns the bytes an anchored regexp can begin with, folded to both
// cases, reporting false when the set is unbounded. Used to skip a lexer's
// regexps outright at positions they cannot match.
func leadSet(re string) ([256]bool, bool) {
	var lead [256]bool
	if !firstBytes(re, &lead) {
		return lead, false
	}
	n := 0
	for c := 0; c < 256; c++ {
		if !lead[c] {
			continue
		}
		n++
		if c >= 'a' && c <= 'z' {
			lead[c-32] = true
		} else if c >= 'A' && c <= 'Z' {
			lead[c+32] = true
		}
	}
	// a set that admits everything is not worth consulting
	return lead, n != 0 && n < 200
}

// -- taginfo regexp expansion ------------------------------------------------

var errUnsupported = errors.New("unsupported regexp construct")

const maxVariants = 4096

// expandRE expands a taginfo regexp into the concrete literal spellings it can
// match, so their leading runs can be collected. Delimiters are kept, unlike a
// squashed-key expansion, because they are what ends a run.
//
// Supported: literal runs, literal escapes, alternation, optional, character
// classes of literals and ranges, and the (?:...) (?i:...) (?-i:...) groups.
// Anything else (\d, \w, +, *, {n,m}, unescaped ., ^, $) is unsupported and
// costs the type its prefilter.
func expandRE(re string, ci bool) ([]string, error) {
	e := &expander{src: re}
	v, err := e.alt(ci)
	if err != nil {
		return nil, err
	}
	if e.pos != len(e.src) {
		return nil, errUnsupported
	}
	return v, nil
}

type expander struct {
	src string
	pos int
}

func (e *expander) eof() bool { return e.pos >= len(e.src) }

func (e *expander) peek() byte {
	if e.eof() {
		return 0
	}
	return e.src[e.pos]
}

// alt parses seq ('|' seq)*.
func (e *expander) alt(ci bool) ([]string, error) {
	var out []string
	for {
		v, err := e.seq(ci)
		if err != nil {
			return nil, err
		}
		out = append(out, v...)
		if len(out) > maxVariants {
			return nil, errUnsupported
		}
		if e.peek() == '|' {
			e.pos++
			continue
		}
		return out, nil
	}
}

// seq parses a concatenation of optionally quantified atoms.
func (e *expander) seq(ci bool) ([]string, error) {
	acc := []string{""}
	for !e.eof() {
		if c := e.peek(); c == '|' || c == ')' {
			break
		}
		v, err := e.atom(ci)
		if err != nil {
			return nil, err
		}
		switch e.peek() {
		case '?':
			e.pos++
			v = append(v, "")
		case '*', '+', '{':
			return nil, errUnsupported
		}
		if len(acc)*len(v) > maxVariants {
			return nil, errUnsupported
		}
		next := make([]string, 0, len(acc)*len(v))
		for _, a := range acc {
			for _, b := range v {
				next = append(next, a+b)
			}
		}
		acc = next
	}
	return acc, nil
}

// atom parses a single group, class, escape or literal character.
func (e *expander) atom(ci bool) ([]string, error) {
	switch c := e.src[e.pos]; c {
	case '(':
		return e.group(ci)
	case '[':
		return e.class()
	case '\\':
		return e.escape()
	case '.', '^', '$', '*', '+', '?', '{', '}', ']':
		return nil, errUnsupported
	default:
		e.pos++
		return []string{string(c)}, nil
	}
}

func (e *expander) group(ci bool) ([]string, error) {
	e.pos++ // (
	gci := ci
	switch {
	case strings.HasPrefix(e.src[e.pos:], "?:"):
		e.pos += 2
	case strings.HasPrefix(e.src[e.pos:], "?i:"):
		e.pos += 3
		gci = true
	case strings.HasPrefix(e.src[e.pos:], "?-i:"):
		e.pos += 4
		gci = false
	case e.peek() == '?':
		return nil, errUnsupported // ?P<..>, ?=, ?!, ...
	}
	v, err := e.alt(gci)
	if err != nil {
		return nil, err
	}
	if e.peek() != ')' {
		return nil, errUnsupported
	}
	e.pos++
	return v, nil
}

// class parses [ ... ], yielding one spelling per member.
func (e *expander) class() ([]string, error) {
	e.pos++ // [
	if e.peek() == '^' {
		return nil, errUnsupported
	}
	var members []byte
	for !e.eof() && e.peek() != ']' {
		c := e.src[e.pos]
		if c == '\\' {
			e.pos++
			if e.eof() {
				return nil, errUnsupported
			}
			if c = e.src[e.pos]; !literalEscape(c) {
				return nil, errUnsupported
			}
		}
		e.pos++
		// range
		if e.peek() == '-' && e.pos+1 < len(e.src) && e.src[e.pos+1] != ']' {
			e.pos++
			hi := e.src[e.pos]
			if hi == '\\' {
				return nil, errUnsupported
			}
			e.pos++
			if hi < c || int(hi)-int(c) > 64 {
				return nil, errUnsupported
			}
			for x := c; x <= hi; x++ {
				members = append(members, x)
			}
			continue
		}
		members = append(members, c)
	}
	if e.peek() != ']' {
		return nil, errUnsupported
	}
	e.pos++
	if len(members) == 0 {
		return nil, errUnsupported
	}
	out, seen := make([]string, 0, len(members)), make(map[byte]bool, len(members))
	for _, m := range members {
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, string(m))
	}
	return out, nil
}

func (e *expander) escape() ([]string, error) {
	e.pos++
	if e.eof() {
		return nil, errUnsupported
	}
	c := e.src[e.pos]
	// \Q ... \E quotes a literal run. taginfo.RE wraps every row that has no
	// regexp of its own this way, so it is the common case here.
	if c == 'Q' {
		e.pos++
		j := strings.Index(e.src[e.pos:], `\E`)
		if j == -1 {
			return nil, errUnsupported
		}
		lit := e.src[e.pos : e.pos+j]
		e.pos += j + 2
		return []string{lit}, nil
	}
	if !literalEscape(c) {
		return nil, errUnsupported // \d \w \s \b ...
	}
	e.pos++
	return []string{string(c)}, nil
}

func literalEscape(c byte) bool {
	switch c {
	case '.', '+', '-', '_', '\'', '/', '&', ',', '!', '(', ')', '[', ']', '{', '}', '|', '?', '*', '^', '$', '\\', ' ':
		return true
	}
	return false
}

// findFunc is taginfo.Find with the linear scan indexed away.
//
// taginfo.Find tests every row's compiled regexp in turn -- 123 of them for
// source -- and Tag.Info calls it twice (once directly, once via Normalize).
// It is the largest single cost in the build phase.
//
// Each row's regexp is fully anchored, so the strings it can match are exactly
// the spellings it expands to. Indexing those by upper cased spelling narrows a
// lookup to a handful of candidate rows, which are then confirmed with the same
// Match call as before -- so case sensitive rows, ie (?-i:US|USA), still decide
// for themselves. Rows that could not be expanded stay in an ordered fallback
// list, and candidates are visited in row order, so the row selected is
// identical to the one the scan would have returned.
func findFunc(infos ...*taginfo.Taginfo) taginfo.FindFunc {
	exact := make(map[string][]int32)
	var rest []int32
	for i, info := range infos {
		v, err := expandRE(info.RE(), true)
		if err != nil {
			rest = append(rest, int32(i))
			continue
		}
		for _, s := range v {
			if s == "" {
				continue
			}
			k := strings.ToUpper(s)
			if c := exact[k]; len(c) == 0 || c[len(c)-1] != int32(i) {
				exact[k] = append(c, int32(i))
			}
		}
	}
	return func(s string) *taginfo.Taginfo {
		// upper case into a stack buffer: the map lookup below then avoids the
		// allocation strings.ToUpper would make
		var key [48]byte
		var cand []int32
		if len(s) <= len(key) {
			for j := 0; j < len(s); j++ {
				c := s[j]
				if c >= 'a' && c <= 'z' {
					c -= 32
				}
				key[j] = c
			}
			cand = exact[string(key[:len(s)])]
		} else {
			cand = exact[strings.ToUpper(s)]
		}
		// walk cand and rest together in row order
		for a, b := 0, 0; a < len(cand) || b < len(rest); {
			var i int32
			if b >= len(rest) || (a < len(cand) && cand[a] < rest[b]) {
				i, a = cand[a], a+1
			} else {
				i, b = rest[b], b+1
			}
			if infos[i].Match(s) {
				return infos[i]
			}
		}
		return nil
	}
}
