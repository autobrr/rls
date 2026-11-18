package rls

import (
	"bytes"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unsafe"

	"github.com/autobrr/rls/reutil"
	"github.com/autobrr/rls/taginfo"
)

// TagParser is a release tag parser.
type TagParser struct {
	tagParser
	// load returns what the built-in default parser parses with, which it
	// builds when first called (see DefaultParser), and tagParser is then not
	// used. It is nil for every other parser. A copy of the default parser
	// has the same load, so it parses with the same state, built once. Only
	// SetBuilder writes load, and no parse may run beside that, as before.
	load func() *tagParser
}

// tagParser is what a TagParser parses with.
type tagParser struct {
	builder  Builder
	ellip    []byte
	once     []LexFunc
	multi    []LexFunc
	notFirst []bool
	// dispatch gives the lexers of multi that can match at a position.
	dispatch *dispatch
	// pooled is whether the tags are lexed into scratch from the pool, which
	// they are when every lexer is one of this package's. A lexer of a
	// caller's may keep the slices it is handed, so a parser with one makes
	// them for each parse alone.
	pooled bool
	// isDelim is delim as a byte table: every delimiter is a single ASCII
	// byte, so membership is all the regexp ever decided.
	isDelim [256]bool
}

// NewTagParser creates a new release tag parser.
func NewTagParser(infos map[string][]*taginfo.Taginfo, lexers ...Lexer) Parser {
	return newTagParser(DefaultBuilder, infos, lexers...)
}

// newTagParser is NewTagParser with the builder to use.
func newTagParser(builder Builder, infos map[string][]*taginfo.Taginfo, lexers ...Lexer) *TagParser {
	// delims
	var v []string
	for r := range rune(255) {
		if isAnyDelim(r) {
			v = append(v, string(r))
		}
	}
	delim := regexp.MustCompile(`^((?:` + reutil.Join(true, v...) + ")+)")
	// build short tags. A word that is a tag in any case is stored in upper
	// case as true. A tag of one word whose regexp matches it only as written
	// ('(?-i:MAX)') is stored as written, and in upper case when that matches,
	// as false: the word in another case is a group ('[Max] Show - 01')
	short := make(map[string]bool)
	hdr := strings.ToLower(TagTypeHDR.String())
	for typ, v := range infos {
		if typ == hdr {
			continue
		}
		for _, info := range v {
			fields := strings.FieldsFunc(info.Tag(), isAnyDelim)
			for _, field := range fields {
				if len(field) >= 5 || strings.Contains(field, "$") {
					continue
				}
				u := strings.ToUpper(field)
				if len(fields) != 1 || info.Match(strings.ToLower(field)) {
					short[u] = true
					continue
				}
				for _, w := range []string{field, u} {
					if _, ok := short[w]; !ok && info.Match(w) {
						short[w] = false
					}
				}
			}
		}
	}
	// separate once and multi. The lexers of this package offer what they
	// need at a position to match while they are initialized, see withGate
	offer := &gateOffer{gates: make(map[unsafe.Pointer]*lexGate)}
	gateOffers.Store(delim, offer)
	defer gateOffers.Delete(delim)
	var once, multi []LexFunc
	var notFirst []bool
	var gates []*lexGate
	pooled := true
	for _, lexer := range lexers {
		f, o, nf := lexer.Initialize(infos, delim, short)
		offer.mu.Lock()
		gate, own := offer.gates[funcID(f)]
		offer.mu.Unlock()
		pooled = pooled && own
		if o {
			once = append(once, f)
		} else {
			multi = append(multi, f)
			notFirst = append(notFirst, nf)
			gates = append(gates, gate)
		}
	}
	// init builder
	if b, ok := builder.(interface {
		Init(map[string][]*taginfo.Taginfo) Builder
	}); ok {
		builder = b.Init(infos)
	}
	p := &TagParser{tagParser: tagParser{
		builder:  builder,
		ellip:    []byte("..."),
		once:     once,
		multi:    multi,
		notFirst: notFirst,
		dispatch: newDispatch(gates),
		pooled:   pooled,
	}}
	for c := range 255 {
		p.isDelim[c] = isAnyDelim(rune(c))
	}
	return p
}

// ParserOption is an option for NewParser.
type ParserOption func(*parserOptions)

// parserOptions are what the options given to NewParser change.
type parserOptions struct {
	extras []map[string][]*taginfo.Taginfo
	lexers []Lexer
}

// NewParser creates a tag parser with the built-in tag infos and the default
// lexers, changed by opts. With no options it builds the parser
// NewDefaultParser builds. It returns the concrete type, as ParseHint and
// SetBuilder are methods of it, and it can still be assigned to DefaultParser.
// The builder is DefaultBuilder, as with NewTagParser.
func NewParser(opts ...ParserOption) *TagParser {
	var o parserOptions
	for _, opt := range opts {
		opt(&o)
	}
	lexers := o.lexers
	if len(lexers) == 0 {
		lexers = DefaultLexers()
	}
	return NewTagParser(taginfo.All(o.extras...), lexers...).(*TagParser)
}

// WithTaginfo adds tag infos to the built-in ones, keyed by tag type as
// taginfo.All keys them ('collection'), the way a caller adds the streaming
// services it knows. It may be given more than once, and the maps add up in
// the order given. The infos under a key the built-in infos already have are
// added after the built-in ones and do not replace them, so where a built-in
// and an extra info match the same text the built-in one is found. A nil map
// adds nothing, and a nil info in a map is left out, which is what a caller
// has that did not check the error of taginfo.New.
func WithTaginfo(extra map[string][]*taginfo.Taginfo) ParserOption {
	return func(o *parserOptions) {
		m := make(map[string][]*taginfo.Taginfo, len(extra))
		for typ, infos := range extra {
			for _, info := range infos {
				if info != nil {
					m[typ] = append(m[typ], info)
				}
			}
		}
		o.extras = append(o.extras, m)
	}
}

// WithLexers replaces the default lexers. Given no lexers it changes nothing,
// and the default lexers, or those of an earlier WithLexers, stay in place, as
// a parser with no lexers would read every name as text. A lexer keeps what
// its parser gave it when it was built, so every parser needs lexers of its
// own: do not give the same Lexer values to two parsers.
func WithLexers(lexers ...Lexer) ParserOption {
	return func(o *parserOptions) {
		if len(lexers) != 0 {
			o.lexers = lexers
		}
	}
}

// Hint is what a caller knows about a name before it is parsed, such as the
// category of the indexer section it was announced in.
type Hint struct {
	// Type is the release type when the caller knows it. Unknown lets the
	// parser decide, as ParseRelease does, and so does a value that is not
	// one of the release types.
	Type Type
	// DayFirst reads a date written with two numbers and a year as day,
	// month, year: '05.06.2024' is 5 June. Without it that is 6 May, and a
	// date is read day first only when its first number cannot be a month
	// ('13.06.2024').
	DayFirst bool
	// Sports says the name is a sports broadcast. It changes what Event
	// returns and nothing else: Event then reports the round, the session
	// and the rest of what it reads for any name of any type, where it
	// otherwise wants a competition it knows and signs of an event. Type,
	// Title and Subtitle are what they are without it.
	Sports bool
}

// ParseHint parses a release from src with what the caller knows about it.
// The zero Hint gives what ParseRelease gives.
//
// The type in h is the release type, and the titles and the tags that are
// kept follow it. It is set when the parser's builder is a *TagBuilder; with
// any other Builder the release is what its Build returns, and the type in h
// has no effect. DayFirst applies to the date tags before any builder sees
// them, and Sports is kept for Event with any builder.
func (p *TagParser) ParseHint(src []byte, h Hint) Release {
	tags, end := parseCopy(p, src)
	return p.buildHint(tags, end, h)
}

// buildHint builds the release of tags that a parse of p made, with what the
// caller knows about the name.
func (p *TagParser) buildHint(tags []Tag, end int, h Hint) Release {
	if h.DayFirst {
		dayFirst(tags)
	}
	var r Release
	builder := p.state().builder
	if b, ok := builder.(*TagBuilder); ok {
		r = b.build(tags, end, h)
	} else {
		r = builder.Build(tags, end)
	}
	r.sports = h.Sports
	return r
}

// dayFirst swaps the month and day of each date tag that was written as two
// two-digit numbers and a four-digit year ('05.06.2024') and read month first,
// when the second number can also be a month. A date read day first already
// ('13.06.2024'), one whose second number is no month ('06.13.2024'), and one
// written year first, with a month name or with a two-digit year are left
// alone. The text as written stays, so the name still prints as given.
//
// The tags are those Parse made for this call alone, so changing their values
// in place is safe.
func dayFirst(tags []Tag) {
	for i := range tags {
		v := tags[i].v
		// a range of years has a fifth value, and is written year first
		if tags[i].typ != TagTypeDate || len(v) != 4 {
			continue
		}
		s := v[0]
		if len(s) != 10 || !isDateSep(s[2]) || !isDateSep(s[5]) || s[6:] != v[1] {
			continue
		}
		// the lexer writes month and day with two digits, so the first
		// number read as the month and the second as the day means the date
		// was read month first, and a second number from 01 to 12 can be the
		// month
		if first, second := s[:2], s[3:5]; v[2] == first && v[3] == second && second >= "01" && second <= "12" {
			v[2], v[3] = second, first
		}
	}
}

// isDateSep reports whether c separates the numbers of a date. A comma and a
// plus do too, since the lexers read them as spaces.
func isDateSep(c byte) bool {
	return c == '-' || c == '.' || c == '_' || c == ' ' || c == ',' || c == '+'
}

// SetBuilder sets the builder for the tag parser.
func (p *TagParser) SetBuilder(builder Builder) {
	if p.load != nil {
		// the default parser, or a copy of it, gets a state of its own, so
		// the builder is set for this parser alone
		p.tagParser, p.load = *p.load(), nil
	}
	p.builder = builder
}

// state returns what p parses with: its own tagParser, or for the built-in
// default parser the one that load builds on first use.
func (p *TagParser) state() *tagParser {
	if p.load != nil {
		return p.load()
	}
	return &p.tagParser
}

// Parse parses tags in buf.
func (p *TagParser) Parse(src []byte) ([]Tag, int) {
	return parseCopy(p, src)
}

// parseCopy parses the tags of a name that is the caller's, given as bytes or
// as a string. The name is copied into one piece of memory with its working
// copy, which the tags then share.
func parseCopy[T string | []byte](p *TagParser, name T) ([]Tag, int) {
	mem := make([]byte, 2*len(name))
	n := copy(mem, name)
	return p.lex(mem[:n:n], mem[n:])
}

// parse parses the tags of src, which is the parser's to keep: the tags hold
// pieces of it and not copies. So it has to be bytes that nothing else holds
// and nothing writes to from here on, which a copy of what the caller gave
// is. The lexers of this package only read it. The tags keep it and the
// working copy alive, and nothing of the caller's.
func (p *TagParser) parse(src []byte) ([]Tag, int) {
	return p.lex(src, make([]byte, len(src)))
}

// lex is parse with the room for the working copy, which is as long as src.
//
// The tags are lexed into scratch that the next parse uses again, and copied
// out of it: the tags a caller gets are a slice of their own that is exactly
// as long as they are, and nothing of the scratch. With a lexer of a caller's
// in the parser the room is made for the parse alone, as such a lexer may
// keep the slices it is handed.
func (p *TagParser) lex(src, buf []byte) ([]Tag, int) {
	return p.state().run(src, buf)
}

// run is lex with the state of the parser.
func (p *tagParser) run(src, buf []byte) ([]Tag, int) {
	// working buf
	for i, c := range src {
		switch c {
		case '_', ',', '+':
			c = ' '
		}
		buf[i] = c
	}
	i, n := 0, len(buf)
	var sc *tagScratch
	var start, end []Tag
	if p.pooled {
		sc = tagScratchPool.Get().(*tagScratch)
		start, end = sc.take(len(src))
	} else {
		start = make([]Tag, 0, tagRoom(len(src)))
	}
	// once
	for _, f := range p.once {
		start, end, i, n, _ = f(src, buf, start, end, i, n)
	}
	// consume
	notFirst := false
	for i < n {
		start, end, i, _ = p.next(src, buf, start, end, i, n, &notFirst)
	}
	// start, and then end reversed, with the delimiters the lexers took into
	// their tags as tags of their own
	tags, k := p.join(src, start, end)
	if sc != nil {
		sc.give(start, end)
		tagScratchPool.Put(sc)
	}
	return tags, k
}

// tagScratch is the room a parse lexes its tags into: those read from the
// start of the name and those read from its end. A parse takes one from the
// pool and gives it back, so that the room is made once and not for every
// name, and is not what a caller keeps.
//
// A scratch only ever holds memory that take made. What the lexers hand back
// in its place, because they outgrew it or built a slice of their own, is
// used for that parse and then left to them.
type tagScratch struct {
	start, end []Tag
}

var tagScratchPool = sync.Pool{New: func() any { return new(tagScratch) }}

// maxScratch is how many tags the scratch that goes back to the pool has
// room for. A name that needs more gets room of its own.
const maxScratch = 1024

// endScratch is the room for the tags read from the end of a name, which are
// few: the extension, the group, the meta suffixes and their delimiters.
const endScratch = 16

// tagRoom is how many tags to make room for in a name of n bytes: a name
// yields roughly one tag per three bytes, and room for one per two holds them
// without growing.
func tagRoom(n int) int {
	return n/2 + 8
}

// take returns the room for the tags of a name of n bytes, empty.
func (sc *tagScratch) take(n int) (start, end []Tag) {
	want := tagRoom(n)
	switch {
	case cap(sc.start) >= want:
	case want <= maxScratch:
		sc.start = make([]Tag, 0, max(want, 64))
	default:
		return make([]Tag, 0, want), sc.ends()
	}
	return sc.start[:0], sc.ends()
}

// ends returns the room for the tags read from the end of a name.
func (sc *tagScratch) ends() []Tag {
	if sc.end == nil {
		sc.end = make([]Tag, 0, endScratch)
	}
	return sc.end[:0]
}

// give takes the room back from a parse that lexed start and end into it,
// and empties it, so that the scratch keeps no name alive while it waits for
// the next parse.
func (sc *tagScratch) give(start, end []Tag) {
	emptyScratch(sc.start, start)
	emptyScratch(sc.end, end)
}

// emptyScratch clears the tags a parse left in room, which is the room of a
// scratch. used is what the lexers made of it: the same memory with tags in
// it, or other memory when the room was not theirs or they left it, in which
// case there is no telling how far they wrote.
func emptyScratch(room, used []Tag) {
	if cap(room) != 0 && cap(used) != 0 && unsafe.SliceData(room) == unsafe.SliceData(used) {
		clear(used)
		return
	}
	clear(room[:cap(room)])
}

// next reads the next token from src up to the next delimiter. Iterates over
// the lexers until a match occurs. If none of the lexers match, then iterates
// over src, capturing all text, until src is exhausted or until a delimiter is
// encountered. Appends captured tags to start or end (where appropriate)
// returning the modified slices and new values of i, n.
//
// Lexers have the choice of matching against src or buf. Buf is the working
// version of src, with underscores and other runes replaced with spaces. Using
// buf allows lexers to matching with Go regexp `\b`.
func (p *tagParser) next(src, buf []byte, start, end []Tag, i, n int, notFirst *bool) ([]Tag, []Tag, int, int) {
	// delimiter
	if bytes.HasPrefix(src[i:n], p.ellip) {
		return append(
			start,
			newTag(src, buf, TagTypeDelim, nil, p.ellip, p.ellip),
		), end, i + len(p.ellip), n
	} else if p.isDelim[src[i]] {
		j := i + 1
		for j < n && p.isDelim[src[j]] {
			j++
		}
		return append(
			start,
			newTag(src, buf, TagTypeDelim, nil, src[i:j], src[i:j]),
		), end, j, n
	}
	// run the lexers that can match here, in lexer order
	startn, mask := len(start), p.dispatch.mask(buf, i, n)
	for a := nextLexer(mask, 0); a < len(p.multi); a = nextLexer(mask, a+1) {
		if p.notFirst[a] && !*notFirst {
			continue
		}
		// a match has to consume input from one end or the other. One that
		// does not would be offered the same input again without end, so it
		// is taken as no match and the next lexer is tried
		s, e, j, k, ok := p.multi[a](src, buf, start, end, i, n)
		if !ok {
			continue
		}
		if i <= j && j <= k && k <= n && (i < j || k < n) {
			*notFirst = *notFirst || startn != len(s)
			return s, e, j, k
		}
		// the tags of a match that is not taken are behind those that are.
		// In scratch, which later parses use again, they are cleared
		if p.pooled && len(s) > len(start) {
			clear(s[len(start):])
		}
		if p.pooled && len(e) > len(end) {
			clear(e[len(end):])
		}
	}
	// text
	j := i
	for j < n && !p.isDelim[src[j]] {
		j++
	}
	*notFirst = true
	return append(
		start,
		newTag(src, buf, TagTypeText, nil, src[i:j], src[i:j]),
	), end, j, n
}

// ParseRelease parses a release from src.
func (p *TagParser) ParseRelease(src []byte) Release {
	tags, end := p.Parse(src)
	return p.state().builder.Build(tags, end)
}

// TagBuilder is a release builder.
type TagBuilder struct {
	// missing finds acronyms without periods.
	missing *regexp.Regexp
	// bad finds bad acronyms with periods.
	bad *regexp.Regexp
	// fix fixes issues with acronyms.
	fix *regexp.Regexp
	// spaces matches multiple spaces.
	spaces *regexp.Regexp
	// ellips matches multiple 3 or more periods (an ellipsis...).
	ellips *regexp.Regexp
	// sum matches a file checksum.
	sum *regexp.Regexp
	// digpre matches digit prefixes.
	digpre *regexp.Regexp
	// digsuf matches digit suffixes.
	digsuf *regexp.Regexp
	// containerf is the container find func.
	containerf *finder
	// audiof is the audio find func.
	audiof *finder
	// resolutionf is the resolution find func.
	resolutionf *finder
	// languagef is the language find func.
	languagef *finder
	// groupf is the group find func.
	groupf *finder
	// infos are the rows of the vocabulary, and words finds those of every
	// type but the group that a word may be. It is made when a name first
	// asks for it, see tagged.
	infos map[string][]*taginfo.Taginfo
	words atomic.Pointer[tagWords]
}

// tagWords are the rows of the vocabulary that a word is asked against:
// those with spellings by a spelling in upper case, and the rows that only
// their regexp confirms by a byte they begin with, see firstBytes.
type tagWords struct {
	spelled map[string][]wordRow
	rest    [256][]*taginfo.Taginfo
}

// wordRow is a row of the vocabulary in the finder of its type.
type wordRow struct {
	f *finder
	i int32
}

// NewTagBuilder creates a new release builder.
func NewTagBuilder() *TagBuilder {
	return &TagBuilder{
		missing: regexp.MustCompile(`\b[A-Z][\. ][A-Z](?:[\. ][A-Z])*[\. ]?\b`),
		bad:     regexp.MustCompile(`[^A-Z][-\. ][A-Z]\.($|[^A-Z])`),
		fix:     regexp.MustCompile(`([A-Z])\.`),
		spaces:  regexp.MustCompile(`\s+`),
		ellips:  regexp.MustCompile(`\.{3,}`),
		sum:     regexp.MustCompile(`(?i)^[a-f0-9]{8}$`),
		digpre:  regexp.MustCompile(`^\d+`),
		digsuf:  regexp.MustCompile(`\d+$`),
	}
}

// Init creates a new builder using the provided infos.
func (b *TagBuilder) Init(infos map[string][]*taginfo.Taginfo) Builder {
	return &TagBuilder{
		missing:     b.missing,
		bad:         b.bad,
		fix:         b.fix,
		spaces:      b.spaces,
		ellips:      b.ellips,
		sum:         b.sum,
		digpre:      b.digpre,
		digsuf:      b.digsuf,
		containerf:  newFinder(TagTypeContainer, infos["container"]...),
		audiof:      newFinder(TagTypeAudio, infos["audio"]...),
		resolutionf: newFinder(TagTypeResolution, infos["resolution"]...),
		languagef:   newFinder(TagTypeLanguage, infos["language"]...),
		groupf:      newFinder(TagTypeGroup, infos["group"]...),
		infos:       infos,
	}
}

// Build builds a release from tags.
func (b *TagBuilder) Build(tags []Tag, end int) Release {
	return b.build(tags, end, Hint{})
}

// build builds a release from tags with what the caller knows about it. A
// type in h is set before specialDate, unset and titles, the steps that read
// the type, so that the tags that are unset and the titles that are read are
// those of that type. Nothing before them reads it.
func (b *TagBuilder) build(tags []Tag, end int, h Hint) Release {
	r := b.assemble(tags, end, h)
	r.Flags, r.Repack = readOther(r.Other)
	return r
}

// assemble builds a release from tags as build does, but for the readings of
// Other, which build takes from the Other that assemble leaves.
func (b *TagBuilder) assemble(tags []Tag, end int, h Hint) Release {
	// flags in brackets that open the name ('[REPACK] [Group] Show'), which
	// only the meta lexer puts there, are left out of the build of the rest,
	// so that the rest reads as if it opened the name
	if k, cut := leadingFlags(tags, end); k != 0 {
		// a bracket after the flags opens in the delimiter that closes them
		// ('] [1080p]', '] (2019)'), and the rest is read with what is left
		// of that delimiter
		closer, from := tags[k], k+1
		d := closer.Delim()
		if cut < len(d) {
			tags[k], from = NewTag(TagTypeDelim, nil, []byte(d[cut:]), []byte(d[cut:])), k
		}
		r := b.assemble(tags[from:], end-from, h)
		tags[k] = closer
		// the build may have made tags of its own: fixHalfEpisode does
		// ('[REPACK] Show.S01E06.5.x264'), and so does the structure stage
		// when it reads a name in a copy, which may be as long as what it
		// was given ('[REPACK] (Group) Title - 01'). So the tags are told
		// apart by where they are kept and not by how many there are. The
		// release was read from those tags, and its dates and unused words
		// are places in them, so they are the tags behind the flags
		if len(r.tags) != len(tags)-from || len(r.tags) != 0 && &r.tags[0] != &tags[from] {
			tags, end = slices.Concat(tags[:from], r.tags), r.end+from
			if from == k {
				// the build was given what the flags left of the delimiter
				// that closes them, and may have written onto it: the
				// bracket of a batch mark goes to the delimiters around it
				// ('[REPACK]([Batch]) Show')
				if rest := tags[k].Delim(); tags[k].Is(TagTypeDelim) && rest != d[cut:] {
					closer = NewTag(TagTypeDelim, nil, []byte(d[:cut]+rest), []byte(d[:cut]+rest))
				}
				tags[k] = closer
			}
		}
		var other []string
		for _, tag := range tags[:k] {
			if tag.Is(TagTypeOther) {
				other = appendOnce(other, tag.Other())
			}
		}
		for _, s := range r.Other {
			other = appendOnce(other, s)
		}
		r.tags, r.end, r.Other = tags, end, other
		for i := range r.dates {
			r.dates[i] += from
		}
		for i := range r.unused {
			r.unused[i] += from
		}
		if r.first != -1 {
			r.first += from
		}
		return r
	}
	r := &Release{
		tags: tags,
		end:  end,
		path: isPath(tags),
	}
	// initialize / fix tags
	b.init(r)
	// collect tags into release
	b.collect(r)
	// guess type. The type the caller gave replaces the guess, which is still
	// made: inspect restores a platform or an arch that was reset when it
	// finds no other sign of the type, and a hint that agrees with the guess
	// has to give the same release
	r.Type = b.packType(r, b.inspect(r, true))
	if Unknown < h.Type && h.Type <= Series {
		r.Type = h.Type
	}
	// an episode with no episode written is one by its date
	if r.Type == Episode && r.Numbering.Episodes == nil && r.Year != 0 && r.Month != 0 && r.Day != 0 {
		r.Numbering.Scheme = SchemeDaily
	}
	// special
	b.specialDate(r)
	// unset tags
	b.unset(r)
	// decide the words that are told from tags by the type
	b.settleVideo(r)
	b.settleGroupType(r)
	b.remuxSource(r)
	// read titles
	i := b.titles(r)
	// demarcate unused, and read the group
	b.unused(r, i)
	return *r
}

// leadingFlags finds the flags in brackets that open the name ('[REPACK]
// [Group] Show', '[PROPER][REPACK] Show'). It returns the position of the
// delimiter that closes the last of them, and how many bytes of that
// delimiter go with the flags: the bracket and what sets it apart from the
// rest ('] ' of '] (2019)'). It returns 0 when the name has no such flags, or
// nothing else.
func leadingFlags(tags []Tag, end int) (int, int) {
	if end < 3 || !tags[0].Is(TagTypeDelim) || tags[0].Delim() != "[" && tags[0].Delim() != "(" {
		return 0, 0
	}
	var k, cut int
	for i := 0; i+2 < end && tags[i+1].Is(TagTypeOther) && tags[i+2].Is(TagTypeDelim); i += 2 {
		d := tags[i+2].Delim()
		if d == "" || d[0] != ']' && d[0] != ')' {
			break
		}
		if info := tags[i+1].Info(); info == nil || !isReleaseFlag(info.Tag()) {
			break
		}
		k, cut = i+2, 1
		for cut < len(d) && d[cut] != '[' && d[cut] != '(' {
			cut++
		}
		// the next flag opens with the last byte of the delimiter
		if cut != len(d)-1 {
			break
		}
	}
	if k == 0 || cut == len(tags[k].Delim()) && k+1 >= end {
		return 0, 0
	}
	return k, cut
}

// init fixes the initial tag set.
func (b *TagBuilder) init(r *Release) {
	b.fixHalfEpisode(r)
	b.fixAudioRange(r)
	// read a name written the way a fansub group writes one by its brackets
	b.structure(r)
	// decide which of the words the lexers claimed are words of a title,
	// and whether what the group lexer guessed is the group
	b.settleGroup(r, b.settle(r))
	b.fixSourceUHD(r)
	b.fixMusic(r)
}

// fixHalfEpisode joins the half of an episode back onto it when a video tag
// follows, ie the '.5' in 'S01E06.5.WEB'. The series lexer hands a digit after
// a season and episode back when a dot and a letter follow it, since it is
// the first word of a title there ('S02E09.4.Days.Out'), and cannot tell a tag
// from a word. Only a tag that no later step turns back into a word counts,
// which a language or an other tag is not ('S01E06.3.French.Hens').
//
// The two tags between are left out of a copy of the tags, and their text is
// the end of the series tag's. The tags the caller gave are not changed.
func (b *TagBuilder) fixHalfEpisode(r *Release) {
	for i := 0; i+4 < r.end; i++ {
		tag := r.tags[i]
		if !tag.Is(TagTypeSeries) || len(tag.v) != 3 || tag.v[1] == "" || tag.v[2] == "" || !strings.HasSuffix(tag.v[0], tag.v[2]) {
			continue
		}
		dot, half, next := r.tags[i+1], r.tags[i+2], r.tags[i+3]
		if !dot.Is(TagTypeDelim) || dot.v[0] != "." || !half.Is(TagTypeText) || len(half.v[0]) != 1 || half.v[0][0] < '0' || half.v[0][0] > '9' ||
			!next.Is(TagTypeDelim) || next.v[0] != "." || !r.tags[i+4].Is(TagTypeResolution, TagTypeSource, TagTypeCodec, TagTypeHDR, TagTypeCollection) {
			continue
		}
		v := slices.Clone(tag.v)
		v[0] += "." + half.v[0]
		r.tags = slices.Delete(slices.Clone(r.tags), i+1, i+3)
		r.tags[i].v, r.end = v, r.end-2
	}
}

// fixAudioRange makes a range of numbers plain text when it is the bit depth
// and the sample rate of a record, ie the '24-96' of 'Artist - Album (2011)
// [FLAC 24-96]', which is 24 bit at 96 kHz and not the episodes 24 to 96. The
// first number is a bit depth and the second a sample rate in kHz ('16-44',
// '24-48', '24-192').
//
// A name with a resolution is a video, where the same numbers are the episodes
// of a batch ('[Grp] Show - 24-48 [1080p]'), unless the range follows an audio
// tag, as the quality of a concert does ('Artist - Live (2011) [1080p FLAC
// 24-96]'), in a bracket of its own or not ('[1080p FLAC (24-96)]'). A bracket
// that closes between the two sets the range apart from the audio ('[Grp]
// Show [BD 1080p FLAC] (24-48)').
func (b *TagBuilder) fixAudioRange(r *Release) {
	var scanned, video bool
	for i := 0; i < r.end; i++ {
		// the shape the episode range lexer gives
		if r.tags[i].typ != TagTypeSeries || len(r.tags[i].v) != 6 {
			continue
		}
		tag := r.tags[i]
		if tag.v[1] != "" || tag.v[2] != "" || tag.v[4] != "-" || !isBitDepth(tag.v[3]) || !isSampleRate(tag.v[5]) {
			continue
		}
		if !scanned {
			scanned, video = true, slices.ContainsFunc(r.tags, func(tag Tag) bool { return tag.Is(TagTypeResolution) })
		}
		if video && !(i >= 2 && r.tags[i-2].Is(TagTypeAudio) && r.tags[i-1].Is(TagTypeDelim) &&
			(r.tags[i-1].v[0] == " " || r.tags[i-1].v[0] == " (" || r.tags[i-1].v[0] == "(")) {
			continue
		}
		r.tags[i] = tag.As(TagTypeText, nil)
	}
}

// isBitDepth reports whether s is a bit depth that audio is sampled at.
func isBitDepth(s string) bool {
	return s == "16" || s == "24" || s == "32"
}

// isSampleRate reports whether s is a sample rate of audio in kHz, with its
// decimals cut off ('44' for 44.1 kHz, '88' for 88.2 kHz).
func isSampleRate(s string) bool {
	switch s {
	case "44", "48", "88", "96", "176", "192", "352", "384":
		return true
	}
	return false
}

// start finds the first text tag after i.
func (b *TagBuilder) start(r *Release, i int) int {
	for ; i < r.end && !r.tags[i].Is(TagTypeText); i++ {
	}
	return i
}

// fixSourceUHD fixes a bare uhd source tag next to a web source tag with only
// resolution, frame rate, hdr and bit depth tags between, as the uhd marks a
// web source and not a bluray (ie, 'Movie.2022.UHD.2160p.WEB...',
// 'Movie.2022.2160p.WEB.UHD...' or 'Event.2026.UHD.2160p50.HLG.WEB...'). The
// first of the pair is changed, as the first source tag is collected.
//
// Next to a tv source the uhd is the resolution of a broadcast (ie,
// 'Show.S01.UHD.HDTV...'), and the tv source is the one to collect.
func (b *TagBuilder) fixSourceUHD(r *Release) {
	for i := range r.end {
		if !r.tags[i].Is(TagTypeSource) || !strings.EqualFold(r.tags[i].Text(), "UHD") {
			continue
		}
		for _, inc := range []int{1, -1} {
			j, s := nextSource(r.tags[:r.end], i, inc)
			switch {
			case j == -1:
				continue
			case s == "WEB", s == "WEB-DL", s == "WEBRiP":
				k := min(i, j)
				r.tags[k] = NewTag(TagTypeSource, r.tags[i].f, []byte(r.tags[k].v[0]), []byte("UHD."+s))
			case strings.Contains(s, "TV"):
				r.tags[i] = r.tags[i].as(TagTypeResolution, b.resolutionf)
			default:
				continue
			}
			break
		}
	}
}

// fixMusic fixes music tags. Changes single cbr audio tag to the cbr container
// (ie, a comic) tag, changes 16bit arch tag to audio tag, and a single episode
// in a name with a music exclusive audio tag and no resolution to text.
func (b *TagBuilder) fixMusic(r *Release) {
	// when only one music tag of `cbr`, change to container `cbr` (comic)
	countCbr, posCbr, hasCbr, musicExcl := 0, 0, false, false
	// scanned is whether the tags after i were searched for a music exclusive
	// audio tag, which is done once: a later search would cover fewer tags
	var scanned bool
	for i := 0; i < r.end; i++ {
		// count audio tags and position of cbr tag
		if r.tags[i].Is(TagTypeAudio) {
			if r.tags[i].Audio() == "CBR" {
				hasCbr, posCbr = true, i
			}
			musicExcl = musicExcl || r.tags[i].InfoExcl()
			countCbr++
		}
		// fix single ep, and 16bit tag when music exclusive
		if singleEp, sbBit := r.tags[i].SingleEp(), (r.tags[i].Is(TagTypeArch) && r.tags[i].Arch() == "16bit"); singleEp || sbBit {
			for j := i + 1; !musicExcl && !scanned && j < r.end; j++ {
				musicExcl = musicExcl || (r.tags[j].Is(TagTypeAudio) && r.tags[j].InfoExcl())
			}
			scanned = true
			switch {
			// a record numbers its tracks and has no resolution, so a single
			// episode in a name with one is the episode ('[DSD] Show - 01
			// [1080p]')
			case musicExcl && singleEp && !slices.ContainsFunc(r.tags[:r.end], func(tag Tag) bool { return tag.Is(TagTypeResolution) }):
				r.tags[i] = r.tags[i].As(TagTypeText, nil)
			case musicExcl && sbBit:
				r.tags[i] = r.tags[i].as(TagTypeAudio, b.audiof)
			}
		}
	}
	// change single cbr audio tag to the cbr container tag
	if hasCbr && countCbr == 1 {
		r.tags[posCbr] = r.tags[posCbr].as(TagTypeContainer, b.containerf)
	}
}

// bareEpisode reports whether the tag at i is one episode written with no
// season: '04', '00', 'E04', 'EP04' or 'Episode 04'. A range ('01-12',
// 'E01E02', 'E05-E07') is not one episode, and half an episode ('21.5', 'Ep
// 5.5') has no absolute number.
func bareEpisode(tags []Tag, i int) bool {
	tag := tags[i]
	if !tag.Is(TagTypeSeries) || len(tag.v) != 3 || tag.v[1] != "" || tag.v[2] == "" || tag.fraction() != "" {
		return false
	}
	// one end of a range between two tags, the higher number last
	if peek(tags, i+1, TagTypeDelim) && tags[i+1].rangeDelim() && peek(tags, i+2, TagTypeSeries) && tags[i+2].v[1] == "" && atoi(tags[i+2].val(2)) > atoi(tag.v[2]) ||
		i >= 2 && tags[i-1].rangeDelim() && tags[i-2].Is(TagTypeSeries) && tags[i-2].v[1] == "" && atoi(tags[i-2].val(2)) < atoi(tag.v[2]) {
		return false
	}
	return !(peek(tags, i+1, TagTypeDelim) && tags[i+1].v[0] == "." &&
		peek(tags, i+2, TagTypeText) && tags[i+2].v[0] != "" && strings.Trim(tags[i+2].v[0], "0123456789") == "")
}

// specialEpisode reports whether the tag at i is an 'SP' that may mark a
// special and not the format of a record. It stands in front of one episode
// written with no season, with nothing but spaces, dots or underscores
// between ('Show SP Episode 00 Prologue'), or a slash and one more word ('Show
// SP/OVA Episode 00'). And the name up to it is a title: text with spaces,
// dots or underscores between, after a group in square brackets or not, and
// nothing that was read as a tag. A hyphen there sets an artist apart from an
// album ('Artist - Album SP Episode 1'), and in a bracket or between hyphens
// the 'SP' is the format ('Album (SP) Episode 1', 'Album - SP - 01').
func specialEpisode(tags []Tag, i int) bool {
	plain := func(k int) bool {
		return peek(tags, k, TagTypeDelim) && strings.Trim(tags[k].v[0], "._ ") == ""
	}
	if !tags[i].Is(TagTypeSource) || tags[i].v[0] != "SP" || !plain(i-1) || !peek(tags, i-2, TagTypeText) {
		return false
	}
	for k := i - 3; k >= 0; k-- {
		if !plain(k) && !tags[k].Is(TagTypeText, TagTypeWhitespace, TagTypeMeta) {
			return false
		}
	}
	k := i + 1
	if peek(tags, k, TagTypeDelim) && tags[k].v[0] == "/" && peek(tags, k+1, TagTypeText) {
		k += 2
	}
	return plain(k) && k+1 < len(tags) && bareEpisode(tags, k+1)
}

// appendOnce appends s to v unless v holds it already. A name that gives a
// value twice has it once, whether it writes the tag twice ('[Dual-Audio] ...
// [Dual-Audio]') or in two spellings ('[Eng] [English]').
func appendOnce(v []string, s string) []string {
	if slices.Contains(v, s) {
		return v
	}
	return append(v, s)
}

// collect collects tags into the release.
func (b *TagBuilder) collect(r *Release) {
	// the series tags, which are read together
	r.Numbering, r.numbered = r.numbering()
	for i := 0; i < len(r.tags); i++ {
		switch r.tags[i].typ {
		case TagTypeText:
		case TagTypePlatform:
			if r.Platform == "" {
				r.Platform = r.tags[i].Platform()
			}
		case TagTypeArch:
			if r.Arch == "" {
				r.Arch = r.tags[i].Arch()
			}
		case TagTypeSource:
			if s := r.tags[i].Source(); preciser(r.Source, s) {
				r.Source = s
			}
		case TagTypeResolution:
			if r.Resolution == "" {
				r.Resolution = r.tags[i].Resolution()
			}
		case TagTypeCollection:
			if r.Collection == "" {
				r.Collection = r.tags[i].Collection()
			}
		case TagTypeDate:
			r.Year, r.Month, r.Day = r.tags[i].Date()
			r.YearEnd = r.tags[i].yearEnd()
		case TagTypeVersion:
			if r.Version == "" {
				r.Version = r.tags[i].Version()
			}
		case TagTypeDisc:
			if r.Disc == "" {
				r.Disc = r.tags[i].Disc()
			}
		case TagTypeCodec:
			r.Codec = appendOnce(r.Codec, r.tags[i].Codec())
		case TagTypeHDR:
			r.DynamicRange |= ParseDynamicRange(r.tags[i].HDR())
		case TagTypeBitDepth:
			if r.BitDepth == "" {
				r.BitDepth = r.tags[i].BitDepth()
			}
		case TagTypeFrameRate:
			if r.FrameRate == "" {
				r.FrameRate = r.tags[i].FrameRate()
			}
		case TagTypeAudio:
			r.Audio = appendOnce(r.Audio, r.tags[i].Audio())
		case TagTypeChannels:
			if r.Channels == "" {
				r.Channels = r.tags[i].Channels()
			}
		case TagTypeOther:
			r.Other = appendOnce(r.Other, r.tags[i].Other())
		case TagTypeCut:
			r.Cut = appendOnce(r.Cut, r.tags[i].Cut())
		case TagTypeEdition:
			r.Edition = appendOnce(r.Edition, r.tags[i].Edition())
		case TagTypeLanguage:
			r.Language = appendOnce(r.Language, r.tags[i].Language())
		case TagTypeSize:
			if r.Size == "" {
				r.Size = r.tags[i].Size()
			}
		case TagTypeRegion:
			if r.Region == "" {
				r.Region = r.tags[i].Region()
			}
		case TagTypeContainer:
			if r.Container == "" {
				r.Container = r.tags[i].Container()
			}
		case TagTypeGenre:
			if r.Genre == "" {
				r.Genre = r.tags[i].Genre()
			}
		case TagTypeID:
			if r.ID == "" {
				r.ID = r.tags[i].ID()
			}
		case TagTypeMeta:
			switch k, v := r.tags[i].Meta(); {
			case k == "site" && r.Site == "":
				r.Site = v
			case k == "sum" && r.Sum == "":
				r.Sum = v
			case k == "pass" && r.Pass == "":
				r.Pass = v
			case k == "req":
				r.Req = true
			default:
				r.Meta = append(r.Meta, k+":"+v)
			}
		case TagTypeExt:
			r.Ext = r.tags[i].Ext()
		}
	}
	// collect year, month, day from unset date tags
	for i := len(r.dates); i > 0; i-- {
		year, month, day := r.tags[r.dates[i-1]].Date()
		if r.Year == 0 && year != 0 {
			r.Year, r.YearEnd = year, r.tags[r.dates[i-1]].yearEnd()
		}
		if r.Month == 0 && month != 0 {
			r.Month = month
		}
		if r.Day == 0 && day != 0 {
			r.Day = day
		}
	}
}

// inspect inspects the release, returning its expected type.
func (b *TagBuilder) inspect(r *Release, initial bool) Type {
	if r.Type != Unknown {
		return r.Type
	}
	n := len(r.tags)
	// inspect types
	// series is whether the name has a series tag
	// season is whether a season other than 0 was written
	// episode is whether a tag writes an episode in its first place, which
	// the range of a batch does not ('01-12'): 'S01E01-E03' is an episode
	// and '[Grp] Show - 01-12' a series. r.numbered is whether such an
	// episode is not 0, a second numbering aside ('S00E00.Episode.12' has
	// none)
	var app, series, season, episode, movie, video bool
	for _, tag := range r.tags {
		if tag.Is(TagTypeSeries) {
			series = true
			season = season || atoi(tag.val(1)) != 0
			episode = episode || tag.val(2) != ""
		}
		video = video || (!tag.Is(TagTypeGroup) && tag.InfoType() == Movie)
	}
	for i := n; i > 0; i-- {
		typ := r.tags[i-1].InfoType()
		app, movie = app || typ == App, movie || typ == Movie
		switch typ {
		case Book, Game:
			if typ == Game && r.tags[i-1].Is(TagTypeGroup, TagTypePlatform) &&
				video && (series || r.Resolution != "" || (r.Year != 0 && r.Month != 0 && r.Day != 0)) {
				continue
			}
			// peek for comic, education, magazine
			for j := i - 1; j > 0; j-- {
				if typ := r.tags[j-1].InfoType(); typ.Is(Comic, Education, Magazine) {
					return typ
				}
			}
			return typ
		case Series, Episode:
			if episode || !series && !slices.Contains(r.Other, "BOXSET") {
				return Episode
			}
			return Series
		case Education:
			if !season && !r.numbered {
				return Education
			}
		case Music:
			// a music tag in a name with an episode and video tags is a
			// word that happens to match ('(JA)', '[EP: 403]', 'SP'). With
			// a resolution in place of the episode, only the disc or
			// broadcast it came on still makes it music ('1080p.MBluRay'),
			// not a word ('Mixed.Team.Final.1080p')
			if series && r.Resolution != "" || video && (series || r.Resolution != "" && !r.tags[i-1].Is(TagTypeSource, TagTypeSize)) {
				continue
			}
			// the disc is then the only source: a music source beside
			// another one is a word of the title ('The.Cassette.2020.1080p.WEB',
			// 'The.Vinyl.2020.1080p.WEB')
			if r.Resolution != "" && r.tags[i-1].Is(TagTypeSource) && slices.ContainsFunc(r.tags, func(tag Tag) bool {
				return tag.Is(TagTypeSource) && tag.InfoType() != Music
			}) {
				continue
			}
			// and 'SP' between a title and an episode marks a special. An
			// audio tag or a year in the name is what a record has ('Album
			// SP Episode 1 FLAC')
			if len(r.Audio) == 0 && r.Year == 0 && specialEpisode(r.tags, i-1) {
				continue
			}
			// peek for audiobook
			for j := i - 1; j > 0; j-- {
				if typ := r.tags[j-1].InfoType(); typ.Is(Audiobook) {
					return typ
				}
			}
			return typ
		case Audiobook, Comic, Magazine:
			return typ
		}
		// exclusive tag not superseded by version/episode/date
		if r.tags[i-1].InfoExcl() &&
			r.Version == "" &&
			!season && !r.numbered &&
			r.Day == 0 && r.Month == 0 {
			return typ
		}
	}
	// check music style tag delimiters, which an episode with a resolution
	// has only by chance ('- 05 [720p] [ES-EN-DE]')
	for count, i := 0, n-1; i > 1 && !(series && r.Resolution != ""); i-- {
		if r.tags[i-1].Is(
			TagTypeDate,
			TagTypeCodec,
			TagTypeHDR,
			TagTypeBitDepth,
			TagTypeAudio,
			TagTypeResolution,
			TagTypeSource,
			TagTypeLanguage,
		) &&
			peek(r.tags, i-2, TagTypeDelim) && strings.HasSuffix(r.tags[i-2].Delim(), "-") &&
			peek(r.tags, i, TagTypeDelim) && strings.HasPrefix(r.tags[i].Delim(), "-") {
			count++
			if count > 1 {
				return Music
			}
		}
	}
	// defaults
	switch {
	case episode || (r.Year != 0 && r.Month != 0 && r.Day != 0):
		return Episode
	case series:
		return Series
	case app || (r.Version != "" && r.Resolution == "" && !movie):
		return App
	case movie || r.Resolution != "":
		return Movie
	case (r.Source == "" || r.Source == "WEB") && r.Resolution == "" && r.Year != 0:
		return Music
	}
	// with no other sign of a type, what the name runs on is one
	if initial && b.unsettle(r) {
		return b.inspect(r, false)
	}
	return Unknown
}

// specialDate handles special dates.
func (b *TagBuilder) specialDate(r *Release) {
	// on magazines, check prior to the date if there is a month listed
	if r.Type == Magazine && r.Year != 0 && r.Month == 0 && r.Day == 0 {
		// seek to prior text
		i := r.dates[0] - 1
		for ; i > 0 && r.tags[i].Is(TagTypeDelim); i-- {
		}
		// reset tag to date if tag is text, and parses as month name
		if i >= 0 && r.tags[i].Is(TagTypeText) {
			s := r.tags[i].Text()
			if t, err := time.Parse("January", s); err == nil {
				r.Month = int(t.Month())
				year, month := strconv.Itoa(r.Year), strconv.Itoa(r.Month)
				r.tags[i] = NewTag(TagTypeDate, nil, []byte(s), []byte(year), []byte(month), nil)
				r.dates = append(r.dates, i)
			}
		}
	}
}

// unset unsets exclusive and other misrecognized tags on the release.
func (b *TagBuilder) unset(r *Release) {
	movieSeriesEpisodeMusicGame, grabSource := r.Type.Is(Movie, Series, Episode, Music, Game), false
	for i := 0; i < len(r.tags); i++ {
		// if source had been previously reset, save it
		if grabSource && r.tags[i].Is(TagTypeSource) && r.Source == "" {
			r.Source = r.tags[i].Source()
		}
		ityp := r.tags[i].InfoType()
		// reset exclusive tags
		if ityp != r.Type && r.tags[i].Is(
			TagTypePlatform,
			TagTypeArch,
			TagTypeSource,
			TagTypeResolution,
			TagTypeCollection,
			TagTypeCodec,
			TagTypeHDR,
			TagTypeBitDepth,
			TagTypeAudio,
			TagTypeChannels,
			TagTypeOther,
			TagTypeCut,
			TagTypeEdition,
			TagTypeLanguage,
			TagTypeSize,
			TagTypeRegion,
			TagTypeContainer,
			TagTypeGenre,
			TagTypeExt,
		) && r.tags[i].InfoExcl() {
			switch typ, s := r.tags[i].TagType(), r.tags[i].Normalize(); {
			case typ == TagTypePlatform && r.Platform == s && !slices.Contains(r.Other, "Strategy.Guide"):
				r.Platform, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeArch && r.Arch == s:
				r.Arch, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeSource && r.Source == s:
				// the source is then the next one in the name
				r.Source, r.tags[i], grabSource = "", r.tags[i].As(TagTypeText, nil), true
			case typ == TagTypeResolution && r.Resolution == s:
				r.Resolution, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeCollection && r.Collection == s:
				r.Collection, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeCodec && slices.Contains(r.Codec, s):
				r.Codec, r.tags[i] = remove(r.Codec, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeHDR:
				// a tag that is none of the formats is in no bit, and is
				// still given back
				r.tags[i] = r.tags[i].As(TagTypeText, nil)
				r.DynamicRange = dropDynamicRange(r.DynamicRange, r.tags, ParseDynamicRange(s))
			case typ == TagTypeBitDepth && r.BitDepth == s:
				r.BitDepth, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeAudio && slices.Contains(r.Audio, s):
				r.Audio, r.tags[i] = remove(r.Audio, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeChannels && r.Channels == s:
				r.Channels, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeOther && slices.Contains(r.Other, s):
				r.Other, r.tags[i] = remove(r.Other, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeCut && slices.Contains(r.Cut, s):
				r.Cut, r.tags[i] = remove(r.Cut, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeEdition && slices.Contains(r.Edition, s):
				r.Edition, r.tags[i] = remove(r.Edition, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeLanguage && slices.Contains(r.Language, s):
				r.Language, r.tags[i] = remove(r.Language, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeSize && r.Size == s:
				r.Size, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeRegion && r.Region == s:
				r.Region, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeContainer && r.Container == s:
				r.Container, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeGenre && r.Genre == s:
				r.Genre, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeExt && r.Ext == s:
				r.Ext, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			}
		} else if !movieSeriesEpisodeMusicGame && r.tags[i].Is(TagTypeSource) && ityp.Is(Movie, Series, Episode) {
			// reset movie/series/episode source tags
			if r.Source == r.tags[i].Normalize() {
				r.Source = ""
			}
			r.tags[i], grabSource = r.tags[i].As(TagTypeText, nil), true
		} else if !movieSeriesEpisodeMusicGame && r.tags[i].Is(TagTypeChannels) {
			r.tags[i], r.Channels = r.tags[i].As(TagTypeText, nil), ""
		}
	}
}

// dropDynamicRange removes format f from d unless a hdr tag left in tags
// still gives it, as two tags can spell one format ('DV' and 'DoVi').
func dropDynamicRange(d DynamicRange, tags []Tag, f DynamicRange) DynamicRange {
	if f == 0 {
		return d
	}
	for _, tag := range tags {
		if tag.Is(TagTypeHDR) && ParseDynamicRange(tag.HDR()) == f {
			return d
		}
	}
	return d &^ f
}

// preciser reports whether source s replaces the source collected so far,
// which it does when there is none or when s is more precise than a CD or a
// DVD.
func preciser(cur, s string) bool {
	return cur == "" || cur == "CD" || (cur == "DVD" && s != "CD")
}

// isRemux reports whether s, a value of Other, is a remux in any case, as a
// caller's other row may spell it ('remux').
func isRemux(s string) bool {
	return strings.EqualFold(s, "REMUX")
}

// remuxSource gives a remux that names no source the disc it was taken from
// ('Movie 1981 1080p Remux AVC DTS-HD MA 2.0-GRP' is a BluRay, 'Movie 1954
// 2160p Remux DoVi HEVC-GRP' a UHD BluRay), as Radarr and Sonarr read a remux
// as a BluRay. The other disc with a 1080p video, the HD DVD, is named as one
// ('1080p HD-DVD Remux'), and a source written in the name is left as it is.
func (b *TagBuilder) remuxSource(r *Release) {
	if r.Source != "" || !r.Type.Is(Movie, Series, Episode) || !slices.ContainsFunc(r.Other, isRemux) {
		return
	}
	switch r.Resolution {
	case "1080p":
		r.Source = "BluRay"
	case "2160p":
		r.Source = "UHD.BluRay"
	}
}

// titles sets the titles for the release.
func (b *TagBuilder) titles(r *Release) int {
	var f func(*Release) int
	var aka bool
	switch r.Type {
	case Movie:
		f, aka = b.movieTitles, true
	case Series, Episode:
		f, aka = b.episodeTitles, true
	case Music:
		f = b.musicTitles
	case Book, Audiobook:
		f = b.bookTitles
	case App, Game:
		f = b.appTitle
	default:
		f = b.defaultTitle
	}
	i := f(r)
	if aka && len(r.Alt) == 0 {
		if title, alt := splitAKA(r.Title); len(alt) != 0 {
			r.Title, r.Alt = title, alt
		}
	}
	return i
}

// splitAKA splits a title on the words 'AKA' and 'aka' into the first name
// and the other names, ie 'A aka B aka C' gives 'A' and 'B', 'C'. A mixed
// case 'Aka' is a word of the title and does not split. Empty names are
// dropped. It returns no other names when there are not two names.
func splitAKA(s string) (string, []string) {
	if !strings.Contains(s, "AKA") && !strings.Contains(s, "aka") {
		return s, nil
	}
	var buf [4]string
	names, start := buf[:0], 0
	for i := 0; i < len(s); {
		end := strings.IndexByte(s[i:], ' ')
		if end == -1 {
			end = len(s)
		} else {
			end += i
		}
		if w := s[i:end]; w == "AKA" || w == "aka" {
			if name := strings.TrimSpace(s[start:i]); name != "" {
				names = append(names, name)
			}
			start = end
		}
		i = end + 1
	}
	if start == 0 {
		return s, nil
	}
	if name := strings.TrimSpace(s[start:]); name != "" {
		names = append(names, name)
	}
	if len(names) < 2 {
		return s, nil
	}
	// The clone keeps buf on the stack and sizes the result to the names.
	return names[0], slices.Clone(names[1:])
}

// movieTitles sets the titles for movies.
func (b *TagBuilder) movieTitles(r *Release) int {
	i, from, to := b.yearTitles(r)
	if from == to {
		return b.unitTitle(r, i)
	}
	// the subtitle is used. any text passed over in front of it is not
	for ; i < from; i++ {
		if r.tags[i].Is(TagTypeText) {
			r.unused = append(r.unused, i)
		}
	}
	return to
}

// yearTitles sets the title, and the subtitle from the text between a date
// and the resolution. Returns the position after the title, and the start and
// end of the subtitle, which are the same when there is none.
func (b *TagBuilder) yearTitles(r *Release) (int, int, int) {
	var pos int
	// seek to text
	for ; pos < len(r.tags) && !r.tags[pos].Is(TagTypeText); pos++ {
	}
	start := b.skipped(r, pos)
	var offset int
	r.Title, offset = b.firstTitle(r, start)
	// a year in parentheses with the title after it is a word of the title
	// ('(2001)A Space Odyssey(1961)', '(2001) A Space Odyssey (1968)',
	// '(2001).A.Space.Odyssey.(1968)')
	if offset == 1 && start+2 < len(r.tags) && r.tags[start].Retyped() && r.tags[start].Prev() == TagTypeDate &&
		r.tags[start+1].Is(TagTypeDelim) && strings.TrimRight(r.tags[start+1].Delim(), "._ ") == ")" && r.tags[start+2].Is(TagTypeText) {
		s, n := b.title(r.tags[start+2:], r.path, TagTypeText)
		r.Title, offset = r.Title+" "+s, offset+1+n
	}
	// seek date
	for pos = 0; pos < len(r.tags) && !r.tags[pos].Is(TagTypeDate); pos++ {
	}
	// check date position
	date := pos
	if date == len(r.tags) {
		return b.boxTitle(r, start, offset), 0, 0
	}
	// seek resolution
	for pos = 0; pos < len(r.tags) && !r.tags[pos].Is(TagTypeResolution); pos++ {
	}
	resolution := pos
	// with no resolution, the first source or codec after the date ends the
	// subtitle ('Event.2020.Final.A.vs.B.HDTV.x264-GRP')
	var noResolution bool
	if resolution == len(r.tags) {
		for pos = date + 1; pos < len(r.tags) && !r.tags[pos].Is(TagTypeSource, TagTypeCodec); pos++ {
		}
		resolution, noResolution = pos, true
	}
	if resolution == len(r.tags) {
		return b.boxTitle(r, start, offset), 0, 0
	}
	// determine if all tags between date and resolution are either text, delimiter, cut, edition
	hasSubtitle := date+1 < resolution-1
	if !hasSubtitle {
		return b.boxTitle(r, start, offset), 0, 0
	}
	// tags in front of the resolution end the subtitle, where tags inside it
	// rule it out ('Semi.Final.England.vs.Argentina.HLG.2160p'). not past an
	// episode, as the text in front of that is not its title. An F1
	// broadcaster ends it as the resolution does, leaving the words after it
	// unused ('Japan.Race.F1TV.Live.1080p')
	end := resolution
	for k := date + 1; k < resolution; k++ {
		if service(r.tags[k]) {
			end = k
			break
		}
	}
	for ; end > date+1 && !r.tags[end-1].Is(TagTypeText, TagTypeCut, TagTypeEdition, TagTypeSeries); end-- {
	}
	if end == date+1 || r.tags[end-1].Is(TagTypeSeries) {
		end = resolution - 1
	}
	end++
	for pos = date + 1; pos < len(r.tags) && hasSubtitle && pos < end; pos++ {
		hasSubtitle = hasSubtitle && r.tags[pos].Is(TagTypeDelim, TagTypeText, TagTypeCut, TagTypeEdition, TagTypeLanguage)
	}
	// capture subtitle
	var from, to int
	if hasSubtitle {
		var depth int
		for pos = date + 1; pos < len(r.tags) && !r.tags[pos].Is(TagTypeText, TagTypeCut, TagTypeEdition); pos++ {
			if r.tags[pos].Is(TagTypeDelim) {
				depth = bracketDepth(r.tags[pos].Delim(), depth)
			}
		}
		// a bracket that opens in front of the source or the other tags
		// holds tags ('(1999) [R2JP DVD-Remux]', '(2024) [English Dub] [CR
		// WEB-DL 1080p]'), and a bar opens another name for the title
		// ('(2019) | Other Name (BD 1080p)'). With the year in front of the
		// title, the text after the year is the title ('(2014) Title [BDRip
		// 1920x1080]')
		if pos < end-1 && pos >= start+offset && !((noResolution || end != resolution) && depth > 0) && r.tags[pos].v[0] != "|" {
			// numbers alone are not a subtitle, ie the rest of a date that
			// does not exist ('Show.2023.02.29.HDTV'), and neither are cut
			// and edition tags with no text beside them, which are in Cut
			// and Edition ('Movie.2013.Extended.Cut.1080p')
			if s, n := b.title(r.tags[pos:end-1], r.path, TagTypeText, TagTypeCut, TagTypeEdition); strings.ContainsFunc(s, unicode.IsLetter) &&
				slices.ContainsFunc(r.tags[pos:pos+n], func(tag Tag) bool { return tag.Is(TagTypeText) }) {
				r.Subtitle, from, to = s, pos, pos+n
			}
		}
	}
	// alternate subtitle delimiter
	if i := strings.LastIndexByte(r.Title, '~'); i != -1 && r.Subtitle == "" {
		r.Title, r.Subtitle = strings.TrimRightFunc(r.Title[:i], isTitleTrimDelim), strings.TrimLeftFunc(r.Title[i+1:], isTitleTrimDelim)
	}
	return min(start+offset, resolution), from, to
}

// boxTitle sets the titles for boxes.
func (b *TagBuilder) boxTitle(r *Release, start, offset int) int {
	n := start + offset
	if n == len(r.tags) || n <= 0 || r.Disc == "" || !r.tags[n].Is(TagTypeCut, TagTypeEdition) {
		return n
	}
	// check previous 4 words for 'the' and where NOT start of last text block
	for pos := n - 1; pos > start+1 && pos > n-8 && pos > 0; pos-- {
		if MustNormalize(r.tags[pos-1].Text()) == "the" {
			prefix, _ := b.title(r.tags[pos-1:n], r.path, TagTypeText)
			// consume remaining text, cut, edition
			suffix, offset := b.title(r.tags[n:], r.path, TagTypeText, TagTypeCut, TagTypeEdition)
			// reform title, subtitle
			r.Title = strings.TrimRightFunc(strings.TrimSuffix(r.Title, prefix), isTitleTrimDelim)
			r.Subtitle = prefix + " " + strings.TrimRightFunc(suffix, isBreakDelim)
			return n + offset
		}
	}
	return n
}

// episodeTitles sets the titles for episodes.
func (b *TagBuilder) episodeTitles(r *Release) int {
	// scan title
	pos, from, to := b.yearTitles(r)
	typ := TagTypeSeries
	if r.Month != 0 && r.Day != 0 {
		typ = TagTypeDate
	}
	// an episode that is an extra and nothing else has its title behind the
	// word that says so ('Title - ED1 - Starry Wish')
	if typ == TagTypeSeries && !slices.ContainsFunc(r.tags, func(tag Tag) bool { return tag.Is(TagTypeSeries) }) {
		return b.unitTitle(r, pos)
	}
	// seek text after date/series, collecting any skipped text
	for ; pos < len(r.tags) && !r.tags[pos].Is(typ); pos++ {
		if r.tags[pos].Is(TagTypeText) {
			r.unused = append(r.unused, pos)
		}
	}
	// kept is for when the subtitle stays the one yearTitles read. Its words
	// are used: they come off the unused text collected so far, and the
	// position returned is where the rest of the unused text begins, with
	// the text in front of the subtitle collected
	kept := func(pos int) int {
		if from == to {
			return pos
		}
		r.unused = slices.DeleteFunc(r.unused, func(i int) bool { return from <= i && i < to })
		if to <= pos {
			return pos
		}
		for ; pos < from; pos++ {
			if r.tags[pos].Is(TagTypeText) {
				r.unused = append(r.unused, pos)
			}
		}
		return to
	}
	if pos == len(r.tags) {
		return kept(pos)
	}
	// episode title must follow source, resolution, collection, date, series,
	// version, disc, other, cut, edition, language or container tags. it must
	// be before any codec, audio tag
	var depth int
	for pos++; pos < len(r.tags) && r.tags[pos].Is(
		TagTypeDelim,
		TagTypeSource,
		TagTypeResolution,
		TagTypeCollection,
		TagTypeDate,
		TagTypeSeries,
		TagTypeVersion,
		TagTypeDisc,
		TagTypeOther,
		TagTypeCut,
		TagTypeEdition,
		TagTypeLanguage,
		TagTypeContainer,
	); pos++ {
		if r.tags[pos].Is(TagTypeDelim) {
			depth = bracketDepth(r.tags[pos].Delim(), depth)
		}
	}
	// check at text tag
	if pos == len(r.tags) || !r.tags[pos].Is(TagTypeText) {
		return kept(pos)
	}
	// the first text after a '+' lists what else the release holds ('S01-04 +
	// OVAs', 'Season 01-25+Movies+Specials', 'S01 1080p + Specials'), and is
	// not a title
	if r.tags[pos-1].Is(TagTypeDelim) && strings.Contains(r.tags[pos-1].Delim(), "+") {
		return kept(pos)
	}
	// text inside a bracket that opened after the episode is a block of tags
	// or another name for the show ('S02E06 [English Dub]', '- 05 (BS11
	// 1080p)'), not the episode title, unless it has the scene layout of one
	// ('S01E02.(The.Title).1080p')
	if depth > 0 && !(depth == 1 && parenTitle(r.tags, pos)) {
		return kept(pos)
	}
	var offset int
	r.Subtitle, offset = b.title(r.tags[pos:], r.path, TagTypeText)
	// the title of a daily episode can name the season, which stays the
	// season and is also a word of the title ('Show.2024.05.06.Tournament.
	// Season.40.720p' is 'Tournament Season 40' of season 40). A range or a
	// list of seasons ('Seasons.1-2'), or a season with another series tag
	// after it ('Season.1.Episode.5'), is numbering and ends the title
	for typ == TagTypeDate && r.Subtitle != "" {
		j := pos + offset
		if j >= len(r.tags) || !r.tags[j].Is(TagTypeSeries) || r.tags[j].wroteEpisode() {
			break
		}
		v := r.tags[j].val(0)
		if !seasonWord(v) || v[len(v)-1] < '0' || v[len(v)-1] > '9' {
			break
		}
		k := j + 1
		for k < len(r.tags) && r.tags[k].Is(TagTypeDelim) {
			k++
		}
		if k < len(r.tags) && r.tags[k].Is(TagTypeSeries) {
			break
		}
		rest, n := b.title(r.tags[j+1:], r.path, TagTypeText)
		r.Subtitle += " " + strings.Map(func(c rune) rune {
			if c == '.' || c == '_' || c == '-' {
				return ' '
			}
			return c
		}, v)
		if offset = j + 1 - pos; !slices.ContainsFunc(r.tags[j+1:j+1+n], func(tag Tag) bool { return tag.Is(TagTypeText) }) {
			break
		}
		r.Subtitle, offset = r.Subtitle+" "+rest, j+1+n-pos
	}
	return pos + offset
}

// musicTitles sets the titles for music.
func (b *TagBuilder) musicTitles(r *Release) int {
	var i int
	r.Title, i = b.mixTitle(r, 0)
	// split artist, title
	for _, s := range []string{" - ", "--", "~", "-"} {
		if j := strings.LastIndex(r.Title, s); j != -1 {
			r.Artist, r.Title = strings.TrimRightFunc(r.Title[:j], isTitleTrimDelim), strings.TrimLeftFunc(r.Title[j+len(s):], isBreakDelim)
			break
		}
	}
	// Artist - Discography
	if r.Artist == "" && r.Title != "" && peek(r.tags[:r.end], i, TagTypeOther) && r.tags[i].Other() == "Discography" &&
		peek(r.tags, i-1, TagTypeDelim) && strings.Contains(r.tags[i-1].Delim(), "-") {
		r.Artist, r.Title, i = r.Title, r.tags[i].Text(), i+1
	}
	// check if date was skipped / at end
	i, skipped, ok := b.checkDate(r, i)
	if ok {
		s := r.tags[i].Delim()
		// Artist - (Prefix) Title
		if r.Artist == "" && strings.HasSuffix(s, "(") {
			title, z := b.mixTitle(r, i+1)
			var subtitle string
			subtitle, z = b.mixTitle(r, z+1)
			if title != "" && subtitle != "" {
				r.Artist, r.Title = r.Title, "("+title+") "+subtitle
				if i, skipped, ok = b.checkDate(r, z); !ok {
					return i
				}
			}
		}
		// (Artist) - Title
		if r.Artist == "" && (skipped || strings.HasPrefix(s, ")")) {
			if title, z := b.mixTitle(r, i+1); title != "" {
				r.Artist, r.Title, i = r.Title, title, z
			}
		}
		// capture subtitle after '(', '__', '-', or '~'
		if r.Subtitle == "" &&
			(strings.HasSuffix(s, "(") || s == "__" || strings.ContainsAny(s, "-~")) &&
			peek(r.tags[:r.end], i+1, TagTypeText) {
			r.Subtitle, i = b.mixTitle(r, i+1)
		}
	}
	if r.Subtitle == "" && r.Artist != "" {
		// try to split artist
		for _, s := range []string{" - ", "--", "~"} {
			if j := strings.LastIndex(r.Artist, s); j != -1 {
				r.Artist, r.Title, r.Subtitle = strings.TrimRightFunc(r.Artist[:j], isTitleTrimDelim), strings.TrimLeftFunc(r.Artist[j+len(s):], isBreakDelim), r.Title
				break
			}
		}
	}
	return i
}

// mixTitle returns the mix title.
func (b *TagBuilder) mixTitle(r *Release, i int) (string, int) {
	start := min(b.start(r, i), len(r.tags))
	for i = start; i < r.end && r.tags[i].Is(TagTypeDelim, TagTypeText, TagTypeOther); i++ {
		if r.tags[i].Is(TagTypeOther) && r.tags[i].Other() != "REMiX" {
			break
		}
	}
	title, offset := b.title(r.tags[start:i], r.path, TagTypeText, TagTypeOther)
	return title, start + offset
}

// checkDate checks if i is at end of tags, and skips a date tag if present.
func (b *TagBuilder) checkDate(r *Release, i int) (int, bool, bool) {
	// bail if at end
	if i == r.end {
		return i, false, false
	}
	// skip a date in middle, such as Artist (2003) - Title
	var skipped bool
	if r.tags[i].Is(TagTypeDate) {
		i, skipped = i+1, true
	}
	// bail if not a delim
	if i == r.end || !r.tags[i].Is(TagTypeDelim) {
		return i, skipped, false
	}
	return i, skipped, true
}

// bookTitles sets the titles for books.
func (b *TagBuilder) bookTitles(r *Release) int {
	var s string
	var pos, offset int
	for ; pos < len(r.tags); pos += offset {
		// seek to text
		for ; pos < len(r.tags) && !r.tags[pos].Is(TagTypeText, TagTypePlatform, TagTypeArch, TagTypeOther, TagTypeRegion); pos++ {
		}
		if pos == len(r.tags) {
			break
		}
		switch isOther := r.tags[pos].Is(TagTypeOther); {
		case isOther && r.tags[pos].InfoType() != Book:
			offset = 1
			continue
		case isOther && r.tags[pos].Other() == "Strategy.Guide":
			s, offset = strings.ReplaceAll(r.tags[pos].Text(), ".", " "), 2
		default:
			s, offset = b.title(r.tags[pos:], r.path, TagTypeText, TagTypePlatform, TagTypeArch, TagTypeRegion)
		}
		if r.Title != "" && s != "" {
			r.Title += " "
		}
		r.Title += s
	}
	if i := strings.LastIndexByte(r.Title, ';'); i != -1 {
		r.Title, r.Subtitle = strings.TrimRightFunc(r.Title[:i], isTitleTrimDelim), strings.TrimLeftFunc(r.Title[i+1:], isTitleTrimDelim)
	}
	if r.Artist == "" {
		for _, s := range []string{" - ", "--", "~"} {
			if before, after, ok := strings.Cut(r.Title, s); ok {
				r.Artist, r.Title = strings.TrimRightFunc(before, isTitleTrimDelim), strings.TrimLeftFunc(after, isBreakDelim)
				break
			}
		}
	}
	if r.Subtitle == "" {
		for _, s := range []string{" - ", "--", "~"} {
			if before, after, ok := strings.Cut(r.Title, s); ok {
				r.Title, r.Subtitle = strings.TrimRightFunc(before, isBreakDelim), strings.TrimLeftFunc(after, isTitleTrimDelim)
				break
			}
		}
	}
	if i := strings.LastIndexByte(r.Title, '-'); r.Artist == "" && i != -1 {
		artist, title := strings.TrimRightFunc(r.Title[:i], isTitleTrimDelim), strings.TrimLeftFunc(r.Title[i+1:], isBreakDelim)
		if !b.digsuf.MatchString(artist) && !b.digpre.MatchString(title) {
			r.Artist, r.Title = artist, title
		}
	}
	return pos
}

// appTitle sets the an app title.
func (b *TagBuilder) appTitle(r *Release) int {
	// seek to text
	var pos int
	for pos = 0; pos < len(r.tags) && !r.tags[pos].Is(TagTypeText, TagTypeDate); pos++ {
	}
	var offset int
	r.Title, offset = b.title(r.tags[pos:], r.path, TagTypeText, TagTypeDate)
	return pos + offset
}

// defaultTitle sets the default title.
func (b *TagBuilder) defaultTitle(r *Release) int {
	// seek to text
	var pos int
	for pos = 0; pos < len(r.tags) && !r.tags[pos].Is(TagTypeText); pos++ {
	}
	pos = b.skipped(r, pos)
	var offset int
	r.Title, offset = b.firstTitle(r, pos)
	return pos + offset
}

// isPath reports whether the name in tags is a file path, where a slash
// separates folders. A file name cannot hold a slash, so a name is a path when
// it ends in a file extension ('Show/Show.S01E06.1080p.BluRay.x265-Grp.mkv'),
// or when the folder in front of its last slash has spaces and the name after
// it has none ('Some Show/Some.Show.S01E01.1080p.WEB.x264-GRP'): a release
// name does not change how it writes its spaces at a slash. In a release name
// the slash is part of the title ('Fate/Zero S01 1080p', 'Face/Off 1997').
func isPath(tags []Tag) bool {
	for _, tag := range slices.Backward(tags) {
		if !tag.Is(TagTypeWhitespace) {
			if tag.Is(TagTypeExt) {
				return true
			}
			break
		}
	}
	// the spaces that set a meta tag apart from the name are not spaces
	// between its words ('[Group] Fate/Zero.S01E01.1080p' is no path, and
	// 'Some Show/Some.Show.S01E01.1080p [ABCD1234]' is one)
	space := func(tags []Tag) bool {
		for i, tag := range tags {
			if !tag.Is(TagTypeDelim, TagTypeWhitespace) {
				continue
			}
			d := tag.v[0]
			if peek(tags, i-1, TagTypeMeta) {
				d = strings.TrimLeft(d, "\t\n\f\r ")
			}
			if peek(tags, i+1, TagTypeMeta) {
				d = strings.TrimRight(d, "\t\n\f\r ")
			}
			if strings.ContainsRune(d, ' ') {
				return true
			}
		}
		return false
	}
	for i, tag := range slices.Backward(tags) {
		if tag.Is(TagTypeDelim) && strings.ContainsRune(tag.v[0], '/') {
			return space(tags[:i]) && !space(tags[i+1:])
		}
	}
	return false
}

// title finds all text in tags, stopping when encountering a non-space
// delimiter, returning the title and ending position. path is whether the
// name is a file path, where a slash separates folders.
func (b *TagBuilder) title(tags []Tag, path bool, types ...TagType) (string, int) {
	return b.titleOf(tags, path, false, types...)
}

// titleOf is title for a name that sets its words apart with spaces or not.
// decimal says that it does, so that a dot between two numbers is the point
// of a decimal and no separator ('Evangelion 3.33 You Can (Not) Redo').
func (b *TagBuilder) titleOf(tags []Tag, path, decimal bool, types ...TagType) (string, int) {
	var v []string
	var i int
loop:
	for ; i < len(tags); i++ {
		switch {
		case tags[i].Is(types...):
			v = append(v, tags[i].TextReplace(".", " ", -1))
		case tags[i].Is(TagTypeDelim):
			s := tags[i].Delim()
			// the dot of a file extension is no part of the name, nor of
			// the delimiter in front of it ('Title..mkv' reads as 'Title.')
			if peek(tags, i+1, TagTypeExt) {
				s = strings.TrimSuffix(s, ".")
			}
			switch {
			case s == "/" && !path && i != 0 && tags[i-1].Is(types...) && !tags[i-1].Was(TagTypeSource) && i+1 < len(tags) && tags[i+1].Is(types...):
				// a slash between two words is part of the title of a release
				// ('Fate/strange Fake S01', 'Face/Off 1997', 'AC/DC - Album'),
				// and is kept as written. In a path it separates folders ('Show/Show.S01E06.mkv'), one
				// with spaces separates the title from another name ('Show /
				// Other Name S01'), and one after a special read back as text
				// lists another label for it ('Show SP/OVA Episode 00'), so
				// those end the title.
				v = append(v, "/")
			case decimal && s == "." && i != 0 && i+1 < len(tags) && tags[i-1].Is(types...) && tags[i+1].Is(types...) &&
				allDigits(tags[i-1].v[0], 4) && allDigits(tags[i+1].v[0], 4):
				v = append(v, ".")
			case !strings.ContainsAny(s, "()[]{}\\/") && s != "__":
				v = append(v, b.delim(s, tags, i, types...))
			default:
				break loop
			}
		default:
			break loop
		}
	}
	// each rewrite below is skipped when a scan shows its pattern cannot
	// match, which is the usual case: a regexp replace copies the string twice
	// even when it changes nothing.
	s := strings.Join(v, "")
	// acronyms missing periods
	if hasAcronym(s) {
		s = b.missing.ReplaceAllStringFunc(s, func(a string) string {
			return strings.TrimLeft(strings.ReplaceAll(strings.TrimSpace(a), " ", "."), ". ") + ". "
		})
	}
	// fix oopsie
	s = strings.ReplaceAll(s, ". .", ". ")
	// acronymns on single letters
	if hasBadAcronym(s) {
		s = b.bad.ReplaceAllStringFunc(s, func(a string) string {
			return b.fix.ReplaceAllString(a, "$1")
		})
	}
	// collapse spaces
	s = b.collapse(s)
	// collapse ellipsis
	if strings.Contains(s, "....") {
		s = b.ellips.ReplaceAllString(s, "...")
	}
	// unescape entities
	s = html.UnescapeString(s)
	if strings.Count(s, "+") > 1 {
		s = strings.ReplaceAll(s, "+", " ")
	}
	// trim
	return strings.TrimFunc(s, isTitleTrimDelim), i
}

// parenTitle reports whether the text at i is an episode title written in
// parentheses in front of the tags, ie 'Show.S01E02.(The.Title).1080p.WEB'.
// The episode is written with its season or as a date, the parentheses hold
// nothing that was read as a tag, and a tag follows them outside any bracket.
// A name that opens with a bracket or numbers its episode with no season is
// not read that way, as there the parentheses hold another name for the show
// ('[Group] Show - 09 (Other Name) 1080p').
func parenTitle(tags []Tag, i int) bool {
	var first int
	for ; first < i && tags[first].Is(TagTypeWhitespace); first++ {
	}
	if tags[first].Is(TagTypeMeta) || tags[first].Is(TagTypeDelim) && strings.ContainsAny(tags[first].v[0], "[({") {
		return false
	}
	if !strings.HasSuffix(tags[i-1].v[0], "(") {
		return false
	}
	// the episode in front
	ep := i - 1
	for ; ep > 0 && !tags[ep].Is(TagTypeSeries, TagTypeDate); ep-- {
	}
	if tags[ep].Is(TagTypeSeries) && (tags[ep].v[1] == "" || !tags[ep].wroteEpisode()) {
		return false
	}
	// seek the closing parenthesis
	var letter bool
	for ; i < len(tags) && !(tags[i].Is(TagTypeDelim) && strings.ContainsAny(tags[i].v[0], "()[]{}")); i++ {
		// a word that was read as a tag is not a word of a title, which the
		// year of '(2022 Airing)' is when another year follows
		if !tags[i].Is(TagTypeDelim) && (!tags[i].Is(TagTypeText) || tags[i].Retyped()) {
			return false
		}
		letter = letter || tags[i].Is(TagTypeText) && strings.ContainsFunc(tags[i].v[0], unicode.IsLetter)
	}
	// a tag follows, set off from the parenthesis
	if !letter || i >= len(tags)-1 {
		return false
	}
	if d := tags[i].v[0]; len(d) < 2 || d[0] != ')' || strings.ContainsAny(d[1:], "()[]{}") {
		return false
	}
	return !tags[i+1].Is(TagTypeText, TagTypeDelim, TagTypeWhitespace, TagTypeMeta, TagTypeGroup, TagTypeExt)
}

// bracketDepth returns how many brackets are open after delim, when depth were
// open before it. A closing bracket with none open is ignored.
func bracketDepth(delim string, depth int) int {
	for i := 0; i < len(delim); i++ {
		switch delim[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth = max(depth-1, 0)
		}
	}
	return depth
}

// unused sets the unused text on the release, and reads the group and the
// checksum that may be among it.
func (b *TagBuilder) unused(r *Release, i int) {
	// collect
	for ; i < len(r.tags); i++ {
		if r.tags[i].Is(TagTypeText) {
			r.unused = append(r.unused, i)
		}
	}
	// the group, which may be the last of the words left over
	b.group(r)
	// the last word left over is the checksum of the file when it reads as
	// one, and group left it. The word in front of a group that was the last
	// word is not looked at
	if n := len(r.unused); n != 0 && r.groupFrom != groupWord && r.Sum == "" {
		if s := r.tags[r.unused[n-1]].Text(); b.checksum(s) {
			r.Sum, r.unused = s, r.unused[:n-1]
		}
	}
	// a checksum in its own bracket with more of the name after it, ie
	// '[Group] Show - 01 [1080p] [ABCD1234] (Alt Title)'
	for k := len(r.unused) - 1; r.Sum == "" && k >= 0; k-- {
		j := r.unused[k]
		if s := r.tags[j].Text(); b.checksum(s) &&
			peek(r.tags, j-1, TagTypeDelim) && strings.HasSuffix(r.tags[j-1].Delim(), "[") &&
			peek(r.tags, j+1, TagTypeDelim) && strings.HasPrefix(r.tags[j+1].Delim(), "]") {
			r.Sum, r.unused = s, slices.Delete(r.unused, k, k+1)
		}
	}
}

// checksum reports whether s reads as the checksum of a file: eight hex
// digits, one of them a number ('ABCD1234', not 'DEADBEEF').
func (b *TagBuilder) checksum(s string) bool {
	return b.sum.MatchString(s) && strings.ContainsAny(s, "0123456789")
}

// delim fixes the delimiter based on surrounding tags.
func (b *TagBuilder) delim(delim string, tags []Tag, i int, types ...TagType) string {
	// special cases
	switch delim {
	case "...":
		return "..."
	case "..", ". ":
		return ". "
	case "":
		return " "
	}
	// convert to spaces, collapse
	s := b.collapse(strings.Map(func(r rune) rune {
		switch r {
		case '-', '+', ',', '.', '~':
			return r
		case '\t', '\n', '\f', '\r', ' ', '_':
			return ' '
		}
		return -1
	}, delim))
	// bail if last tag or not a period
	if s != "." || i == len(tags)-1 {
		return b.collapse(strings.Map(func(r rune) rune {
			if r == '.' {
				return ' '
			}
			return r
		}, s))
	}
	// get ante, prev, next
	var ante, prev, next string
	if i > 2 && tags[i-2].Is(TagTypeDelim) {
		ante = tags[i-2].Delim()
	}
	if i != 0 && tags[i-1].Is(types...) {
		prev = tags[i-1].Text()
	}
	if i < len(tags)-1 && tags[i+1].Is(types...) {
		next = tags[i+1].Text()
	}
	// acronyms / abbreviations
	if isUpperLetter([]rune(prev)) && isUpperLetter([]rune(next)) && !strings.ContainsAny(ante, "-~") {
		return "."
	}
	// version, such as 2.0 or 5.1 (book title)
	if isDigit([]rune(prev)) && isDigit([]rune(next)) && !strings.ContainsAny(ante, "-~") {
		return "."
	}
	return " "
}

// collapse replaces each run of whitespace in s with a single space.
func (b *TagBuilder) collapse(s string) string {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\t', '\n', '\f', '\r':
			return b.spaces.ReplaceAllString(s, " ")
		case ' ':
			if i+1 < len(s) && s[i+1] == ' ' {
				return b.spaces.ReplaceAllString(s, " ")
			}
		}
	}
	return s
}

// hasAcronym reports whether s holds an upper case letter, a period or space,
// and another upper case letter in a row, which every match of missing starts
// with.
func hasAcronym(s string) bool {
	for i := 0; i+2 < len(s); i++ {
		if (s[i+1] == '.' || s[i+1] == ' ') && s[i] >= 'A' && s[i] <= 'Z' && s[i+2] >= 'A' && s[i+2] <= 'Z' {
			return true
		}
	}
	return false
}

// hasBadAcronym reports whether s holds a hyphen, period or space, an upper
// case letter and a period in a row, which every match of bad contains.
func hasBadAcronym(s string) bool {
	for i := 1; i+2 < len(s); i++ {
		if s[i+2] == '.' && s[i+1] >= 'A' && s[i+1] <= 'Z' && (s[i] == '-' || s[i] == '.' || s[i] == ' ') {
			return true
		}
	}
	return false
}

// isUpperLetter determines if s is all upper case.
func isUpperLetter(r []rune) bool {
	switch len(r) {
	case 0:
		return true
	case 1:
		return unicode.IsUpper(r[0])
	}
	return false
}

// isDigit determines if s is a digit.
func isDigit(r []rune) bool {
	switch len(r) {
	case 1:
		return unicode.IsDigit(r[0])
	}
	return false
}

// remove removes str from v.
func remove(v []string, str string) []string {
	var s []string
	for _, t := range v {
		if str == t {
			continue
		}
		s = append(s, t)
	}
	return s
}

// peek determines if i is of type.
func peek(tags []Tag, i int, types ...TagType) bool {
	return 0 <= i && i < len(tags) && tags[i].Is(types...)
}

// nextSource returns the index and value of the source tag next to i in the
// direction of inc, when only whitespace, delimiters and resolution, frame
// rate, hdr and bit depth tags are between them, or -1.
func nextSource(tags []Tag, i, inc int) (int, string) {
	for i += inc; 0 <= i && i < len(tags); i += inc {
		if tags[i].Is(TagTypeWhitespace, TagTypeDelim, TagTypeResolution, TagTypeFrameRate, TagTypeHDR, TagTypeBitDepth) {
			continue
		}
		if tags[i].Is(TagTypeSource) {
			return i, tags[i].Source()
		}
		break
	}
	return -1, ""
}
