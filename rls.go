// Package rls parses release information.
package rls

import (
	"bytes"
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/autobrr/rls/taginfo"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

//go:generate stringer -type TagType -trimprefix TagType

// Release is release information.
type Release struct {
	Type Type

	Artist   string
	Title    string
	Subtitle string
	// Alt is the other names of a movie or series given with 'AKA' or 'aka'
	// in the title, one name per element ('A.aka.B.aka.C.2001' gives 'B' and
	// 'C'), or the title in CJK script from a leading bracket in front of a
	// scene name ('[港灯].Harbor.Lights.S01-GRP'). A fansub name gives them
	// behind a bar or a slash, or in a bracket behind the title ('[Group] A
	// | B - 01', '[Group] A / B - 01', '[Group] A (B) - 01').
	Alt []string

	Platform string
	Arch     string

	Source     string
	Resolution string
	// FrameRate is the frames per second as written ('60fps', '23.976fps'),
	// on its own or after the resolution ('1080p60').
	FrameRate  string
	Collection string

	Year  int
	Month int
	Day   int
	// YearEnd is the last year when the name gives a range of years
	// ('2014-2015', '2024-25', '1999-2003'), and Year is the first.
	YearEnd int

	// Numbering is the seasons and the episodes the name gives.
	Numbering Numbering
	Version   string
	Disc      string

	Codec    []string
	HDR      []string
	BitDepth string
	Audio    []string
	Channels string
	Other    []string
	Cut      []string
	Edition  []string
	Language []string

	Size      string
	Region    string
	Container string
	Genre     string
	ID        string
	Group     string
	Meta      []string
	Site      string
	Sum       string
	Pass      string
	Req       bool
	Ext       string

	// sports is whether the caller said the name is a sports broadcast, which
	// Event reads.
	sports bool

	// path is whether the name is a file path, which isPath reports.
	path bool
	// numbered is whether a tag that is no second numbering writes an episode
	// other than 0 in its first place, which the range of a batch does not
	// ('01-12'). The type is read by it, and Event tells a show from an event
	// by it.
	numbered bool
	// groupFrom is where Group was read.
	groupFrom groupOrigin
	// shaped is whether the structure stage read the name, see shapeOf.
	// first is then the first word of its title, as a position in the tags,
	// and boxed whether the title stands in a bracket of its own
	// ('[Group][Title][01]').
	shaped, boxed bool
	first         int
	// pack is whether the name lists what a pack holds ('TV+OVA+Movie',
	// 'Complete Series'), which makes it a series, and single whether it is
	// the name of one file, which the checksum of the file says.
	pack, single bool

	tags   []Tag
	dates  []int
	unused []int
	end    int
}

// Parse creates a release from src.
func Parse(src []byte) Release {
	return DefaultParser.ParseRelease(src)
}

// ParseString creates a release from src
func ParseString(src string) Release {
	// the one copy of the name that the tags of the release share
	if p, ok := DefaultParser.(*TagParser); ok {
		tags, end := parseCopy(p, src)
		return p.state().builder.Build(tags, end)
	}
	return DefaultParser.ParseRelease([]byte(src))
}

// ParseHint creates a release from src with what the caller knows about it.
// It uses the ParseHint method of DefaultParser. When DefaultParser has been
// replaced by a Parser without that method, it calls ParseRelease and the hint
// is ignored.
func ParseHint(src []byte, h Hint) Release {
	if p, ok := DefaultParser.(interface {
		ParseHint([]byte, Hint) Release
	}); ok {
		return p.ParseHint(src, h)
	}
	return DefaultParser.ParseRelease(src)
}

// ParseStringHint creates a release from src with what the caller knows about
// it. As with ParseHint, the hint is ignored when DefaultParser has no
// ParseHint method.
func ParseStringHint(src string, h Hint) Release {
	if p, ok := DefaultParser.(*TagParser); ok {
		tags, end := parseCopy(p, src)
		return p.buildHint(tags, end, h)
	}
	return ParseHint([]byte(src), h)
}

// Format satisfies the fmt.Formatter interface.
//
// Format Options:
//
//	o - original release
//	v - original release, same as o; this is what fmt.Println, %v and %+v print
//	s - normalized capture value (2009)
//	e - tag type and normal value (as in s) surrounded by angle brackets (<Date:2009>)
//	q - original captured value, quoted
func (r Release) Format(f fmt.State, verb rune) {
	switch verb {
	case 'q':
		buf := new(bytes.Buffer)
		for _, tag := range r.tags {
			if _, err := fmt.Fprintf(buf, "%o", tag); err != nil {
				panic(err)
			}
		}
		fmt.Fprintf(f, "%q", buf.Bytes())
	case 'v':
		// Tag.Format uses v for the per-tag form, so print each tag as o
		for _, tag := range r.tags {
			tag.Format(f, 'o')
		}
	case 's', 'o', 'e':
		for _, tag := range r.tags {
			tag.Format(f, verb)
		}
	}
}

// String satisfies the fmt.Stringer interface.
func (r Release) String() string {
	var v []string
	for _, tag := range r.tags {
		v = append(v, fmt.Sprintf("%o", tag))
	}
	return strings.Join(v, "")
}

// Tags returns all tags.
func (r Release) Tags() []Tag {
	return r.tags
}

// maxRangeFill is how many episodes are filled in between the ends of the
// ranges in one release ('E001-E130'). A range that does not fit in what is
// left is given by its two ends alone, which keeps a name such as
// 'E1-E99999.E1-E99999...' from growing into millions of episodes.
const maxRangeFill = 10000

// rangeEnd reports whether the series tag at i writes the end of a range that
// the series tag before it starts, ie the '31' of 'S04E29-31v2'. The series
// lexer reads 'S04E29-31' as one tag, and leaves the '31' to the next tag when
// a version follows it. The tag at i is a bare number, a bare hyphen or tilde
// stands between the two, the tag before writes a season and an episode, and
// the number at i is higher than the last episode it writes.
//
// As in the lexer, four digits after a lower episode are a year or another
// number and not the end of a range ('S23E19-1174v2', 'S01E05-2020v2'), unless
// the episodes are numbered that high ('S01E998-1002v2'). An episode written
// with a word numbers the episode a second way, as it does after a dot
// ('S01E05-Episode.10').
func (r Release) rangeEnd(i int) bool {
	if i < 2 || !r.tags[i-1].rangeDelim() {
		return false
	}
	tag, prev := r.tags[i], r.tags[i-2]
	if prev.typ != TagTypeSeries || prev.val(1) == "" {
		return false
	}
	if s := tag.val(0); s == "" || s[0] < '0' || s[0] > '9' {
		return false
	}
	var start string
	for _, b := range prev.rest(2) {
		if b != "" && b != "-" {
			start = b
		}
	}
	end := tag.val(2)
	first, last := atoi(start), atoi(end)
	if start == "" || len(end) >= 4 && (len(start) < 3 || last-first >= 1000) {
		return false
	}
	return first < last
}

// seasonRange returns the index of the series tag that starts a range of
// seasons which the series tag at i ends, or -1. Both tags write a season and
// no episode, the second season is the higher one, and a hyphen or tilde
// stands between the two with at most dots, underscores and spaces beside it
// ('S01-S03', 'S01-03', 'Seasons 1~7', '(Season 1 - 3)', 'S01 ~ S03').
//
// An episode written after the range with no season of its own is in none of
// them ('Season 1-4 (1-76)').
func (r Release) seasonRange(i int) int {
	tag, start := r.tags[i], -1
	if tag.typ != TagTypeSeries || tag.val(1) == "" || tag.wroteEpisode() {
		return -1
	}
	if i >= 2 && r.tags[i-1].typ == TagTypeDelim && strings.ContainsAny(r.tags[i-1].val(0), "-~") && len(strings.Trim(r.tags[i-1].val(0), "._ ")) == 1 {
		start = i - 2
	}
	if start == -1 || r.tags[start].typ != TagTypeSeries || r.tags[start].val(1) == "" || r.tags[start].wroteEpisode() ||
		atoi(r.tags[start].val(1)) >= atoi(tag.val(1)) {
		return -1
	}
	return start
}

// seasonList returns the index of the series tag before the one at i when the
// two are seasons of a list, or -1. Both tags write a season and no episode,
// the second season is the higher one, and a plus, an ampersand or a comma
// stands between the two with nothing but spaces and underscores beside it
// ('S01+S02', 'Season 1 + 2', 'S1, S2', 'Season 01 & Season 02'). An
// ampersand is a word of its own between two seasons, and the end of the text
// of the first in a list the series lexer read as one ('Seasons 1 &' of
// 'Seasons 1 & 2').
//
// As after a range, an episode written after the list with no season of its
// own is in none of them, whether it is a range or one episode ('S1+S2
// (01-25)', 'S1 + S2 - 05').
func (r Release) seasonList(i int) int {
	tag := r.tags[i]
	if tag.typ != TagTypeSeries || tag.val(1) == "" || tag.wroteEpisode() {
		return -1
	}
	var sep bool
	for j := i - 1; j >= 0; j-- {
		s := strings.Trim(r.tags[j].val(0), " _")
		switch r.tags[j].typ {
		case TagTypeDelim, TagTypeWhitespace, TagTypeText:
			if s != "" && s != "+" && s != "&" && s != "," {
				return -1
			}
			sep = sep || s != ""
		case TagTypeSeries:
			sep = sep || strings.HasSuffix(s, "&")
			if !sep || r.tags[j].val(1) == "" || r.tags[j].wroteEpisode() || atoi(r.tags[j].val(1)) >= atoi(tag.val(1)) {
				return -1
			}
			return j
		default:
			return -1
		}
	}
	return -1
}

// rangeDelim reports whether the tag is the bare hyphen or tilde that makes a
// range of the episode tags on its two sides ('E02-E04', 'E29~E31').
func (tag Tag) rangeDelim() bool {
	return tag.typ == TagTypeDelim && len(tag.v) != 0 && (tag.v[0] == "-" || tag.v[0] == "~")
}

// wroteEpisode reports whether a series tag writes an episode, which a season
// pack ('S01') does not.
func (tag Tag) wroteEpisode() bool {
	for _, b := range tag.rest(2) {
		if b != "" && b != "-" {
			return true
		}
	}
	return false
}

// fraction returns the decimal part an episode is written with, ie the '.5' of
// '21.5', 'S01E06.5' or 'Ep 5.5', and the empty string for a whole episode.
// The lexers and the builder's fixHalfEpisode leave the part in the text of
// the tag, behind the last episode, and it is read from there.
func (tag Tag) fraction() string {
	if tag.typ != TagTypeSeries || len(tag.v) < 3 || tag.v[len(tag.v)-1] == "" {
		return ""
	}
	s := tag.v[0]
	i := strings.LastIndexByte(s, '.')
	if i == -1 || i == len(s)-1 || len(s)-i > 3 || !strings.HasSuffix(s[:i], tag.v[len(tag.v)-1]) {
		return ""
	}
	for _, c := range []byte(s[i+1:]) {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return s[i:]
}

// continuation reports whether a series tag is written as 'E' and digits
// ('E06' of 'S01E05-E06'), which continues the season and episode before it
// rather than numbering the episode a second way ('Episode.1174').
func (tag Tag) continuation() bool {
	s := tag.val(0)
	return len(s) >= 2 && (s[0] == 'E' || s[0] == 'e') && '0' <= s[1] && s[1] <= '9'
}

// Unused returns text tags not used in titles.
func (r Release) Unused() []Tag {
	var unused []Tag
	for _, i := range r.unused {
		unused = append(unused, r.tags[i])
	}
	return unused
}

// Dates returns date tags not used.
func (r Release) Dates() []Tag {
	var dates []Tag
	for _, i := range r.dates {
		dates = append(dates, r.tags[i])
	}
	return dates
}

// IsProper reports whether the release is a proper, written PROPER or
// REAL.PROPER. It reads Other, where the parser puts those two values, and
// compares without regard to case.
func (r Release) IsProper() bool {
	return r.hasOther("PROPER", "REAL.PROPER")
}

// IsRepack reports whether the release is a repack, written REPACK, REPACKED,
// REREPACK or REPACK2 to REPACK99. It reads Other, where the parser puts them
// as REPACK, REREPACK and REPACK3 to REPACK99, and compares without regard to
// case. A REPACK2 set by hand counts too. A re-rip (RERiP) is not a repack.
func (r Release) IsRepack() bool {
	if r.hasOther("REPACK", "REREPACK") {
		return true
	}
	for _, s := range r.Other {
		if len(s) < 7 || len(s) > 8 || !strings.EqualFold(s[:6], "REPACK") {
			continue
		}
		// one digit from 2 on, or two digits that do not begin with 0
		if d := s[6:]; d[0] >= '1' && d[0] <= '9' && (len(d) == 1 && d[0] != '1' || len(d) == 2 && d[1] >= '0' && d[1] <= '9') {
			return true
		}
	}
	return false
}

// IsRemux reports whether the release is a remux. It reads Other and compares
// with REMUX without regard to case.
func (r Release) IsRemux() bool {
	return r.hasOther("REMUX")
}

// IsHybrid reports whether the release is a hybrid of several sources. It
// reads Other and compares with HYBRiD without regard to case.
func (r Release) IsHybrid() bool {
	return r.hasOther("HYBRiD")
}

// hasOther reports whether Other holds any of the given values, ignoring case.
func (r Release) hasOther(values ...string) bool {
	for _, o := range r.Other {
		for _, v := range values {
			if strings.EqualFold(o, v) {
				return true
			}
		}
	}
	return false
}

// Tag is a release tag.
type Tag struct {
	typ TagType
	v   []string
	f   taginfo.FindFunc
	// was is the type before the last As with wasBit set, and 0 for a tag
	// that was never retyped. Keeping both in one field keeps the Tag the
	// size it was.
	was   TagType
	prevf taginfo.FindFunc
	// ninfo and info are the rows f finds for the captured value and for the
	// normalized value. Both are fixed by typ, v and f, so they are resolved
	// once when the tag is made rather than on every Normalize and Info call.
	ninfo *taginfo.Taginfo
	info  *taginfo.Taginfo
}

// val returns the i'th value of the tag, or "" when the tag has fewer. A zero
// Tag, or one made by NewTag from too few values for its type, has no value
// for the accessors to read, and they must not panic on it.
func (tag Tag) val(i int) string {
	if uint(i) < uint(len(tag.v)) {
		return tag.v[i]
	}
	return ""
}

// rest returns the values of the tag from the i'th on, or nil when the tag
// has fewer.
func (tag Tag) rest(i int) []string {
	if i < len(tag.v) {
		return tag.v[i:]
	}
	return nil
}

// resolve looks up the tag's rows.
func (tag Tag) resolve() Tag {
	tag.ninfo, tag.info = nil, nil
	if tag.f != nil {
		tag.ninfo = tag.f(tag.val(1))
		tag.info = tag.f(tag.Normalize())
	}
	return tag
}

// NewTag creates a new tag.
func NewTag(typ TagType, f taginfo.FindFunc, b ...[]byte) Tag {
	if len(b) < 2 {
		panic("must provide at least 2 values to NewTag")
	}
	v := make([]string, len(b))
	for i := range b {
		// text and delimiter tags pass the same bytes as the original and the
		// value; share the one string rather than copying it twice
		if i > 0 && len(b[i]) == len(b[i-1]) && len(b[i]) > 1 && &b[i][0] == &b[i-1][0] {
			v[i] = v[i-1]
			continue
		}
		v[i] = string(b[i])
	}
	return Tag{
		typ: typ,
		v:   v,
		f:   f,
	}.resolve()
}

// newTag is NewTag for the lexers of this package. A value whose bytes lie in
// src or in buf becomes a string that shares their memory, where NewTag copies
// it, and a value from anywhere else is copied. A TagParser hands its lexers a
// src and a buf of its own, so the tags of a parse hold one copy of the name
// between them. Nothing may write to either once a tag points into it, which
// the lexers of this package do not, and a lexer of a caller's must not.
func newTag(src, buf []byte, typ TagType, f *finder, b ...[]byte) Tag {
	if len(b) < 2 {
		panic("must provide at least 2 values to NewTag")
	}
	v := make([]string, len(b))
	for i := range b {
		switch {
		case len(b[i]) == 0:
		case i > 0 && len(b[i]) == len(b[i-1]) && &b[i][0] == &b[i-1][0]:
			v[i] = v[i-1]
		case within(src, b[i]), within(buf, b[i]):
			v[i] = unsafe.String(&b[i][0], len(b[i]))
		default:
			v[i] = string(b[i])
		}
	}
	tag := Tag{
		typ: typ,
		v:   v,
	}
	if f != nil {
		tag.f = f.fn
		f.resolve(&tag)
	}
	return tag
}

// within reports whether the bytes of b, which is not empty, are bytes of a.
func within(a, b []byte) bool {
	if len(a) == 0 {
		return false
	}
	p, base := uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&a[0]))
	return base <= p && p-base+uintptr(len(b)) <= uintptr(len(a))
}

// ParseTags parses tags from src.
func ParseTags(src []byte) ([]Tag, int) {
	return DefaultParser.Parse(src)
}

// ParseTagsString parses tags from src.
func ParseTagsString(src string) ([]Tag, int) {
	if p, ok := DefaultParser.(*TagParser); ok {
		return parseCopy(p, src)
	}
	return DefaultParser.Parse([]byte(src))
}

// As returns a copy of tag as a tag of the specified type.
func (tag Tag) As(typ TagType, f taginfo.FindFunc) Tag {
	return Tag{
		typ:   typ,
		f:     f,
		v:     tag.v,
		was:   tag.typ ^ wasBit,
		prevf: tag.f,
	}.resolve()
}

// as is As for a finder of this package, whose rows it looks up without the
// two calls of a find func.
func (tag Tag) as(typ TagType, f *finder) Tag {
	t := Tag{
		typ:   typ,
		v:     tag.v,
		was:   tag.typ ^ wasBit,
		prevf: tag.f,
	}
	if f != nil {
		t.f = f.fn
		f.resolve(&t)
	}
	return t
}

// Revert returns a copy of tag as the tag's previous type. The copy does not
// remember the type it had, so it is not Retyped. A tag that was never
// retyped reverts to a whitespace tag, since that is what Prev reports for
// it.
func (tag Tag) Revert() Tag {
	return Tag{
		typ: tag.Prev(),
		f:   tag.prevf,
		v:   tag.v,
	}.resolve()
}

// Is returns true when tag is of a type.
func (tag Tag) Is(types ...TagType) bool {
	return slices.Contains(types, tag.typ)
}

// Was returns true when the tag's previous type is one of types. A tag that
// was never retyped reports TagTypeWhitespace, as Prev does. Retyped tells
// the two apart.
func (tag Tag) Was(types ...TagType) bool {
	prev := tag.Prev()
	return slices.Contains(types, prev)
}

// Info retrieves the tag's tag info.
func (tag Tag) Info() *taginfo.Taginfo {
	return tag.info
}

// SingleEp returns true when the
func (tag Tag) SingleEp() bool {
	if tag.typ.Is(TagTypeSeries) {
		s, e := tag.Series()
		return s == 0 && e != 0 && tag.val(1) == "" && tag.val(0) == tag.val(2)
	}
	return false
}

// InfoType returns the associated tag info type.
func (tag Tag) InfoType() Type {
	if info := tag.Info(); info != nil {
		return Type(info.Type())
	}
	return Unknown
}

// InfoExcl returns the associated tag info excl.
func (tag Tag) InfoExcl() bool {
	if info := tag.Info(); info != nil {
		return info.Excl()
	}
	return false
}

// TagType returns the tag's tag type.
func (tag Tag) TagType() TagType {
	return tag.typ
}

// Prev returns the tag's previous tag type. A tag that was never retyped
// reports TagTypeWhitespace, which is the zero value, and so does a tag that
// Revert returned. Prev cannot tell either from a tag that was whitespace
// before. Retyped can.
func (tag Tag) Prev() TagType {
	if tag.was == 0 {
		return TagTypeWhitespace
	}
	return tag.was ^ wasBit
}

// Retyped reports whether the tag was given another type after it was read.
// It is true for a tag that As returned, whatever type As gave it, and false
// for a tag as the lexer made it, for the zero Tag and for a tag that Revert
// returned. It answers what Prev() == TagTypeWhitespace cannot, because a tag
// that was never retyped and a tag that was whitespace before give the same
// Prev.
func (tag Tag) Retyped() bool {
	return tag.was != 0
}

// wasBit is set in Tag.was beside the previous type. It is the sign bit, so
// that the one type a retyped tag cannot be told to have had is the lowest
// int, where the plain "type plus one" would lose TagType(-1).
const wasBit TagType = math.MinInt

// InfoTitle retrieves the tag's title.
func (tag Tag) InfoTitle() string {
	if info := tag.Info(); info != nil {
		_, s := info.Expand(tag.val(1))
		return s
	}
	return ""
}

// normalize attempts to normalize s, which is the captured value. A tag info
// with $1, $2, ... $N in its tag takes them from its own match of s: the
// tag's values hold the capture groups of every tag info of the type, so
// their positions say nothing about the one that matched.
func (tag Tag) normalize(s string) string {
	if tag.f != nil {
		// every caller passes the captured value, which is what ninfo is for
		info := tag.ninfo
		if len(tag.v) < 2 || s != tag.val(1) {
			info = tag.f(s)
		}
		if info != nil {
			s, _ = info.Expand(s)
		}
	}
	return s
}

// Normalize returns the normalized string for the tag.
func (tag Tag) Normalize() string {
	switch tag.typ {
	case TagTypeWhitespace:
		return tag.Whitespace()
	case TagTypeDelim:
		return tag.Delim()
	case TagTypeText:
		return tag.Text()
	case TagTypePlatform:
		return tag.Platform()
	case TagTypeArch:
		return tag.Arch()
	case TagTypeSource:
		return tag.Source()
	case TagTypeResolution:
		return tag.Resolution()
	case TagTypeCollection:
		return tag.Collection()
	case TagTypeDate:
		year, month, day := tag.Date()
		if month != 0 && day != 0 {
			return fmt.Sprintf("%d-%02d-%02d", year, month, day)
		}
		if last := tag.yearEnd(); last != 0 {
			return fmt.Sprintf("%d-%d", year, last)
		}
		return strconv.Itoa(year)
	case TagTypeSeries:
		series, episode := tag.Series()
		if episode != 0 {
			return fmt.Sprintf("S%02dE%02d", series, episode)
		}
		return fmt.Sprintf("S%02d", series)
	case TagTypeVersion:
		return tag.Version()
	case TagTypeDisc:
		return tag.Disc()
	case TagTypeCodec:
		return tag.Codec()
	case TagTypeHDR:
		return tag.HDR()
	case TagTypeBitDepth:
		return tag.BitDepth()
	case TagTypeAudio:
		return tag.Audio()
	case TagTypeChannels:
		return tag.Channels()
	case TagTypeOther:
		return tag.Other()
	case TagTypeCut:
		return tag.Cut()
	case TagTypeEdition:
		return tag.Edition()
	case TagTypeLanguage:
		return tag.Language()
	case TagTypeSize:
		return tag.Size()
	case TagTypeRegion:
		return tag.Region()
	case TagTypeContainer:
		return tag.Container()
	case TagTypeGenre:
		return tag.Genre()
	case TagTypeID:
		return tag.ID()
	case TagTypeGroup:
		return tag.Group()
	case TagTypeMeta:
		typ, s := tag.Meta()
		switch typ {
		case "site", "sum":
			// a group in the square brackets of CJK text keeps them, see
			// leadSquare ('【Group】')
			if o := tag.val(0); squared(o) {
				return o
			}
			return "[" + s + "]"
		case "pass":
			return "{{" + s + "}}"
		case "req":
			return "[REQ]"
		}
		return "[[" + typ + ":" + s + "]]"
	case TagTypeExt:
		return tag.Ext()
	case TagTypeFrameRate:
		return tag.FrameRate()
	case tagTypeUnit:
		// the kind, and what it counts or names: 'ova 02', 'special 01 -
		// 03', 'special 1 & 3', 'volume 06', 'contents Complete Series',
		// 'batch'
		return strings.Join(tag.rest(1), " ")
	}
	return ""
}

// Match determines if s matches the tag.
func (tag Tag) Match(s string, verb rune, types ...TagType) bool {
	if len(types) != 0 && !tag.Is(types...) {
		return false
	}
	v := fmt.Sprintf("%"+string(verb), tag)
	switch {
	case s == "":
		return true
	case tag.f != nil && (verb == 's'):
		if info := tag.f(s); info != nil {
			s = info.Tag()
		}
	}
	if verb == 'r' {
		re := matchRE(s)
		return re != nil && re.MatchString(v)
	}
	return s == v
}

// reCacheMax is how many patterns reCache holds. The pattern comes from the
// caller, so without a limit every distinct one would stay in memory forever.
const reCacheMax = 128

// reCache caches the regexps compiled for Match's 'r' verb, which would
// otherwise recompile the caller's pattern on every call. A pattern that does
// not compile is cached as a nil *regexp.Regexp, so it is not compiled again.
var reCache sync.Map // string -> *regexp.Regexp

// reCacheLen is how many patterns reCache holds, or is about to hold.
var reCacheLen atomic.Int64

// matchRE returns the compiled regexp for s, or nil when s is not a valid
// pattern ('(' does not compile, so nothing matches it). Once reCache is full a
// new pattern is compiled for the call alone and not stored.
func matchRE(s string) *regexp.Regexp {
	if v, ok := reCache.Load(s); ok {
		return v.(*regexp.Regexp)
	}
	// re is nil when s does not compile
	re, _ := regexp.Compile(s)
	if reCacheLen.Add(1) > reCacheMax {
		reCacheLen.Add(-1)
		return re
	}
	if v, loaded := reCache.LoadOrStore(s, re); loaded {
		// another call stored it first
		reCacheLen.Add(-1)
		return v.(*regexp.Regexp)
	}
	return re
}

// Format satisfies the fmt.Formatter interface.
//
// Format Options:
//
//	q - all values including captured values, quoted (["2009", "2009", "", ""])
//	o - original capture (2009)
//	v - tag type followed by colon and quoted capture value (Date:["2009", "", ""])
//	s - normalized capture value (2009)
//	r - same as s
//	e - tag type and normal value (as in s) surrounded by angle brackets (<Date:2009>)
func (tag Tag) Format(f fmt.State, verb rune) {
	var buf []byte
	switch verb {
	case 'q':
		buf = append(buf, fmt.Sprintf("%q", tag.v)...)
	case 'o':
		buf = append(buf, tag.val(0)...)
	case 'v':
		buf = append(buf, fmt.Sprintf("%s:%q", tag.typeName(), tag.rest(1))...)
	case 'e':
		s := strconv.Quote(tag.Normalize())
		buf = append(buf, "<"+tag.typeName()+":"+s[1:len(s)-1]+">"...)
	case 's', 'r':
		buf = append(buf, tag.Normalize()...)
	}
	_, _ = f.Write(buf)
}

// typeName returns the name of the type of the tag as Format prints it,
// which is 'Unit' for a tag of the structure stage, see tagTypeUnit.
func (tag Tag) typeName() string {
	if tag.typ == tagTypeUnit {
		return "Unit"
	}
	return tag.typ.String()
}

// Whitespace normalizes the whitespace value.
func (tag Tag) Whitespace() string {
	return tag.val(1)
}

// Delim normalizes the delimiter value.
func (tag Tag) Delim() string {
	return tag.val(1)
}

// Text normalizes the text value.
func (tag Tag) Text() string {
	// what was a date or a series reads as it was written
	i := 1
	switch tag.was {
	case TagTypeDate ^ wasBit, TagTypeSeries ^ wasBit:
		i = 0
	case TagTypeChannels ^ wasBit:
		return tag.Channels()
	}
	if i < len(tag.v) {
		return tag.v[i]
	}
	return ""
}

// TextReplace normalizes the text value and replaces the supplied string.
func (tag Tag) TextReplace(old, newstr string, n int) string {
	s := tag.Text()
	if tag.was == TagTypeChannels^wasBit {
		return s
	}
	return strings.Replace(s, old, newstr, n)
}

// Platform normalizes the platform value.
func (tag Tag) Platform() string {
	return tag.normalize(tag.val(1))
}

// Arch normalizes the arch value.
func (tag Tag) Arch() string {
	return tag.normalize(tag.val(1))
}

// Source normalizes the source value.
func (tag Tag) Source() string {
	return tag.normalize(tag.val(1))
}

// Resolution normalizes the resolution value.
func (tag Tag) Resolution() string {
	return tag.normalize(tag.val(1))
}

// Collection normalizes the collection value.
func (tag Tag) Collection() string {
	return tag.normalize(tag.val(1))
}

// atoi parses a decimal, reporting 0 when s is not one. Unlike strconv.Atoi it
// allocates nothing on failure, and every caller here discards the error --
// those discarded *NumError values were 2.4% of all objects allocated.
func atoi(s string) int {
	if s == "" || len(s) > 18 {
		return 0
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// Date normalizes the date value.
func (tag Tag) Date() (int, int, int) {
	year, month, day := atoi(tag.val(1)), atoi(tag.val(2)), atoi(tag.val(3))
	return year, month, day
}

// yearEnd returns the last year of a date written as a range of years
// ('2014-2015'), or 0.
func (tag Tag) yearEnd() int {
	if len(tag.v) > 4 {
		return atoi(tag.v[4])
	}
	return 0
}

// Series normalizes the series value.
func (tag Tag) Series() (int, int) {
	series, episode := atoi(tag.val(1)), atoi(tag.val(2))
	return series, episode
}

// Episodes normalizes the episodes value. A "-" between two episodes makes
// them the ends of a range ('E05-07'), and the episodes in between are
// included. A written episode 0 is included ('E00'), and a tag that writes no
// episode gives none ('S01').
func (tag Tag) Episodes() []int {
	fill := maxRangeFill
	return tag.episodes(&fill)
}

// episodes is Episodes with the number of episodes it may fill ranges with in
// fill, which it counts down. A range wider than that is left as its ends.
func (tag Tag) episodes(fill *int) []int {
	var v []int
	prev, ranged := -1, false
	for _, b := range tag.rest(2) {
		if b == "-" {
			ranged = true
			continue
		}
		episode := atoi(b)
		if n := episode - prev - 1; ranged && prev != -1 && 0 < n && n <= *fill {
			for j := prev + 1; j < episode; j++ {
				v = append(v, j)
			}
			*fill -= n
		}
		// an episode that was not written is the empty string, and a written
		// 0 is an episode ('S01E00')
		if b != "" {
			v, prev = append(v, episode), episode
		}
		ranged = false
	}
	return v
}

// Version normalizes the version value.
func (tag Tag) Version() string {
	return tag.val(1)
}

// Disc normmalizes the disc value.
func (tag Tag) Disc() string {
	if len(tag.v) < 3 {
		return ""
	}
	disc := atoi(tag.val(2))
	switch tag.val(1) {
	case "CD", "DVD":
		return fmt.Sprintf("%s%d", tag.val(1), disc)
	case "S":
		return fmt.Sprintf("%dDiSCS", disc)
	case "X":
		return fmt.Sprintf("%dx", disc)
	}
	return fmt.Sprintf("D%02d", disc)
}

// Codec normalizes a codec value.
func (tag Tag) Codec() string {
	return tag.normalize(tag.val(1))
}

// HDR normalizes a hdr value.
func (tag Tag) HDR() string {
	return tag.normalize(tag.val(1))
}

// BitDepth normalizes a bitdepth value.
func (tag Tag) BitDepth() string {
	return tag.normalize(tag.val(1))
}

// Audio normalizes an audio value.
func (tag Tag) Audio() string {
	return tag.normalize(tag.val(1))
}

// Channels normalizes an channels value.
func (tag Tag) Channels() string {
	s := strings.Map(func(r rune) rune {
		switch r {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return r
		}
		return -1
	}, tag.normalize(tag.val(1)))
	if s == "" {
		return ""
	}
	return s[:1] + "." + s[1:]
}

// Other normalizes the other value.
func (tag Tag) Other() string {
	s := tag.normalize(tag.val(1))
	switch y := strings.ToUpper(s); y {
	case "19XX", "20XX":
		return y
	}
	return s
}

// Cut normalizes the cut value.
func (tag Tag) Cut() string {
	return tag.normalize(tag.val(1))
}

// Edition normalizes the edition value.
func (tag Tag) Edition() string {
	return tag.normalize(tag.val(1))
}

// Language normalizes the language value.
func (tag Tag) Language() string {
	return tag.normalize(tag.val(1))
}

// Size normalizes the size value.
func (tag Tag) Size() string {
	return strings.ReplaceAll(strings.ToUpper(tag.normalize(tag.val(1))), "I", "i")
}

// Region normalizes the region value.
func (tag Tag) Region() string {
	return tag.normalize(tag.val(1))
}

// Container normalizes the container value.
func (tag Tag) Container() string {
	return tag.normalize(tag.val(1))
}

// Genre normalizes the genre value.
func (tag Tag) Genre() string {
	return tag.normalize(tag.val(1))
}

// ID normalizes the id value.
func (tag Tag) ID() string {
	return tag.normalize(tag.val(1))
}

// Group normalizes the group value.
func (tag Tag) Group() string {
	return tag.val(1)
}

// Meta normalizes a file meta value.
func (tag Tag) Meta() (string, string) {
	return tag.val(1), tag.val(2)
}

// Ext normalizes a file ext value.
func (tag Tag) Ext() string {
	return strings.ToLower(tag.val(1))
}

// FrameRate normalizes a frame rate value.
func (tag Tag) FrameRate() string {
	if len(tag.v) < 2 {
		return ""
	}
	return tag.v[1] + "fps"
}

// TagType is a tag type.
type TagType int

// TagType values.
const (
	TagTypeWhitespace TagType = iota
	TagTypeDelim
	TagTypeText
	TagTypePlatform
	TagTypeArch
	TagTypeSource
	TagTypeResolution
	TagTypeCollection
	TagTypeDate
	TagTypeSeries
	TagTypeVersion
	TagTypeDisc
	TagTypeCodec
	TagTypeHDR
	TagTypeBitDepth
	TagTypeAudio
	TagTypeChannels
	TagTypeOther
	TagTypeCut
	TagTypeEdition
	TagTypeLanguage
	TagTypeSize
	TagTypeRegion
	TagTypeContainer
	TagTypeGenre
	TagTypeID
	TagTypeGroup
	TagTypeMeta
	TagTypeExt
	TagTypeFrameRate
)

// Is returns true when tag type is in types.
func (typ TagType) Is(types ...TagType) bool {
	return slices.Contains(types, typ)
}

// Type is a release type.
type Type int

// Release types.
const (
	Unknown Type = iota
	App
	Audiobook
	Book
	Comic
	Education
	Episode
	Game
	Magazine
	Movie
	Music
	Series
)

// ParseType parses a type from s.
func ParseType(s string) Type {
	switch s {
	case "app":
		return App
	case "audiobook":
		return Audiobook
	case "book":
		return Book
	case "comic":
		return Comic
	case "education":
		return Education
	case "episode":
		return Episode
	case "game":
		return Game
	case "magazine":
		return Magazine
	case "movie":
		return Movie
	case "music":
		return Music
	case "series":
		return Series
	}
	return Unknown
}

// String satisfies the fmt.Stringer interface.
func (typ Type) String() string {
	switch typ {
	case App:
		return "app"
	case Audiobook:
		return "audiobook"
	case Book:
		return "book"
	case Comic:
		return "comic"
	case Education:
		return "education"
	case Episode:
		return "episode"
	case Game:
		return "game"
	case Magazine:
		return "magazine"
	case Movie:
		return "movie"
	case Music:
		return "music"
	case Series:
		return "series"
	}
	return ""
}

// Is returns true when the type is in types.
func (typ Type) Is(types ...Type) bool {
	return slices.Contains(types, typ)
}

// Builder is the interface for release builders.
type Builder interface {
	Build([]Tag, int) Release
}

// Parser is the interface for parsers.
type Parser interface {
	Parse([]byte) ([]Tag, int)
	ParseRelease([]byte) Release
}

// NewDefaultParser creates a new default tag parser.
func NewDefaultParser() Parser {
	return NewTagParser(taginfo.All(), DefaultLexers()...)
}

// DefaultBuilder is the default release tag builder.
var DefaultBuilder Builder

// DefaultParser is the default tag parser.
//
// The built-in one is a *TagParser that is built when it first parses, so a
// program that never parses, or that assigns its own parser here before it
// parses, does not pay for it.
var DefaultParser Parser

// defaultBuilds counts the times the built-in default parser was built, for
// the tests.
var defaultBuilds atomic.Int32

// newDefaultTagParser returns the built-in default parser, unbuilt. It is
// built with builder, the DefaultBuilder of the time the package was
// initialized, as it was when it was built there.
func newDefaultTagParser(builder Builder) *TagParser {
	return &TagParser{load: sync.OnceValue(func() *tagParser {
		defaultBuilds.Add(1)
		return &newTagParser(builder, taginfo.All(), DefaultLexers()...).tagParser
	})}
}

func init() {
	for i := Unknown; i <= Series; i++ {
		taginfo.RegisterType(i.String(), int(i))
	}
	DefaultBuilder = NewTagBuilder()
	DefaultParser = newDefaultTagParser(DefaultBuilder)
}

// CompareMap is the release compare map. Modifying this will alter the order
// by which different release types are grouped together when using Compare
// with a sort operation.
var CompareMap = map[Type]int{
	Unknown:   0,
	Movie:     1,
	Series:    2,
	Episode:   2,
	Music:     3,
	App:       4,
	Game:      5,
	Book:      6,
	Audiobook: 7,
	Comic:     9,
	Education: 8,
	Magazine:  10,
}

// Compare compares a to b, normalizing titles with Normalize, comparing the
// resulting lower cased strings. Release types are grouped together based on
// the precedence defined in CompareMap. The season and the episode compared
// are those Numbering.Season and Numbering.Episode give, with 0 for none.
func Compare(a, b Release) int {
	aseason, _ := a.Numbering.Season()
	bseason, _ := b.Numbering.Season()
	aepisode, _ := a.Numbering.Episode()
	bepisode, _ := b.Numbering.Episode()
	var c int
	for _, f := range []func() int{
		compareInt(CompareMap[a.Type], CompareMap[b.Type]),
		compareTitle(a.Artist, b.Artist),
		compareTitle(a.Title, b.Title),
		compareInt(a.Year, b.Year),
		compareInt(a.Month, b.Month),
		compareInt(a.Day, b.Day),
		compareInt(aseason, bseason),
		compareInt(aepisode, bepisode),
		compareTitle(a.Subtitle, b.Subtitle),
		compareTitles(a.Alt, b.Alt),
		compareIntString(a.Resolution, b.Resolution),
		compareString(a.Version, b.Version),
		compareString(a.Group, b.Group),
		compareString(a.Title, b.Title),
		compareOriginal(a, b),
	} {
		if c = f(); c != 0 {
			return c
		}
	}
	return c
}

// compareInt returns a func that compares a, b.
func compareInt(a, b int) func() int {
	return func() int {
		switch {
		case a < b:
			return -1
		case b < a:
			return 1
		}
		return 0
	}
}

// compareIntString returns a func that compares the numbers in a, b, which
// for a resolution is its line count. At the same line count an interlaced
// picture ('1080i') sorts below a progressive one ('1080p').
func compareIntString(a, b string) func() int {
	return func() int {
		switch {
		case a == b:
			return 0
		case a == "" && b != "":
			return -1
		case b == "" && a != "":
			return 1
		case !strings.ContainsAny(a, "0123456789"):
			return 0
		case !strings.ContainsAny(b, "0123456789"):
			return 1
		}
		if c := cmp.Compare(digits(a), digits(b)); c != 0 {
			return c
		}
		switch ai, bi := strings.HasSuffix(a, "i"), strings.HasSuffix(b, "i"); {
		case ai && !bi:
			return -1
		case bi && !ai:
			return 1
		}
		return 0
	}
}

// digits returns the digits of s read as one number, so '1080p' and '1080i'
// are 1080. A number of thousands of columns ('DCI4K', 'DCI2K') is given as
// the line count it stands for, 2160 and 1080.
func digits(s string) int {
	var n int
	// no more than nine digits, which an int holds on every platform
	for i := 0; i < len(s) && n < 1e8; i++ {
		if c := s[i]; '0' <= c && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	if n < 100 && (strings.HasSuffix(s, "K") || strings.HasSuffix(s, "k")) {
		n *= 540
	}
	return n
}

// compareTitle returns a func that does a title comparison of a, b.
func compareTitle(a, b string) func() int {
	// const cutset = "\t\n\f\r -._,()[]{}+\\/~"
	return func() int {
		switch {
		case a == b:
			return 0
		case a == "" && b != "":
			return -1
		case b == "" && a != "":
			return 1
		}
		a, b := MustNormalize(a), MustNormalize(b)
		av, bv := strings.FieldsFunc(strings.ToLower(a), isBreakDelim), strings.FieldsFunc(strings.ToLower(b), isBreakDelim)
		start, min := 0, 3
		if len(av) > 0 && len(bv) > 0 && av[0] == bv[0] && slices.Contains([]string{"a", "an", "the"}, av[0]) {
			start, min = 1, 1
		}
		// compare fields up to a prefix length
		i := start
		for ; i < start+min && i < len(av) && i < len(bv); i++ {
			switch {
			case av[i] == "and" && bv[i] == "&", av[i] == "&" && bv[i] == "and":
				continue
			}
			if cmp := compareTitleNumber(av[i], bv[i], i); cmp != 0 {
				return cmp
			}
		}
		if i < start+min {
			// haven't reached end of prefixes and a or b is prefix of the
			// other, determine which is shorter
			switch {
			case len(av) < len(bv):
				return -1
			case len(bv) < len(av):
				return +1
			}
		}
		return 0
	}
}

// compareTitles compares a and b name by name with compareTitle, then by
// the number of names.
func compareTitles(a, b []string) func() int {
	return func() int {
		for i := range min(len(a), len(b)) {
			if c := compareTitle(a[i], b[i])(); c != 0 {
				return c
			}
		}
		return compareInt(len(a), len(b))()
	}
}

// compareTitleNumber compares a, b as numbers if both are numbers or roman
// numerals (such as VI or 6).
func compareTitleNumber(a, b string, i int) int {
	ai, arom, aok := convNumber(a)
	bi, brom, bok := convNumber(b)
	abad := i == 0 && arom && aok && (ai == 1 || ai == 5 || ai == 50)
	bbad := i == 0 && brom && bok && (bi == 1 || bi == 5 || bi == 50)
	switch {
	case (arom || brom) && i == 0:
	case abad && bbad:
	case aok && bbad:
		return -1
	case bok && abad:
		return +1
	case aok && bok && ai < bi, aok && !bok && !abad:
		return -1
	case aok && bok && bi < ai, bok && !aok && !bbad:
		return +1
	}
	return strings.Compare(a, b)
}

// compareString returns a func that compares a, b.
func compareString(a, b string) func() int {
	return func() int {
		return strings.Compare(a, b)
	}
}

// compareOriginal returns a func that compares a, b's original string.
func compareOriginal(a, b Release) func() int {
	return func() int {
		return strings.Compare(fmt.Sprintf("%o", a), fmt.Sprintf("%o", b))
	}
}

// isAnyDelim returns true if r is any delimiter.
func isAnyDelim(r rune) bool {
	switch r {
	case '\t', '\n', '\f', '\r', ' ', '(', ')', '+', ',', '-', '.', '_', '[', '/', '\\', ']', '{', '}', '~':
		return true
	}
	return false
}

// isBreakDelim returns true if r is a break delimiter (same as any, excluding
// '-').
func isBreakDelim(r rune) bool {
	switch r {
	case '\t', '\n', '\f', '\r', ' ', '(', ')', '+', ',' /*, '-'*/, '.', '_', '[', '/', '\\', ']', '{', '}', '~':
		return true
	}
	return false
}

// isTitleTrimDelim returns true if r is a title trim delimiter (any delim
// except '.', '+').
func isTitleTrimDelim(r rune) bool {
	switch r {
	case '\t', '\n', '\f', '\r', ' ', '(', ')' /*, '+'*/, ',', '-' /*, '.'*/, '_', '[', '/', '\\', ']', '{', '}', '~':
		return true
	}
	return false
}

// convNumber attempts to convert a int or roman numeral.
func convNumber(s string) (int, bool, bool) {
	if i, err := strconv.Atoi(s); err == nil {
		return i, false, true
	} else if i, ok := parseRoman(s); ok && i < 100 {
		return i, true, true
	}
	return 0, false, false
}

// parseRoman parses roman numerals.
func parseRoman(s string) (int, bool) {
	if s == "" {
		return 0, true
	}
	var i, r int
	for j := 0; j < len(s); j++ {
		switch r = roman(s[j]); {
		case r == 0, j < len(s)-2 && r < roman(s[j+1]) && roman(s[j+1]) < roman(s[j+2]):
			return -1, false
		case j < len(s)-1 && r < roman(s[j+1]):
			i -= r
		default:
			i += r
		}
	}
	return i, true
}

// roman returns the value for a roman numeral.
func roman(c byte) int {
	switch c {
	case 'i':
		return 1
	case 'v':
		return 5
	case 'x':
		return 10
	case 'l':
		return 50
	case 'c':
		return 100
	case 'd':
		return 500
	case 'm':
		return 1000
	}
	return 0
}

// Find finds a tag.
func Find(tags []Tag, s string, count int, verb rune, types ...TagType) ([]Tag, int) {
	// a count below 0 or past the tags means all of them
	if count < 0 || count > len(tags) {
		count = len(tags)
	}
	// copy
	if s == "" && len(types) == 0 {
		v := make([]Tag, count)
		copy(v, tags[:count])
		return v, count
	}
	var v []Tag
	var i int
	// collect matching
	for ; i < len(tags) && len(v) < count; i++ {
		if tags[i].Match(s, verb, types...) {
			v = append(v, tags[i])
		}
	}
	return v, i
}

// NewCleaner creates a text transformer chain that transforms text to its
// textual decomposed clean form (NFD), removing all non-spacing marks,
// converting all spaces to ' ', removing ', collapsing adjacent spaces into a
// single ' ', and finally returning the canonical normalized form (NFC).
//
// See: https://go.dev/blog/normalization
func NewCleaner() transform.Transformer {
	return transform.Chain(norm.NFD, newCleanCollapser(), norm.NFC)
}

// newCleanCollapser creates the collapser used by NewCleaner.
func newCleanCollapser() Collapser {
	return NewCollapser(
		false, true,
		`'`, " \t\r\n\f",
		nil,
	)
}

// cleaners pools the transform chains used by MustClean. Building the chain is
// the dominant cost of a call, and a transform.Transformer carries state across
// Reset so a single shared instance would not be safe for concurrent use.
var cleaners = sync.Pool{
	New: func() any { return NewCleaner() },
}

// MustClean applies the Clean transform to s.
func MustClean(s string) string {
	t := cleaners.Get().(transform.Transformer)
	defer cleaners.Put(t)
	s, _, err := transform.String(t, s)
	if err != nil {
		panic(err)
	}
	return s
}

// NewNormalizer creates a new a text transformer chain (similiar to
// NewCleaner) that normalizes text to lower case clean form useful for
// matching titles.
//
// See: https://go.dev/blog/normalization
func NewNormalizer() transform.Transformer {
	return transform.Chain(norm.NFD, newNormalizeCollapser(), norm.NFC)
}

// newNormalizeCollapser creates the collapser used by NewNormalizer.
func newNormalizeCollapser() Collapser {
	return NewCollapser(
		true, true,
		"`"+`':;~!@#%^*=+()[]{}<>/?|\",`, " \t\r\n\f._",
		func(r, prev, next rune) rune {
			switch {
			case r == '-' && unicode.IsSpace(prev):
				return -1
			case r == '$' && (unicode.IsLetter(prev) || unicode.IsLetter(next)):
				return 'S'
			case r == '£' && (unicode.IsLetter(prev) || unicode.IsLetter(next)):
				return 'L'
			case r == '$', r == '£':
				return -1
			}
			return r
		},
	)
}

// normalizers pools the transform chains used by MustNormalize. See cleaners.
var normalizers = sync.Pool{
	New: func() any { return NewNormalizer() },
}

// MustNormalize applies the Normalize transform to s, returning a lower cased,
// clean form of s useful for matching titles.
func MustNormalize(s string) string {
	t := normalizers.Get().(transform.Transformer)
	defer normalizers.Put(t)
	s, _, err := transform.String(t, s)
	if err != nil {
		panic(err)
	}
	return s
}

// Collapser is a transform.Transformer that converts spaces to ' ', removes
// specified runes, and collapses adjacent spaces to a single ' '.
//
// See: https://go.dev/blog/normalization
type Collapser struct {
	// Spc is the space rune.
	Spc rune
	// Lower toggles lower casing.
	Lower bool
	// Trim toggles trimming leading/trailing spaces.
	Trim bool
	// Remove are the runes to remove.
	Remove map[rune]bool
	// Space are the runes to convert to Spc.
	Space map[rune]bool
	// Transformer is a rune transformer.
	Transformer func(rune, rune, rune) rune
	// state is what one Transform call has to know about the calls before
	// it. It is nil for a Collapser built as a struct literal, which then
	// treats every call as the whole text.
	state *collapserState
}

// collapserState is the state a Collapser carries from one Transform call to
// the next. A transform chain passes text longer than its buffers in pieces, so
// without it the spaces at the edge of every piece would be trimmed as if they
// were at the edge of the text.
type collapserState struct {
	// prev is the last rune written, or held back in spaces.
	prev rune
	// started is true once the leading spaces have ended.
	started bool
	// spaces is the number of Spc runes held back until a rune follows them:
	// spaces at the end of the text are never written.
	spaces int
	// invalid is true when the last byte read was not valid UTF-8.
	invalid bool
}

// NewCollapser creates a text transformer that collapses spaces, removes
// specific runes, optionally lower cases runes, and removes non-spacing marks.
//
// Ideally used between a transform chain of norm.NFD and norm.NFC.
//
// Clean, Normalize, and MustClean, MustNormalize are provided for utility
// purposes.
//
// The returned Collapser carries state between calls to Transform, so it must
// not be used by two transform chains, or two goroutines, at the same time.
//
// See: https://go.dev/blog/normalization
func NewCollapser(lower, trim bool, remove, space string, transformer func(rune, rune, rune) rune) Collapser {
	rem, spc := make(map[rune]bool), make(map[rune]bool)
	for _, r := range remove {
		rem[r] = true
	}
	for _, r := range space {
		spc[r] = true
	}
	return Collapser{
		Spc:         ' ',
		Lower:       lower,
		Trim:        trim,
		Remove:      rem,
		Space:       spc,
		Transformer: transformer,
		state:       new(collapserState),
	}
}

// Transform satisfies the transform.Transformer interface.
//
// A byte that is not valid UTF-8 is written as U+FFFD, one for each run of such
// bytes, which is what strings.ToValidUTF8(s, "\uFFFD") produces.
func (c Collapser) Transform(dst, src []byte, atEOF bool) (int, int, error) {
	st := c.state
	if st == nil {
		st = new(collapserState)
	}
	var i, l, n int
	var r rune
	var spcbuf, b [utf8.UTFMax]byte
	spc := spcbuf[:utf8.EncodeRune(spcbuf[:], c.Spc)]
	s, d := len(src), len(dst)
	for ; i < s; i = i + l {
		r, l = utf8.DecodeRune(src[i:])
		invalid := r == utf8.RuneError && l == 1
		// the rest of the rune may be in the next call
		if invalid && !atEOF && !utf8.FullRune(src[i:]) {
			return n, i, transform.ErrShortSrc
		}
		// so may the rune the transformer is shown as next
		if c.Transformer != nil && !atEOF && !utf8.FullRune(src[i+l:]) {
			return n, i, transform.ErrShortSrc
		}
		if invalid && st.invalid {
			continue
		}
		space, remove := c.Space[r], c.Remove[r]
		// leading spaces end at the first rune that is kept or is a mark
		leading := c.Trim && !st.started
		st.started, st.invalid = st.started || invalid || !space && !remove, invalid
		switch {
		case space && (st.prev == c.Spc || leading), remove, unicode.Is(unicode.Mn, r):
			continue
		case space:
			r = c.Spc
		}
		if c.Transformer != nil {
			if r = c.Transformer(r, st.prev, rpeek(src, i+l, s)); r == -1 {
				continue
			}
		}
		if c.Lower {
			r = unicode.ToLower(r)
		}
		// hold a space back until a rune follows it, which trims the end
		if c.Trim && r == c.Spc {
			st.prev, st.spaces = r, st.spaces+1
			continue
		}
		j := utf8.EncodeRune(b[:], r)
		if d < n+st.spaces*len(spc)+j {
			// r is read again in the next call. started and invalid come
			// out the same then, and a first invalid byte is not skipped
			st.invalid = false
			return n, i, transform.ErrShortDst
		}
		for ; st.spaces != 0; st.spaces-- {
			n += copy(dst[n:], spc)
		}
		n += copy(dst[n:], b[:j])
		st.prev = r
	}
	// the text has ended: start afresh for a caller that does not Reset
	if atEOF {
		*st = collapserState{}
	}
	return n, i, nil
}

// Reset satisfies the transform.Transformer interface.
func (c Collapser) Reset() {
	if c.state != nil {
		*c.state = collapserState{}
	}
}

// rpeek
func rpeek(b []byte, i, n int) rune {
	if i < n {
		if r, _ := utf8.DecodeRune(b[i:]); r != utf8.RuneError {
			return r
		}
	}
	return 0
}
