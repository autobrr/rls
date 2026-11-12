package rls

import (
	"bytes"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/autobrr/rls/reutil"
	"github.com/autobrr/rls/taginfo"
)

// TagParser is a release tag parser.
type TagParser struct {
	builder  Builder
	ellip    []byte
	once     []LexFunc
	multi    []LexFunc
	notFirst []bool
	// isDelim is delim as a byte table: every delimiter is a single ASCII
	// byte, so membership is all the regexp ever decided.
	isDelim [256]bool
}

// NewTagParser creates a new release tag parser.
func NewTagParser(infos map[string][]*taginfo.Taginfo, lexers ...Lexer) Parser {
	// delims
	var v []string
	for r := rune(0); r < 255; r++ {
		if isAnyDelim(r) {
			v = append(v, string(r))
		}
	}
	delim := regexp.MustCompile(`^((?:` + reutil.Join(true, v...) + ")+)")
	// build short tags
	short := make(map[string]bool)
	hdr := strings.ToLower(TagTypeHDR.String())
	for typ, v := range infos {
		if typ == hdr {
			continue
		}
		for _, info := range v {
			for _, field := range strings.FieldsFunc(info.Tag(), isAnyDelim) {
				if len(field) < 5 && !strings.Contains(field, "$") {
					short[strings.ToUpper(field)] = true
				}
			}
		}
	}
	// separate once and multi
	var once, multi []LexFunc
	var notFirst []bool
	for _, lexer := range lexers {
		if f, o, nf := lexer.Initialize(infos, delim, short); o {
			once = append(once, f)
		} else {
			multi = append(multi, f)
			notFirst = append(notFirst, nf)
		}
	}
	// init builder
	builder := DefaultBuilder
	if b, ok := builder.(interface {
		Init(map[string][]*taginfo.Taginfo) Builder
	}); ok {
		builder = b.Init(infos)
	}
	p := &TagParser{
		builder:  builder,
		ellip:    []byte("..."),
		once:     once,
		multi:    multi,
		notFirst: notFirst,
	}
	for c := 0; c < 255; c++ {
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
	return p.parseHint(bytes.Clone(src), h)
}

// parseHint is ParseHint for a name that is the parser's to keep.
func (p *TagParser) parseHint(src []byte, h Hint) Release {
	tags, end := p.parse(src)
	if h.DayFirst {
		dayFirst(tags)
	}
	var r Release
	if b, ok := p.builder.(*TagBuilder); ok {
		r = b.build(tags, end, h)
	} else {
		r = p.builder.Build(tags, end)
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
	p.builder = builder
}

// Parse parses tags in buf.
func (p *TagParser) Parse(src []byte) ([]Tag, int) {
	return p.parse(bytes.Clone(src))
}

// parse parses the tags of src, which is the parser's to keep: the tags hold
// pieces of it and not copies. So it has to be bytes that nothing else holds
// and nothing writes to from here on, which a copy of what the caller gave
// is. The lexers of this package only read it. The tags keep it and the
// working copy alive, and nothing of the caller's.
func (p *TagParser) parse(src []byte) ([]Tag, int) {
	// working buf
	buf := make([]byte, len(src))
	for i, c := range src {
		switch c {
		case '_', ',', '+':
			c = ' '
		}
		buf[i] = c
	}
	i, n := 0, len(buf)
	// a name yields roughly one tag per three bytes; sizing for one per two
	// lets start grow once instead of doubling its way up from nothing
	start, end := make([]Tag, 0, len(src)/2+8), []Tag(nil)
	// once
	for _, f := range p.once {
		start, end, i, n, _ = f(src, buf, start, end, i, n)
	}
	// consume
	notFirst := false
	for i < n {
		start, end, i, _ = p.next(src, buf, start, end, i, n, &notFirst)
	}
	// add end (reversed) to start
	tags := start
	for i := len(end); i > 0; i-- {
		tags = append(tags, end[i-1])
	}
	return tags, len(tags) - len(end)
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
func (p *TagParser) next(src, buf []byte, start, end []Tag, i, n int, notFirst *bool) ([]Tag, []Tag, int, int) {
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
	// run all lexers
	startn := len(start)
	for a, f := range p.multi {
		if p.notFirst[a] && !*notFirst {
			continue
		}
		// a match has to consume input from one end or the other. One that
		// does not would be offered the same input again without end, so it
		// is taken as no match and the next lexer is tried
		if s, e, j, k, ok := f(src, buf, start, end, i, n); ok && i <= j && j <= k && k <= n && (i < j || k < n) {
			*notFirst = *notFirst || startn != len(s)
			return s, e, j, k
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
	return p.builder.Build(p.Parse(src))
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
	containerf taginfo.FindFunc
	// audiof is the audio find func.
	audiof taginfo.FindFunc
	// resolutionf is the resolution find func.
	resolutionf taginfo.FindFunc
	// languagef is the language find func.
	languagef taginfo.FindFunc
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
		containerf:  findFunc(infos["container"]...),
		audiof:      findFunc(infos["audio"]...),
		resolutionf: findFunc(infos["resolution"]...),
		languagef:   findFunc(infos["language"]...),
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
	r := &Release{
		tags: tags,
		end:  end,
	}
	// initialize / fix tags
	b.init(r)
	b.fixTailGroup(r)
	// collect tags into release
	b.collect(r)
	// guess type. The type the caller gave replaces the guess, which is still
	// made: inspect restores a platform or an arch that was reset when it
	// finds no other sign of the type, and a hint that agrees with the guess
	// has to give the same release
	r.Type = b.inspect(r, true)
	if Unknown < h.Type && h.Type <= Series {
		r.Type = h.Type
	}
	// special
	b.specialDate(r)
	// unset tags
	b.unset(r)
	// read titles
	i := b.titles(r)
	// fansub style names carry the release group in a leading bracket, which is
	// captured as a site meta tag. surface it as the group when no group was
	// otherwise found. done before unused so the last-unused-text fallback in
	// unused does not claim an unrelated token as the group.
	b.leadingGroup(r)
	// demarcate unused
	b.unused(r, i)
	b.trailingGroup(r)
	return *r
}

// leadingGroup sets the group from a site meta tag appearing in a leading
// bracket (ie, '[SubsPlease] Show - 04 ...'), when no group was found.
func (b *TagBuilder) leadingGroup(r *Release) {
	if r.Group != "" || r.Site == "" {
		return
	}
	for i := 0; i < len(r.tags); i++ {
		switch {
		case r.tags[i].Is(TagTypeWhitespace, TagTypeDelim):
			continue
		case r.tags[i].Is(TagTypeMeta):
			if k, v := r.tags[i].Meta(); k == "site" && v == r.Site {
				r.Group = v
			}
		}
		return
	}
}

// trailingGroup sets the group from a site in a bracket that closes the name
// (ie, 'Show - 04 [1080p][Group].mkv'), when nothing else gave a group. It
// runs last, so that a group named in the tags is preferred ('(BD 1080p -
// Encoder)[Uploader]').
func (b *TagBuilder) trailingGroup(r *Release) {
	if r.Group != "" || r.Site == "" {
		return
	}
	for i := len(r.tags) - 1; i >= 0; i-- {
		switch {
		case r.tags[i].Is(TagTypeWhitespace, TagTypeDelim, TagTypeExt):
			continue
		case r.tags[i].Is(TagTypeMeta):
			// a site given by name ('[[site:name]]', '-={name}=-') is not
			// a group
			if k, v := r.tags[i].Meta(); k == "site" && v == r.Site {
				if s := strings.TrimSpace(r.tags[i].v[0]); strings.HasPrefix(s, "[") && !strings.HasPrefix(s, "[[") {
					r.Group = v
				}
				return
			}
			continue
		}
		return
	}
}

// fixTailGroup hands a guessed group back to the name when the name opens
// with its group in a bracket. The group lexer takes whatever follows the last
// hyphen, which is a group after a tag ('[TGx] Movie.1080p.x264-GRP'). It is
// the end of the title after a word ('[Commie] Hataraku Maou-sama!'). After a
// hyphen set off by spaces it is a note after an edition ('[Group] Movie -
// 30th Anniversary - Patch Subs'), the title of the episode after a series
// tag ('[Group] Show Episode 00 - Prologue'), and a language when the word
// names one ('[Group] Movie (BD 1080p) - VOSTFR'), which it also is right
// after another language ('VF-VOSTFR').
func (b *TagBuilder) fixTailGroup(r *Release) {
	i := 0
	for ; i < len(r.tags) && r.tags[i].Is(TagTypeWhitespace, TagTypeDelim); i++ {
	}
	if i == len(r.tags) || !r.tags[i].Is(TagTypeMeta) {
		return
	}
	if k, _ := r.tags[i].Meta(); k != "site" {
		return
	}
	for g := i + 2; g < len(r.tags); g++ {
		if !r.tags[g].Is(TagTypeGroup) || r.tags[g].f != nil || !r.tags[g-1].Is(TagTypeDelim) || r.tags[g-1].v[0] != "-" {
			continue
		}
		p := g - 2
		for ; p > i && r.tags[p].Is(TagTypeWhitespace, TagTypeDelim); p-- {
		}
		// the group tag holds the spaces after the hyphen, which go to the
		// delimiter for the title to read 'Chaos - Prologue'
		s, grp := r.tags[g].v[0], r.tags[g].v[1]
		d := "-" + s[:len(s)-len(strings.TrimLeft(s, " \t_"))]
		spaced := len(d) > 1 && p != g-2 && strings.HasSuffix(r.tags[g-2].v[0], " ")
		var tag Tag
		switch {
		case r.tags[p].Is(TagTypeText), spaced && r.tags[p].Is(TagTypeSeries, TagTypeEdition):
			tag = NewTag(TagTypeText, nil, []byte(s[len(d)-1:]), []byte(grp))
		case spaced || r.tags[p].Is(TagTypeLanguage):
			// a builder that was not initialized has no language rows
			if b.languagef == nil {
				continue
			}
			// not a row that takes any word with 'sub' or 'dub' in it
			if info := b.languagef(grp); info == nil || strings.Contains(info.Tag(), "$") {
				continue
			}
			tag = NewTag(TagTypeLanguage, b.languagef, []byte(s[len(d)-1:]), []byte(grp))
		default:
			continue
		}
		r.tags[g-1], r.tags[g] = NewTag(TagTypeDelim, nil, []byte(d), []byte(d)), tag
		if r.end == g-1 {
			r.end = g + 1
		}
	}
}

// init fixes the initial tag set.
func (b *TagBuilder) init(r *Release) {
	b.fixHalfEpisode(r)
	b.fixFirstDate(r)
	// determine earliest pivot
	m, pivot := b.pivots(r, TagTypeDate, TagTypeSource, TagTypeSeries, TagTypeResolution, TagTypeVersion)
	date, series := m[TagTypeDate], m[TagTypeSeries]
	// reset/collect dates
	if date != -1 {
		r.dates = append(r.dates, date)
	}
	if dates := b.reset(r, date, TagTypeDate); len(dates) != 0 {
		r.dates = append(r.dates, dates...)
	}
	// fix special tags
	if date != -1 || series != -1 {
		i := min(date, series)
		switch {
		case date == -1:
			i = series
		case series == -1:
			i = date
		}
		b.fixSpecial(r, i, series != -1)
		b.fixEpisodeTitle(r, i)
	}
	// get first text prior to pivot
	end := b.end(r, pivot)
	// reset language/other/arch/platform prior to end
	_ = b.reset(r, end, TagTypeLanguage, TagTypeArch, TagTypePlatform)
	b.fixFirst(r)
	b.fixSourceWeb(r)
	b.fixSourceUHD(r)
	start := b.start(r, 0)
	b.fixBad(r, start, end)
	b.fixNoText(r, end)
	b.fixIsolated(r)
	b.fixWords(r)
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

// fixFirstDate fixes the special case of a date occuring before text, and
// there are no other date tags.
func (b *TagBuilder) fixFirstDate(r *Release) {
	i, last := 0, -1
	// seek last
	for i = r.end; i > 0; i-- {
		if r.tags[i-1].Is(TagTypeDate) {
			last = i - 1
			break
		}
	}
	if last == -1 { // not found
		return
	}
	// seek first
	for i = 0; i < r.end && r.tags[i].Is(TagTypeWhitespace, TagTypeDelim); i++ {
	}
	switch {
	case i < last,
		peek(r.tags, i-1, TagTypeDelim) && strings.HasSuffix(r.tags[i-1].Delim(), "("):
	case r.tags[i].Is(TagTypeDate):
		r.tags[i] = r.tags[i].As(TagTypeText, nil)
	}
}

// pivots finds the last position for the specified tags, generating a map of
// the types and earliest position of all.
func (b *TagBuilder) pivots(r *Release, types ...TagType) (map[TagType]int, int) {
	m := make(map[TagType]int, len(types))
	for _, typ := range types {
		m[typ] = -1
	}
	j := -1
	// find minimum of types
	for i := r.end; i > 0; i-- {
		if typ := r.tags[i-1].TagType(); typ.Is(types...) && m[typ] == -1 {
			m[typ], j = i-1, i-1
		}
	}
	if j != -1 {
		return m, j
	}
	return m, r.end
}

// reset any tags of types prior to i.
func (b *TagBuilder) reset(r *Release, i int, types ...TagType) []int {
	// reset date tag and collect indexes
	var v []int
	for ; i > 0; i-- {
		if r.tags[i-1].Is(types...) {
			r.tags[i-1], v = r.tags[i-1].As(TagTypeText, nil), append(v, i-1)
		}
	}
	return v
}

// start finds the first text tag after i.
func (b *TagBuilder) start(r *Release, i int) int {
	for ; i < r.end && !r.tags[i].Is(TagTypeText); i++ {
	}
	return i
}

// end finds the first text tag before i.
func (b *TagBuilder) end(r *Release, i int) int {
	for ; i > 0 && !r.tags[i-1].Is(TagTypeText); i-- {
	}
	return i
}

// fixFirst fixes the first non text tag if it was badly matched.
func (b *TagBuilder) fixFirst(r *Release) {
	var i int
	// seek. meta is skipped along with whitespace and delimiters: a leading
	// bracket holds the site or the fansub group, never part of the title, and
	// stopping on it left the first real title token unchecked (ie,
	// '[Group] Complete Manual - 04' kept COMPLETE as an other tag).
	for ; i < r.end && r.tags[i].Is(TagTypeWhitespace, TagTypeDelim, TagTypeMeta); i++ {
	}
	// a tag that opens a bracket of tags is a tag of the release, and the
	// title comes after the bracket ('[Group][1080p] Title', '[BD 1080p] Title
	// [Group]'). with nothing but words after it the bracket holds the title
	// ('[Group] [Complete Manual] - 04')
	var bracket bool
	if i != r.end && i != 0 && r.tags[i-1].Is(TagTypeDelim) {
		d := r.tags[i-1].v[0]
		bracket = d != "" && strings.IndexByte("[({", d[len(d)-1]) != -1
	}
	if bracket {
		var words, tags int
		for j := i + 1; j < r.end && !(r.tags[j].Is(TagTypeDelim) && strings.ContainsAny(r.tags[j].v[0], "])}")); j++ {
			switch {
			case r.tags[j].Is(TagTypeText):
				words++
			case !r.tags[j].Is(TagTypeDelim, TagTypeWhitespace):
				tags++
			}
		}
		bracket = words == 0 || tags != 0
	}
	if !bracket && i != r.end && r.tags[i].Is(
		TagTypePlatform,
		TagTypeArch,
		TagTypeSource,
		TagTypeResolution,
		TagTypeCodec,
		TagTypeHDR,
		TagTypeBitDepth,
		TagTypeAudio,
		TagTypeOther,
		TagTypeCut,
		TagTypeEdition,
		TagTypeLanguage,
		TagTypeRegion,
	) {
		r.tags[i] = r.tags[i].As(TagTypeText, nil)
	}
	// a genre written as a bare word opens the title ('DOC.Martin.S01E01'),
	// where one in parentheses is the genre ('(Comedy) Title')
	if i != r.end && r.tags[i].Is(TagTypeGenre) && !strings.HasSuffix(r.tags[i].v[0], ")") {
		r.tags[i] = r.tags[i].As(TagTypeText, nil)
	}
}

// fixBad fixes bad collection/language/other/arch/platform tags before i.
func (b *TagBuilder) fixBad(r *Release, start, i int) {
	// seek non language/edition/collection/cut/other/source/delim
	for ; i > start &&
		r.tags[i-1].Is(
			TagTypeLanguage,
			TagTypeEdition,
			TagTypeCut,
			TagTypeOther,
			TagTypeCollection,
			TagTypeDelim,
			TagTypeSource,
		); i-- {
	}
	// fix collection/language/other/arch/platform tag between start and i
	for ; i > start; i-- {
		switch {
		case r.tags[i-1].Is(TagTypeCollection) && r.tags[i-1].Collection() == "IMAX",
			r.tags[i-1].Is(TagTypeOther) && r.tags[i-1].Other() == "REMiX":
			// ignore imax
		case r.tags[i-1].Is(
			TagTypeCollection,
			TagTypeLanguage,
			TagTypeOther,
			TagTypeArch,
			TagTypePlatform,
		):
			r.tags[i-1] = r.tags[i-1].As(TagTypeText, nil)
		}
	}
}

// fixSpecial fixes special collection and other tags before i.
func (b *TagBuilder) fixSpecial(r *Release, i int, series bool) {
	for ; i > 0; i-- {
		typ, c, o, t := r.tags[i-1].TagType(), r.tags[i-1].Collection(), r.tags[i-1].Other(), r.tags[i-1].Text()
		switch l := strings.ToLower(t); {
		case typ == TagTypeCollection && (c == "CC" || c == "RED" || (c == "AMZN" && l == "amazon")),
			typ == TagTypeSource && t == "Web",
			typ == TagTypeCut && t == "Uncut",
			typ == TagTypeOther && (o == "MD" || o == "RESTORATiON"),
			typ == TagTypeCut && l == "dc",
			series && (typ == TagTypeArch || typ == TagTypePlatform):
			r.tags[i-1] = r.tags[i-1].As(TagTypeText, nil)
		}
	}
}

// fixEpisodeTitle fixes language, collection and region tags inside the title
// of an episode, ie 'NO' in 'Show.S01E02.NO.WAY.OUT.1080p'. The title runs
// from the series tag, or a date with a day, at i up to the first source,
// resolution or codec tag. A tag in it is a word of the title when it stands
// between the series tag or text and two more words of text, with nothing but
// plain delimiters around them. That leaves alone a tag followed by other
// tags ('Show.S01E02.GERMAN.1080p'), one that follows other tags
// ('German.Dubbed.DL.x'), one with a single word after it ('English Dub'),
// and one inside brackets ('[English Dub]').
//
// After a year alone the text is the name of an event, and only the tag right
// after the year is fixed, ie 'Hungarian' in 'Formula1.2026.Hungarian.Grand.
// Prix.1080p'. Words further in have text on both sides, which fixIsolated
// handles. A 'MULTi' is left alone ('2024.MULTI.Audio.Sub').
func (b *TagBuilder) fixEpisodeTitle(r *Release, i int) {
	var year bool
	if r.tags[i].Is(TagTypeDate) {
		_, month, day := r.tags[i].Date()
		year = month == 0 || day == 0
	}
	plain := func(k int) bool {
		return r.tags[k].Is(TagTypeDelim) && !strings.ContainsAny(r.tags[k].v[0], "[](){}")
	}
	// prev is the tag before the one at k, across a delimiter
	for k, prev := i+2, i; k+4 < r.end && !r.tags[k].Is(TagTypeSource, TagTypeResolution, TagTypeCodec); k++ {
		if r.tags[k].Is(TagTypeDelim, TagTypeWhitespace) {
			continue
		}
		if year && (prev != i || strings.HasPrefix(r.tags[k].Language(), "MULTi")) {
			break
		}
		if r.tags[k].Is(TagTypeLanguage, TagTypeCollection, TagTypeRegion) && prev == k-2 && plain(k-1) &&
			(prev == i || r.tags[prev].Is(TagTypeText)) &&
			plain(k+1) && r.tags[k+2].Is(TagTypeText) && plain(k+3) && r.tags[k+4].Is(TagTypeText) {
			r.tags[k] = r.tags[k].As(TagTypeText, nil)
		}
		prev = k
	}
}

// fixSourceWeb fixes a bare web source tag preceded by text when any source
// tag follows, as a release names its source once amongst the technical tags:
// a preceding web tag is part of a title (ie,
// 'Show.S03E03.A.Dark.Web.1080p.WEBRip...').
func (b *TagBuilder) fixSourceWeb(r *Release) {
	for i := 0; i < r.end; i++ {
		if !r.tags[i].Is(TagTypeSource) || r.tags[i].Source() != "WEB" || !isolated(r.tags[:r.end], i, -1) {
			continue
		}
		// seek source after i
		for j := i + 1; j < r.end; j++ {
			if r.tags[j].Is(TagTypeSource) {
				r.tags[i] = r.tags[i].As(TagTypeText, nil)
				break
			}
		}
	}
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
				r.tags[i] = r.tags[i].As(TagTypeResolution, b.resolutionf)
			default:
				continue
			}
			break
		}
	}
}

// fixNoText fixes having no text but has a collection tag.
func (b *TagBuilder) fixNoText(r *Release, end int) {
	// bail if any text tags
	i, n := 0, min(end+1, len(r.tags))
	for ; i < n; i++ {
		if r.tags[i].Is(TagTypeText) {
			return
		}
	}
	// reset
	for i = 0; i < n; i++ {
		if r.tags[i].Is(TagTypeCollection) {
			r.tags[i] = r.tags[i].As(TagTypeText, nil)
		}
	}
}

// fixIsolated fixes isolated collection/language/other/arch/platform tags.
func (b *TagBuilder) fixIsolated(r *Release) {
	// if tag is isolated with text on both sides, change to text
	for i := r.end - 1; i > 1; i-- {
		// a bare 'dual' is the audio tag next to other tags or a bracket, and
		// a word between two words ('Freestyle.Dual.Moguls')
		dual := r.tags[i-1].Is(TagTypeAudio) && strings.EqualFold(r.tags[i-1].v[0], "dual") &&
			r.tags[i-2].Is(TagTypeDelim) && strings.Trim(r.tags[i-2].Delim(), "._ ") == "" &&
			r.tags[i].Is(TagTypeDelim) && strings.Trim(r.tags[i].Delim(), "._ ") == ""
		if (dual || r.tags[i-1].Is(
			TagTypeCollection,
			TagTypeLanguage,
			TagTypeOther,
			TagTypeArch,
			TagTypePlatform,
			// region is deliberately absent. Adding it fixes 'ATP.Tour.US.Open'
			// but breaks '[UK version]' and 'Videogame EUR NSW', which are
			// structurally identical and genuinely regions. A region between two
			// words is not separable from a title containing a region name
			// without knowing where the title ends.
		)) &&
			isolated(r.tags[:r.end], i-1, -1) &&
			isolated(r.tags[:r.end], i-1, +1) &&
			r.tags[i-1].Other() != "REMiX" {
			r.tags[i-1] = r.tags[i-1].As(TagTypeText, nil)
		}
	}
}

// fixWords fixes tags that are words of a name, in one pass:
//
// A region between two words with plain delimiters around it, outside any
// bracket, ie the 'US' in 'Tennis.US.Open.2025'. The words must have been
// read as text, and not be a tag that was reset to text, which keeps the
// region in 'Videogame EUR NSW'. A note in brackets names the region it means
// ('(Contains the UK dub)').
//
// A region, language or platform on either side of a 'vs', which is one of
// the two sides of a match, ie the 'USA' in 'Canada.Vs.USA.1080p' or the 'PSV'
// in 'Ajax.vs.PSV.720p'.
//
// A bare 'extended' in front of 'highlights', where it names the programme and
// not a cut of it, ie 'Arsenal.vs.Liverpool.Extended.Highlights.1080p'.
func (b *TagBuilder) fixWords(r *Release) {
	plain := func(i int) bool {
		return r.tags[i].typ == TagTypeDelim && strings.Trim(r.tags[i].v[0], "._ ") == ""
	}
	word := func(i int) bool {
		return r.tags[i].typ == TagTypeText && !r.tags[i].Retyped()
	}
	side := func(i int) bool {
		switch r.tags[i].typ {
		case TagTypeText, TagTypeRegion, TagTypeLanguage, TagTypePlatform:
			return true
		}
		return false
	}
	var depth int
	for i := 0; i < r.end-2; i++ {
		switch r.tags[i].typ {
		case TagTypeDelim:
			depth = bracketDepth(r.tags[i].v[0], depth)
		case TagTypeRegion:
			if i >= 2 && depth == 0 && plain(i-1) && plain(i+1) && word(i-2) && word(i+2) {
				r.tags[i] = r.tags[i].As(TagTypeText, nil)
			}
		case TagTypeText:
			if i < 2 || !word(i) || !isVersus(r.tags[i].v[0]) || !plain(i-1) || !plain(i+1) || !side(i-2) || !side(i+2) {
				continue
			}
			for _, k := range [...]int{i - 2, i + 2} {
				if r.tags[k].typ != TagTypeText {
					r.tags[k] = r.tags[k].As(TagTypeText, nil)
				}
			}
		case TagTypeCut:
			if strings.EqualFold(r.tags[i].v[0], "extended") && plain(i+1) &&
				r.tags[i+2].Is(TagTypeText, TagTypeOther) && strings.EqualFold(r.tags[i+2].v[0], "highlights") {
				r.tags[i] = r.tags[i].As(TagTypeText, nil)
			}
		}
	}
}

// isVersus reports whether s is a word that stands between the two sides of a
// match.
func isVersus(s string) bool {
	switch len(s) {
	case 1:
		return s == "v" || s == "V" || s == "@"
	case 2:
		return strings.EqualFold(s, "vs") || strings.EqualFold(s, "at")
	case 5:
		return strings.EqualFold(s, "gegen")
	case 6:
		return strings.EqualFold(s, "contre")
	}
	return false
}

// fixMusic fixes music tags. Changes single cbr audio tag to the cbr container
// (ie, a comic) tag, changes 16bit arch tag to audio tag, and converts
// 'bootleg' tag not surrounded by '-' or '()' back to text.
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
		// reset bootleg tag that is not '-bootleg-' or '(bootleg)'
		if i != 0 && r.tags[i].Is(TagTypeOther) && r.tags[i].Other() == "BOOTLEG" {
			wrapped := (peek(r.tags, i-1, TagTypeDelim) && strings.HasSuffix(r.tags[i-1].Delim(), "-") &&
				peek(r.tags, i+1, TagTypeDelim) && strings.HasPrefix(r.tags[i+1].Delim(), "-")) ||
				(peek(r.tags, i-1, TagTypeDelim) && strings.HasSuffix(r.tags[i-1].Delim(), "(") &&
					peek(r.tags, i+1, TagTypeDelim) && strings.HasPrefix(r.tags[i+1].Delim(), ")"))
			if !wrapped {
				r.tags[i] = r.tags[i].As(TagTypeText, nil)
			}
		}
		// fix single ep, and 16bit tag when music exclusive
		if singleEp, sbBit := r.tags[i].SingleEp(), (r.tags[i].Is(TagTypeArch) && r.tags[i].Arch() == "16bit"); singleEp || sbBit {
			for j := i + 1; !musicExcl && !scanned && j < r.end; j++ {
				musicExcl = musicExcl || (r.tags[j].Is(TagTypeAudio) && r.tags[j].InfoExcl())
			}
			scanned = true
			switch {
			case musicExcl && singleEp:
				r.tags[i] = r.tags[i].As(TagTypeText, nil)
			case musicExcl && sbBit:
				r.tags[i] = r.tags[i].As(TagTypeAudio, b.audiof)
			}
		}
	}
	// change single cbr audio tag to the cbr container tag
	if hasCbr && countCbr == 1 {
		r.tags[posCbr] = r.tags[posCbr].As(TagTypeContainer, b.containerf)
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
	// bareEp is whether an episode was written with no season, ie the '04' in
	// 'Show - 04 (1080p)', 'Show E04' or 'Show Episode 04', and bare the first
	// such episode. season is whether a season was written anywhere in the
	// name, as a number ('S02E04', '2nd Season') or as the specials ('S00E04')
	// seasoned is whether a tag before the one at hand wrote a season
	var bareEp, season, seasoned bool
	var bare int
	for i := 0; i < len(r.tags); i++ {
		// the first one that is not 0, as Episode is
		if (!bareEp || bare == 0) && bareEpisode(r.tags, i) {
			bareEp, bare = true, atoi(r.tags[i].v[2])
		}
		season = season || r.tags[i].Is(TagTypeSeries) && r.tags[i].v[1] != ""
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
			// stomping allowed when more precise source available
			if s := r.tags[i].Source(); r.Source == "" || r.Source == "CD" || (r.Source == "DVD" && s != "CD") {
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
		case TagTypeSeries:
			// the first season the name writes, which season 0 is
			series, episode := r.tags[i].Series()
			if !seasoned {
				r.Series, seasoned = series, r.tags[i].v[1] != ""
			}
			if r.Episode == 0 {
				r.Episode = episode
			}
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
			r.HDR = appendOnce(r.HDR, r.tags[i].HDR())
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
		case TagTypeGroup:
			r.Group = r.tags[i].Group()
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
	// a bare number is an absolute episode only when nothing qualified it: a
	// season marker anywhere makes it season relative by fansub convention
	// ('Hell Mode S2 - 04' is episode 4 of season 2, not absolute 4).
	if bareEp && !season {
		r.absolute, r.isAbsolute = bare, true
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
	// episode is whether an episode was written, which r.Episode cannot say
	// for episode 0 ('Show - 00', 'S01E00')
	// season is whether a season other than 0 was written, which r.Series
	// cannot say when season 0 is written first ('S00E02 (S02E00)')
	var app, series, season, episode, movie, video bool
	for _, tag := range r.tags {
		series = series || tag.Is(TagTypeSeries)
		season = season || tag.Is(TagTypeSeries) && len(tag.v) > 1 && atoi(tag.v[1]) != 0
		episode = episode || tag.Is(TagTypeSeries) && len(tag.v) > 2 && tag.v[2] != ""
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
			if r.Episode != 0 || episode || (r.Series == 0 && r.Episode == 0 && !series) && !contains(r.Other, "BOXSET") {
				return Episode
			}
			return Series
		case Education:
			if !season && r.Episode == 0 {
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
			!season && r.Episode == 0 &&
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
	case r.Episode != 0 || episode || (r.Year != 0 && r.Month != 0 && r.Day != 0):
		return Episode
	case r.Series != 0 || series:
		return Series
	case app || (r.Version != "" && r.Resolution == "" && !movie):
		return App
	case movie || r.Resolution != "":
		return Movie
	case (r.Source == "" || r.Source == "WEB") && r.Resolution == "" && r.Year != 0:
		return Music
	}
	// check for platform, arch tags previously reset
	if initial {
		var reinspect bool
		for i := n; i > 0; i-- {
			if r.tags[i-1].Was(TagTypePlatform, TagTypeArch) {
				r.tags[i-1], reinspect = r.tags[i-1].Revert(), true
				switch r.tags[i-1].TagType() {
				case TagTypePlatform:
					r.Platform = r.tags[i-1].Platform()
				case TagTypeArch:
					r.Arch = r.tags[i-1].Arch()
				}
			}
		}
		if reinspect {
			return b.inspect(r, false)
		}
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
			TagTypeGroup,
			TagTypeExt,
		) && r.tags[i].InfoExcl() {
			switch typ, s := r.tags[i].TagType(), r.tags[i].Normalize(); {
			case typ == TagTypePlatform && r.Platform == s && !contains(r.Other, "Strategy.Guide"):
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
			case typ == TagTypeCodec && contains(r.Codec, s):
				r.Codec, r.tags[i] = remove(r.Codec, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeHDR && contains(r.HDR, s):
				r.HDR, r.tags[i] = remove(r.HDR, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeBitDepth && r.BitDepth == s:
				r.BitDepth, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeAudio && contains(r.Audio, s):
				r.Audio, r.tags[i] = remove(r.Audio, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeChannels && r.Channels == s:
				r.Channels, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeOther && contains(r.Other, s):
				r.Other, r.tags[i] = remove(r.Other, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeCut && contains(r.Cut, s):
				r.Cut, r.tags[i] = remove(r.Cut, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeEdition && contains(r.Edition, s):
				r.Edition, r.tags[i] = remove(r.Edition, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeLanguage && contains(r.Language, s):
				r.Language, r.tags[i] = remove(r.Language, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeSize && r.Size == s:
				r.Size, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeRegion && r.Region == s:
				r.Region, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeContainer && r.Container == s:
				r.Container, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeGenre && r.Genre == s:
				r.Genre, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeGroup && r.Group == s:
				r.Group, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
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
	if aka {
		if s := strings.Split(r.Title, " AKA "); r.Alt == "" && len(s) == 2 && s[0] != "" && s[1] != "" {
			r.Title, r.Alt = s[0], s[1]
		}
	}
	return i
}

// movieTitles sets the titles for movies.
func (b *TagBuilder) movieTitles(r *Release) int {
	i, from, to := b.yearTitles(r)
	if from == to {
		return i
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
	start := pos
	var offset int
	r.Title, offset = b.title(r.tags[start:], TagTypeText)
	// a year in parentheses with the title after it is a word of the title
	// ('(2001)A Space Odyssey(1961)', '(2001) A Space Odyssey (1968)',
	// '(2001).A.Space.Odyssey.(1968)')
	if offset == 1 && start+2 < len(r.tags) && r.tags[start].Retyped() && r.tags[start].Prev() == TagTypeDate &&
		r.tags[start+1].Is(TagTypeDelim) && strings.TrimRight(r.tags[start+1].Delim(), "._ ") == ")" && r.tags[start+2].Is(TagTypeText) {
		s, n := b.title(r.tags[start+2:], TagTypeText)
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
	// episode, as the text in front of that is not its title
	end := resolution
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
		// ('(2019) | Other Name (BD 1080p)')
		if pos < end-1 && !((noResolution || end != resolution) && depth > 0) && r.tags[pos].v[0] != "|" {
			// numbers alone are not a subtitle, ie the rest of a date that
			// does not exist ('Show.2023.02.29.HDTV'), and neither are cut
			// and edition tags with no text beside them, which are in Cut
			// and Edition ('Movie.2013.Extended.Cut.1080p')
			if s, n := b.title(r.tags[pos:end-1], TagTypeText, TagTypeCut, TagTypeEdition); strings.ContainsFunc(s, unicode.IsLetter) &&
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
			prefix, _ := b.title(r.tags[pos-1:n], TagTypeText)
			// consume remaining text, cut, edition
			suffix, offset := b.title(r.tags[n:], TagTypeText, TagTypeCut, TagTypeEdition)
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
	r.Subtitle, offset = b.title(r.tags[pos:], TagTypeText)
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
	title, offset := b.title(r.tags[start:i], TagTypeText, TagTypeOther)
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
			s, offset = b.title(r.tags[pos:], TagTypeText, TagTypePlatform, TagTypeArch, TagTypeRegion)
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
			if i := strings.Index(r.Title, s); i != -1 {
				r.Artist, r.Title = strings.TrimRightFunc(r.Title[:i], isTitleTrimDelim), strings.TrimLeftFunc(r.Title[i+len(s):], isBreakDelim)
				break
			}
		}
	}
	if r.Subtitle == "" {
		for _, s := range []string{" - ", "--", "~"} {
			if i := strings.Index(r.Title, s); i != -1 {
				r.Title, r.Subtitle = strings.TrimRightFunc(r.Title[:i], isBreakDelim), strings.TrimLeftFunc(r.Title[i+len(s):], isTitleTrimDelim)
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
	r.Title, offset = b.title(r.tags[pos:], TagTypeText, TagTypeDate)
	return pos + offset
}

// defaultTitle sets the default title.
func (b *TagBuilder) defaultTitle(r *Release) int {
	// seek to text
	var pos int
	for pos = 0; pos < len(r.tags) && !r.tags[pos].Is(TagTypeText); pos++ {
	}
	var offset int
	r.Title, offset = b.title(r.tags[pos:], TagTypeText)
	return pos + offset
}

// title finds all text in tags, stopping when encountering a non-space
// delimiter, returning the title and ending position.
func (b *TagBuilder) title(tags []Tag, types ...TagType) (string, int) {
	var v []string
	var i int
loop:
	for ; i < len(tags); i++ {
		switch {
		case tags[i].Is(types...):
			v = append(v, tags[i].TextReplace(".", " ", -1))
		case tags[i].Is(TagTypeDelim):
			if s := tags[i].Delim(); !strings.ContainsAny(s, "()[]{}\\/") && s != "__" {
				v = append(v, b.delim(s, tags, i, types...))
			} else {
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

// beforeTag reports whether a tag follows the text at i inside the bracket
// that holds it, ie the 'HEVC2' in '(1280x720 HEVC2 AAC)' or the 'Conan' in
// '(Meitantei Conan, Multi-Subs)'. Such a word is not the group, which a block
// of tags names last ('(1080p x265 AAC Group)').
func beforeTag(tags []Tag, i int) bool {
	var tagged bool
	for i++; i < len(tags); i++ {
		switch {
		case !tags[i].Is(TagTypeDelim):
			tagged = tagged || !tags[i].Is(TagTypeText, TagTypeWhitespace)
		case strings.ContainsAny(tags[i].Delim(), "([{"):
			return false
		case strings.ContainsAny(tags[i].Delim(), ")]}"):
			return tagged
		}
	}
	return false
}

// plusList reports whether the text at i is in a list that a '+' joins on,
// ie the 'Specials' in 'S01 + Specials' and in 'S01 + Movies & Specials', or
// the 'Movie' in '[720p] + Movie'. Such a word is not the group. A '+' with no
// space beside it counts only in a name that separates its words some other
// way, which 'Artist+-+Album+(ripper)' does not.
func plusList(tags []Tag, i int) bool {
	for i--; i >= 0 && tags[i].Is(TagTypeText, TagTypeDelim); i-- {
		if !tags[i].Is(TagTypeDelim) {
			continue
		}
		// what follows the last bracket
		d := tags[i].Delim()
		k := strings.LastIndexAny(d, "()[]{}")
		switch s := d[k+1:]; {
		case strings.TrimSpace(s) != "+":
		case s != "+":
			return true
		default:
			return slices.ContainsFunc(tags, func(tag Tag) bool {
				return tag.Is(TagTypeDelim) && (tag.Delim() == " " || tag.Delim() == ".")
			})
		}
		if k != -1 {
			return false
		}
	}
	return false
}

// unused sets the unused text on the release.
func (b *TagBuilder) unused(r *Release, i int) {
	// collect
	for ; i < len(r.tags); i++ {
		if r.tags[i].Is(TagTypeText) {
			r.unused = append(r.unused, i)
		}
	}
	b.bracketedGroup(r)
	// final conversions
	if n := len(r.unused); n != 0 {
		switch s := r.tags[r.unused[n-1]].Text(); {
		case r.Sum == "" && b.sum.MatchString(s) && strings.ContainsAny(s, "0123456789"):
			r.Sum, r.unused = s, r.unused[:n-1]
		case r.Group == "" && strings.ContainsFunc(s, unicode.IsLetter) && !beforeTag(r.tags, r.unused[n-1]) && !plusList(r.tags, r.unused[n-1]):
			last := r.unused[n-1]
			r.Group, r.unused = s, r.unused[:n-1]
			b.widenGroup(r, last)
		}
	}
	// a checksum in its own bracket with more of the name after it, ie
	// '[Group] Show - 01 [1080p] [ABCD1234] (Alt Title)'
	for k := len(r.unused) - 1; r.Sum == "" && k >= 0; k-- {
		j := r.unused[k]
		if s := r.tags[j].Text(); b.sum.MatchString(s) && strings.ContainsAny(s, "0123456789") &&
			peek(r.tags, j-1, TagTypeDelim) && strings.HasSuffix(r.tags[j-1].Delim(), "[") &&
			peek(r.tags, j+1, TagTypeDelim) && strings.HasPrefix(r.tags[j+1].Delim(), "]") {
			r.Sum, r.unused = s, slices.Delete(r.unused, k, k+1)
		}
	}
}

// bracketedGroup sets the group from a leftover word that a hyphen joins to the
// tags and a bracket follows, ie 'H 264-VARYG (Alt Title, Multi-Subs)'. The
// group lexer does not read a name that ends in a bracket, and the last
// leftover word of such a name is inside the bracket.
func (b *TagBuilder) bracketedGroup(r *Release) {
	if r.Group != "" {
		return
	}
	for k := len(r.unused) - 1; k >= 0; k-- {
		i := r.unused[k]
		if i < 2 || !r.tags[i-1].Is(TagTypeDelim) {
			continue
		}
		if d := r.tags[i-1].Delim(); !strings.HasSuffix(d, "-") || strings.ContainsAny(d, "()[]{}") || !r.tags[i-2].Is(
			TagTypeSource,
			TagTypeResolution,
			TagTypeFrameRate,
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
			TagTypeContainer,
		) {
			continue
		}
		// a group may itself hold hyphens ('-Tsundere-Raws')
		last, n := i, k+1
		for ; n < len(r.unused) && r.unused[n] == last+2 && r.tags[last+1].Is(TagTypeDelim) && r.tags[last+1].Delim() == "-"; n++ {
			last += 2
		}
		if !peek(r.tags, last+1, TagTypeDelim) || !strings.ContainsAny(r.tags[last+1].Delim(), "([{") {
			continue
		}
		var sb strings.Builder
		for j := i; j <= last; j++ {
			sb.WriteString(r.tags[j].Text())
		}
		r.Group, r.unused = sb.String(), slices.Delete(r.unused, k, n)
		return
	}
}

// hyphenGroup bounds a group widened from leftover words that follow a hyphen,
// as in "(... - Kyoshi Mura)": two words of any length that each contain a
// letter (ie, 'Kyoshi Mura', 'Ky0_Mura'). Leftover words have already failed
// to lex as tags, so it can be looser than the group lexer's bound.
var hyphenGroup = regexp.MustCompile(`(?i)^[a-z0-9]*[a-z][a-z0-9]*[ _][a-z0-9]*[a-z][a-z0-9]*$`)

// widenGroup joins the leftover words in front of a guessed group, so
// "(... - DarQ HONE)" yields the same group as "-DarQ HONE". Words right after
// a hyphen may be separated by single spaces or underscores, within
// hyphenGroup or spacedGroup. Other words join across single spaces within
// spacedGroup only, so trailing alternate titles like "(Meitantei Conan)" are
// not taken as a group. A run outside the bound keeps the single word.
func (b *TagBuilder) widenGroup(r *Release, i int) {
	// a comic names its scan group in the parentheses that close the name,
	// and the hyphen is part of it ('(Scan-Team).cbr')
	if n, j := widenRun(r, i, "-"); r.Type == Comic && n < len(r.unused) && j > 0 &&
		strings.HasSuffix(r.tags[j-1].Text(), "(") && peek(r.tags, i+1, TagTypeDelim) && strings.HasPrefix(r.tags[i+1].Text(), ")") {
		r.Group, r.unused = joinRun(r, n), r.unused[:n]
		return
	}
	if n, j := widenRun(r, i, " _"); n < len(r.unused) && j > 0 && strings.Contains(r.tags[j-1].Text(), "-") {
		if group := joinRun(r, n); hyphenGroup.MatchString(group) || spacedGroup.MatchString(group) {
			r.Group, r.unused = group, r.unused[:n]
			return
		}
	}
	if n, _ := widenRun(r, i, " "); n < len(r.unused) {
		if group := joinRun(r, n); spacedGroup.MatchString(group) {
			r.Group, r.unused = group, r.unused[:n]
		}
	}
}

// widenRun returns where the run of leftover words in front of the guessed
// group at tag i starts, as an index into r.unused and into r.tags, when
// single delimiters from seps separate the words.
func widenRun(r *Release, i int, seps string) (int, int) {
	n := len(r.unused)
	for ; n > 0 && r.unused[n-1] == i-2 && len(r.tags[i-1].Text()) == 1 && strings.Contains(seps, r.tags[i-1].Text()); n-- {
		i = r.unused[n-1]
	}
	return n, i
}

// joinRun joins the leftover words from n in r.unused onto the guessed group,
// keeping their delimiters.
func joinRun(r *Release, n int) string {
	var sb strings.Builder
	for _, j := range r.unused[n:] {
		sb.WriteString(r.tags[j].Text())
		sb.WriteString(r.tags[j+1].Text())
	}
	return sb.String() + r.Group
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

// contains returns if v contains str.
func contains(v []string, str string) bool {
	for _, s := range v {
		if str == s {
			return true
		}
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

// isolated determines if the tag in the indicated incremental direction
// (-1/+1) is not text/whitespace.
func isolated(tags []Tag, i, inc int) bool {
	for i += inc; 0 < i && i < len(tags)-1 && tags[i+inc].Is(TagTypeWhitespace, TagTypeDelim); i += inc {
	}
	i += inc
	// the len(tags)-1 bound is deliberate: it excludes the final tag, which at
	// this point in the build is usually an as-yet unidentified group (the
	// group is only settled in unused). Widening it to len(tags) makes
	// 'MiniMeters v0 8 4 Beta MacOS BTCR' treat the trailing group BTCR as
	// title text and lose the MacOS platform.
	return 0 <= i && i < len(tags)-1 && tags[i].Is(TagTypeText)
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
