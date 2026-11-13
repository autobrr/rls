package rls

import (
	"errors"
	"regexp/syntax"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

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
//
// That holds for ASCII text. Runs are compared byte for byte, and (?i) also
// matches 'k' and 's' with the non-ASCII runes U+212A and U+017F, so a
// spelling written with one of those is rejected unless fold is set.
//
// No other rune outside ASCII is matched by a spelling, which is ASCII: such
// a rune ends a run as a delimiter does, and at the start of a position only
// an unexpanded row can match it.
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
	// rows are the unexpanded rows themselves: the bytes each can begin with,
	// and a literal each of its matches has to contain. A position is only
	// handed to the alternation for such a row when both are present, which
	// keeps a row like ([A-Zi]*)([SD])UB([A-Zi]*) -- able to begin with any
	// letter -- from admitting every word.
	rows []patterned
	// fold makes maybe exact against (?i), which matches 'k' and 's' with the
	// non-ASCII runes U+212A and U+017F: a run that stops at one of those
	// goes on through it as far as the regexp is concerned, so it is let
	// through. It is set where the prefilter gates something that ran
	// unconditionally before. The older prefilters leave it off and so
	// reject 'M\u212aV' where the container alternation would match.
	fold bool
	// nonRun are the bytes, none of them a letter or a digit, that a spelling
	// of a row begins with ('-sign-language'). A position that begins with
	// any other such byte can only be matched by an unexpanded row.
	nonRun [256]bool
}

type patterned struct {
	lead [256]bool
	lit  []byte
	fold bool
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
	res := make([]string, len(infos))
	for i, info := range infos {
		res[i] = info.RE()
	}
	return newPrefilterRE(res, ci)
}

// noPrefilter makes every type go without a prefilter, for a test to compare
// a parser built that way with one that has them.
var noPrefilter bool

// newPrefilterRE is newPrefilter for bare regexps.
func newPrefilterRE(res []string, ci bool) (*prefilter, bool) {
	if noPrefilter {
		return nil, false
	}
	pf := &prefilter{runs: make(map[string]struct{})}
	for _, re := range res {
		v, err := expandRE(re, ci)
		if err != nil {
			// the row cannot be reduced to literals. Fall back to bounding its
			// first byte, which is enough for the patterned rows; if even that
			// is unbounded the type gets no prefilter at all.
			var lead [256]bool
			if !firstBytes(re, &lead) {
				return nil, false
			}
			row := patterned{}
			for c := 0; c < 256; c++ {
				if lead[c] {
					pf.lead[c], pf.hasLead, row.lead[c] = true, true, true
					// a lead byte is only consulted for runs, so fold case
					if c >= 'a' && c <= 'z' {
						pf.lead[c-32], row.lead[c-32] = true, true
					} else if c >= 'A' && c <= 'Z' {
						pf.lead[c+32], row.lead[c+32] = true, true
					}
				}
			}
			row.lit, row.fold = requiredLiteral(re, ci)
			pf.rows = append(pf.rows, row)
			continue
		}
		for _, s := range v {
			run := firstRun(s)
			if run == "" {
				// an alternative that opens with a non-run byte can never match
				// at a position maybe is consulted for: maybe only rejects when
				// buf[i] is a run byte, and such an alternative needs buf[i] to
				// be its (non-run) first byte. It contributes nothing to key on
				// and must not cost the type its prefilter ('other' lost its
				// prefilter to the '-sign-language' spelling of Sign.Language).
				pf.fold = true
				if s != "" {
					pf.nonRun[s[0]] = true
				}
				continue
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
	switch {
	case j == 0 && (i >= n || pf.nonRun[buf[i]] || foldRune(buf, i, n)):
		// not on a letter or a digit: a spelling begins with this byte, or
		// with the letter this rune folds to. Any other byte begins a match
		// of an unexpanded row at most, which the lead bytes below decide
		return true
	case j == 0:
	case pf.fold && foldRune(buf, i+j, n):
		return true
	case j <= pf.max:
		if _, ok := pf.runs[string(key[:j])]; ok {
			return true
		}
	}
	// no literal run matched; an unexpanded row may still match here
	if !pf.hasLead || !pf.lead[buf[i]] {
		return false
	}
	for k := range pf.rows {
		if row := &pf.rows[k]; row.lead[buf[i]] && (len(row.lit) == 0 || hasLiteral(buf[i:n], row.lit, row.fold)) {
			return true
		}
	}
	return false
}

// foldRune reports whether b[i:n] begins with one of the two runes outside
// ASCII that an ASCII letter matches without regard to case: U+017F for 's'
// and U+212A for 'k'.
func foldRune(b []byte, i, n int) bool {
	if i >= n {
		return false
	}
	switch b[i] {
	case 0xc5:
		return i+1 < n && b[i+1] == 0xbf
	case 0xe2:
		return i+2 < n && b[i+1] == 0x84 && b[i+2] == 0xaa
	}
	return false
}

// requiredLiteral returns the longest literal every match of re must contain,
// lower cased when it is matched case insensitively, or nil when none can be
// shown.
func requiredLiteral(re string, ci bool) ([]byte, bool) {
	flags := syntax.Perl
	if ci {
		flags |= syntax.FoldCase
	}
	r, err := syntax.Parse(re, flags)
	if err != nil {
		return nil, false
	}
	var best []rune
	var fold bool
	var walk func(*syntax.Regexp)
	walk = func(r *syntax.Regexp) {
		switch r.Op {
		case syntax.OpLiteral:
			if len(r.Rune) > len(best) {
				best, fold = r.Rune, r.Flags&syntax.FoldCase != 0
			}
		case syntax.OpCapture, syntax.OpPlus, syntax.OpConcat:
			for _, sub := range r.Sub {
				walk(sub)
			}
		case syntax.OpRepeat:
			if r.Min > 0 {
				walk(r.Sub[0])
			}
		}
	}
	walk(r)
	for _, c := range best {
		if c >= 0x80 {
			return nil, false
		}
	}
	lit := []byte(string(best))
	if fold {
		lit = []byte(strings.ToLower(string(best)))
	}
	return lit, fold
}

// literalWindow bounds how far hasLiteral looks, so that a long name of
// patterned looking tokens costs a fixed amount per token rather than a scan
// of everything after it.
const literalWindow = 256

// hasLiteral reports whether b contains lit, or a non-ASCII byte. The latter
// is treated as a hit because a case insensitive literal also matches the
// non-ASCII folds of 'k' and 's' (U+212A, U+017F). A b longer than
// literalWindow is not examined and counts as a hit.
func hasLiteral(b, lit []byte, fold bool) bool {
	if len(b) > literalWindow {
		return true
	}
	c0 := lit[0]
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c >= 0x80 {
			return true
		}
		if fold && c >= 'A' && c <= 'Z' {
			c += 32
		}
		if c != c0 || len(b)-i < len(lit) {
			continue
		}
		j := 1
		for ; j < len(lit); j++ {
			d := b[i+j]
			if fold && d >= 'A' && d <= 'Z' {
				d += 32
			}
			if d != lit[j] {
				break
			}
		}
		if j == len(lit) {
			return true
		}
	}
	return false
}

// maybePrefix is maybe for a lexer whose match may run on past the first run of
// its alternation without a boundary, as the audio lexer's does into channel
// digits ('DDP5.1'), an attached codec ('AACx264') or Atmos ('DDPAtmos'). The
// run at i then only has to begin with one of the type's first runs, and
// continue with a byte from next.
func (pf *prefilter) maybePrefix(buf []byte, i, n int, next *[256]bool) bool {
	if pf == nil {
		return true
	}
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
	if j == 0 || j == len(key) || foldRune(buf, i+j, n) || pf.hasLead {
		return true
	}
	for l := 1; l <= j && l <= pf.max; l++ {
		if l < j && !next[buf[i+l]] {
			continue
		}
		if _, ok := pf.runs[string(key[:l])]; ok {
			return true
		}
	}
	return false
}

// firstBytes adds to lead the bytes a match of re can begin with, reporting
// false when it gives no set: for a regexp that begins with any rune ('.'),
// for one that matches the empty string, which it does at every position, and
// for one that does not compile. The callers skip a position whose byte is not
// in the set without running the regexp, so a byte left out loses a tag, and
// one too many costs a run of the regexp. A set may hold nearly every byte
// ('[^a]'), and it is empty for a regexp that matches no string.
//
// The set is read off the program the regexp compiles to, which is what
// matches it: every instruction that takes a rune and is reached from the
// start with nothing read. An assertion on the way is taken to hold ('^',
// '$', '\b').
//
// Some callers match the regexp as it is written and some without regard to
// case, and the set is that of both readings: 'k' begins with 'K' and with
// the first byte of the Kelvin sign, and '[^k]' begins with 'K' as well.
func firstBytes(re string, lead *[256]bool) bool {
	var set [256]bool
	for _, flags := range [...]syntax.Flags{syntax.Perl, syntax.Perl | syntax.FoldCase} {
		r, err := syntax.Parse(re, flags)
		if err != nil {
			return false
		}
		prog, err := syntax.Compile(r.Simplify())
		if err != nil {
			return false
		}
		seen := make([]bool, len(prog.Inst))
		stack := []uint32{uint32(prog.Start)}
		for len(stack) != 0 {
			pc := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[pc] {
				continue
			}
			seen[pc] = true
			switch inst := &prog.Inst[pc]; inst.Op {
			case syntax.InstAlt, syntax.InstAltMatch:
				stack = append(stack, inst.Out, inst.Arg)
			case syntax.InstCapture, syntax.InstNop, syntax.InstEmptyWidth:
				stack = append(stack, inst.Out)
			case syntax.InstMatch, syntax.InstRuneAny, syntax.InstRuneAnyNotNL:
				return false
			case syntax.InstRune, syntax.InstRune1:
				instBytes(inst, &set)
			}
		}
	}
	for c, ok := range set {
		lead[c] = lead[c] || ok
	}
	return true
}

// instBytes adds to set the first byte of every rune the instruction takes.
func instBytes(inst *syntax.Inst, set *[256]bool) {
	// one rune, in every case it is written in when the instruction folds
	if len(inst.Rune) == 1 {
		r := inst.Rune[0]
		rangeBytes(r, r, set)
		if syntax.Flags(inst.Arg)&syntax.FoldCase != 0 {
			for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
				rangeBytes(f, f, set)
			}
		}
		return
	}
	for i := 0; i+1 < len(inst.Rune); i += 2 {
		rangeBytes(inst.Rune[i], inst.Rune[i+1], set)
	}
}

// rangeBytes adds to set the first byte of the UTF-8 of every rune from lo to
// hi. A byte that is no UTF-8 is read as U+FFFD, so a range with that rune in
// it begins with any such byte.
func rangeBytes(lo, hi rune, set *[256]bool) {
	for c := lo; c <= hi && c < utf8.RuneSelf; c++ {
		set[c] = true
	}
	if hi < utf8.RuneSelf {
		return
	}
	lo = max(lo, utf8.RuneSelf)
	if lo <= utf8.RuneError && utf8.RuneError <= hi {
		for c := utf8.RuneSelf; c < len(set); c++ {
			set[c] = true
		}
		return
	}
	// a surrogate has no encoding and no input decodes to one. Written as
	// the literal a regexp begins with it is looked for as the bytes of
	// U+FFFD all the same ('\x{D800}x' matches "�x")
	const surrogateMin, surrogateMax = 0xd800, 0xdfff
	if lo <= surrogateMax && hi >= surrogateMin {
		set[0xef] = true
	}
	// the first bytes grow with the runes on either side of the surrogates
	if lo >= surrogateMin && lo <= surrogateMax {
		lo = surrogateMax + 1
	}
	if hi >= surrogateMin && hi <= surrogateMax {
		hi = surrogateMin - 1
	}
	if lo > hi {
		return
	}
	var a, b [utf8.UTFMax]byte
	utf8.EncodeRune(a[:], lo)
	utf8.EncodeRune(b[:], hi)
	for c := int(a[0]); c <= int(b[0]); c++ {
		set[c] = true
	}
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
//
// A regexp with a non-ASCII rune is unsupported too. The expander works on
// bytes, so '[uü]' would give 'u' and the two bytes of 'ü' as three members,
// and the spellings built from them match nothing.
//
// The spellings are cached and shared: a row is expanded by its lexer, by the
// group and meta lexers and by the builder, and a caller must not change the
// slice it gets.
func expandRE(re string, ci bool) ([]string, error) {
	key := expandKey{re, ci}
	if v, ok := expanded.Load(key); ok {
		x := v.(expansion)
		return x.v, x.err
	}
	v, err := expandREOnce(re, ci)
	// the rows of a program are few. The bound is for one that makes rows up
	// as it runs
	if expandedLen.Add(1) <= maxExpanded {
		expanded.Store(key, expansion{v, err})
	}
	return v, err
}

// expandKey is what an expansion depends on.
type expandKey struct {
	re string
	ci bool
}

// expansion is what expandRE gave for a key.
type expansion struct {
	v   []string
	err error
}

// maxExpanded is how many expansions are cached.
const maxExpanded = 8192

var (
	expanded    sync.Map
	expandedLen atomic.Int64
)

// expandREOnce is expandRE without the cache.
func expandREOnce(re string, ci bool) ([]string, error) {
	for i := 0; i < len(re); i++ {
		if re[i] >= 0x80 {
			return nil, errUnsupported
		}
	}
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
		// a quantifier after the quote applies to its last character alone,
		// which a list of whole literals cannot say
		if !e.eof() && strings.IndexByte("?*+{", e.src[e.pos]) != -1 {
			return nil, errUnsupported
		}
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

// A pairSet holds which pairs of leading ASCII bytes a ^-anchored regexp can
// begin a match with, found by walking the compiled program two bytes deep.
// Zero width assertions are assumed to hold, so the set over-approximates: a
// pair outside it cannot start a match.
type pairSet [128 * 128 / 64]uint64

func (ps *pairSet) has(a, b byte) bool {
	k := uint(a)<<7 | uint(b)
	return ps[k>>6]&(1<<(k&63)) != 0
}

// newPairSet returns the pair set for re, or nil when it could not be built.
func newPairSet(re string) *pairSet {
	r, err := syntax.Parse(re, syntax.Perl)
	if err != nil {
		return nil
	}
	prog, err := syntax.Compile(r.Simplify())
	if err != nil {
		return nil
	}
	// closure expands pcs through everything that consumes no input,
	// reporting whether a match is reachable
	closure := func(pcs []uint32) ([]uint32, bool) {
		seen, out, match := make(map[uint32]bool), []uint32(nil), false
		var add func(uint32)
		add = func(pc uint32) {
			if seen[pc] {
				return
			}
			seen[pc] = true
			switch inst := &prog.Inst[pc]; inst.Op {
			case syntax.InstAlt, syntax.InstAltMatch:
				add(inst.Out)
				add(inst.Arg)
			case syntax.InstCapture, syntax.InstNop, syntax.InstEmptyWidth:
				add(inst.Out)
			case syntax.InstMatch:
				match = true
			case syntax.InstFail:
			default:
				out = append(out, pc)
			}
		}
		for _, pc := range pcs {
			add(pc)
		}
		return out, match
	}
	step := func(pcs []uint32, c byte) []uint32 {
		var out []uint32
		for _, pc := range pcs {
			if inst := &prog.Inst[pc]; inst.MatchRune(rune(c)) {
				out = append(out, inst.Out)
			}
		}
		return out
	}
	ps := new(pairSet)
	s0, match := closure([]uint32{uint32(prog.Start)})
	if match {
		return nil // matches the empty string: nothing to rule out
	}
	for a := 0; a < 128; a++ {
		s1, match := closure(step(s0, byte(a)))
		for b := 0; b < 128; b++ {
			if match || len(step(s1, byte(b))) != 0 {
				k := uint(a)<<7 | uint(b)
				ps[k>>6] |= 1 << (k & 63)
			}
		}
	}
	return ps
}
