// Package rls parses release information.
package rls

import (
	"bytes"
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

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
	Alt      string

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

	Series  int
	Episode int
	Version string
	Disc    string

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

	tags   []Tag
	dates  []int
	unused []int
	end    int
	// absolute is the episode Absolute returns, and isAbsolute whether there
	// is one.
	absolute   int
	isAbsolute bool
}

// Parse creates a release from src.
func Parse(src []byte) Release {
	return DefaultParser.ParseRelease(src)
}

// ParseString creates a release from src
func ParseString(src string) Release {
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

// SeriesEpisodes returns all series and episodes. The episodes inside a range
// are included, up to maxRangeFill of them. A written episode 0 is an episode
// ('S01E00', '[Grp] Show - 00-12'), and a season with no episode gives no
// pair ('S01').
func (r Release) SeriesEpisodes() [][]int {
	var v [][]int
	last, fill, none := -1, maxRangeFill, 0
	for i, tag := range r.tags {
		if !tag.Is(TagTypeSeries) {
			continue
		}
		series, _ := tag.Series()
		// a tag that writes no season is in the season written before it,
		// which a written season 0 is not ('S04E29 ... S00E36')
		bare := tag.val(1) == ""
		for k, episode := range tag.episodes(&fill) {
			s := series
			if bare && last != -1 {
				s, _ = r.tags[last].Series()
			}
			// a hyphen between two episode tags of one season makes a range
			// ('E02-E04', 'S00E02-S00E04'), which runs from the last episode
			// of the tag before the hyphen
			if k == 0 && i >= 2 && r.tags[i-2].Is(TagTypeSeries) && (bare || tag.val(1) == r.tags[i-2].val(1)) && fmt.Sprintf("%o", r.tags[i-1]) == "-" {
				if prev := r.tags[i-2].episodes(&none); len(prev) != 0 {
					if first := prev[len(prev)-1] + 1; episode-first <= fill {
						for j := first; j < episode; j++ {
							v = append(v, []int{s, j})
						}
						fill -= max(episode-first, 0)
					}
				}
			}
			v = append(v, []int{s, episode})
		}
		if !bare {
			last = i
		}
	}
	sort.Slice(v, func(i, j int) bool {
		if v[i][0] == v[j][0] {
			return v[i][1] < v[j][1]
		}
		return v[i][0] < v[j][0]
	})
	return v
}

// Absolute returns the episode when the name numbers its episodes absolutely
// and not within a season, ie '[Group] One Piece - 1169'. Episode carries the
// same number. What Absolute adds is that no season qualifies it, so it is not
// comparable against a season's episode without an external mapping (AniDB,
// anime-relations, TheTVDB). ok is false for a season relative number ('Hell
// Mode S2 - 04') and for a special ('Show.S00E05'), both of which are already
// qualified. A written episode 0 is absolute like any other: '[Group] Show -
// 00' gives 0 and true, and a name with no episode 0 and false.
func (r Release) Absolute() (episode int, ok bool) {
	return r.absolute, r.isAbsolute
}

// HasSeason reports whether the name gives a season, which season 0 is
// ('S00E05'). The Series field cannot tell a season 0 from none, and
// HasSeason answers from the tags. A Release built by hand with a Series has
// a season too.
func (r Release) HasSeason() bool {
	if r.Series != 0 {
		return true
	}
	for _, tag := range r.tags {
		if tag.typ == TagTypeSeries && tag.val(1) != "" {
			return true
		}
	}
	return false
}

// HasEpisode reports whether the name gives an episode, which episode 0 and
// the episodes of a range are. So 'S01E00', '[Grp] Show - 00' and the batch
// '[Grp] Show - 01-12' have an episode though Episode is 0, and the season
// pack 'Show.S01' has none.
func (r Release) HasEpisode() bool {
	if r.Episode != 0 {
		return true
	}
	for _, tag := range r.tags {
		if tag.typ == TagTypeSeries && tag.wroteEpisode() {
			return true
		}
	}
	return false
}

// Seasons returns the seasons the name gives, each once and in order. Two
// seasons written with no episode and a bare hyphen between them are a range
// ('S01-S03'), and the seasons in between are included, up to maxRangeFill of
// them. A Release built by hand with a Series and no tags gives that season.
func (r Release) Seasons() []int {
	var v []int
	fill := maxRangeFill
	for i, tag := range r.tags {
		if tag.typ != TagTypeSeries || tag.val(1) == "" {
			continue
		}
		season := atoi(tag.val(1))
		// the season before starts a range when its text ends in the hyphen
		// ('S01-' of 'S01-S03'), or when a hyphen alone stands between the two
		// ('S01 - S03')
		var start Tag
		switch {
		case i >= 2 && r.tags[i-1].typ == TagTypeDelim && strings.Trim(r.tags[i-1].val(0), "._ ") == "-":
			start = r.tags[i-2]
		case i >= 1 && strings.HasSuffix(r.tags[i-1].val(0), "-"):
			start = r.tags[i-1]
		}
		if start.typ == TagTypeSeries && start.val(1) != "" && !start.wroteEpisode() && !tag.wroteEpisode() {
			if prev := atoi(start.val(1)); prev < season-1 && season-prev-1 <= fill {
				for j := prev + 1; j < season; j++ {
					v = append(v, j)
				}
				fill -= season - prev - 1
			}
		}
		v = append(v, season)
	}
	if v == nil && r.Series != 0 {
		return []int{r.Series}
	}
	slices.Sort(v)
	return slices.Compact(v)
}

// EpisodeRange returns the first and last episode when the name gives more
// than one ('S01E05E06', 'S01E02-04', '01-12'). The episodes are those of the
// first season the name gives, the one in Series: 'S01E01.S02E01' is not a
// range. When a tag writes a season and an episode together ('S23E19'), an
// episode written without a season counts only as an 'E06' continuation, so
// 'S23E19.Episode.1174' is one episode under two numberings. The same episode
// written twice is one episode. The ends of a range come back however wide it
// is.
func (r Release) EpisodeRange() (first, last int, ok bool) {
	scheme := false
	for _, tag := range r.tags {
		if tag.typ == TagTypeSeries && tag.val(1) != "" && tag.wroteEpisode() {
			scheme = true
			break
		}
	}
	// cur is the season the nearest earlier tag wrote, which an episode
	// written without one is in
	cur, season, found := 0, 0, false
	for _, tag := range r.tags {
		if tag.typ != TagTypeSeries {
			continue
		}
		s := cur
		switch {
		case tag.val(1) != "":
			s = atoi(tag.val(1))
			cur = s
		case scheme && !tag.continuation():
			continue
		}
		for _, b := range tag.rest(2) {
			if b == "" || b == "-" {
				continue
			}
			episode := atoi(b)
			switch {
			case !found:
				season, first, last, found = s, episode, episode, true
			case s == season:
				first, last = min(first, episode), max(last, episode)
			}
		}
	}
	if !found || first == last {
		return 0, 0, false
	}
	return first, last, true
}

// EpisodeSpan is one episode, or one range of episodes, as a name writes it.
// Season is the season the name gives for it, and empty when it gives none.
// First is the episode. Last is the end of a range, and empty for one episode.
// The numbers are the digits of the name, so a leading zero and half an
// episode are kept: '21.5', '06.5', '00'.
type EpisodeSpan struct {
	Season string
	First  string
	Last   string
}

// EpisodeSpans returns the episodes the name gives, in the order it gives
// them, with a range as its two ends. It tells half an episode from the whole
// one ('21.5' and '21' are both Episode 21) and an episode 0 from none. A
// season pack gives no span.
//
// An episode written with no season is in the season written before it, as in
// SeriesEpisodes: the '04' of 'S2 - 04', and also a second numbering of the
// same episode ('S23E19.Episode.1174' gives 1174 in season 23). An episode
// written twice is listed twice. A hyphen or a tilde inside a tag makes a
// range ('S01E02-04', '01 ~ 12'), and so does a bare hyphen between two tags
// of one season when the end is the higher number ('E02-E04'). A Release built
// by hand with an Episode and no episode in its tags gives that episode.
func (r Release) EpisodeSpans() []EpisodeSpan {
	var v []EpisodeSpan
	var season string
	// open is whether the tag before wrote the last span, which a hyphen may
	// then extend
	var open bool
	for i, tag := range r.tags {
		if tag.typ != TagTypeSeries {
			continue
		}
		bare := tag.val(1) == ""
		if !bare {
			season = tag.val(1)
		}
		eps := tag.rest(2)
		ranged := open && i >= 2 && r.tags[i-2].typ == TagTypeSeries && r.tags[i-1].val(0) == "-" &&
			(bare || tag.val(1) == r.tags[i-2].val(1))
		open = false
		for k, b := range eps {
			switch b {
			case "":
				continue
			case "-":
				ranged = open
				continue
			}
			// the fraction follows the last number of the tag
			if k == len(eps)-1 {
				b += tag.fraction()
			}
			if last := len(v) - 1; ranged && wholeEpisode(b) > wholeEpisode(cmp.Or(v[last].Last, v[last].First)) {
				v[last].Last = b
			} else {
				v = append(v, EpisodeSpan{Season: season, First: b})
			}
			ranged, open = false, true
		}
	}
	if len(v) == 0 && r.Episode != 0 {
		v = append(v, EpisodeSpan{First: strconv.Itoa(r.Episode)})
		if r.Series != 0 {
			v[0].Season = strconv.Itoa(r.Series)
		}
	}
	return v
}

// wholeEpisode returns the whole number of an episode as EpisodeSpans gives
// it, ie 21 for '21.5'.
func wholeEpisode(s string) int {
	s, _, _ = strings.Cut(s, ".")
	return atoi(s)
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
// The lexers leave the part in the text of the tag, behind the last episode,
// and it is read from there.
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
// REREPACK or REPACK2. It reads Other, where the parser puts them as REPACK and
// REREPACK, and compares without regard to case. A re-rip (RERiP) is not a
// repack.
func (r Release) IsRepack() bool {
	return r.hasOther("REPACK", "REREPACK")
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
	for i := 0; i < len(b); i++ {
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

// ParseTags parses tags from src.
func ParseTags(src []byte) ([]Tag, int) {
	return DefaultParser.Parse(src)
}

// ParseTagsString parses tags from src.
func ParseTagsString(src string) ([]Tag, int) {
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
	for _, typ := range types {
		if tag.typ == typ {
			return true
		}
	}
	return false
}

// Was returns true when the tag's previous type is one of types. A tag that
// was never retyped reports TagTypeWhitespace, as Prev does. Retyped tells
// the two apart.
func (tag Tag) Was(types ...TagType) bool {
	prev := tag.Prev()
	for _, typ := range types {
		if prev == typ {
			return true
		}
	}
	return false
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
		buf = append(buf, fmt.Sprintf("%s:%q", tag.typ, tag.rest(1))...)
	case 'e':
		s := strconv.Quote(tag.Normalize())
		buf = append(buf, "<"+tag.typ.String()+":"+s[1:len(s)-1]+">"...)
	case 's', 'r':
		buf = append(buf, tag.Normalize()...)
	}
	_, _ = f.Write(buf)
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
	for _, t := range types {
		if typ == t {
			return true
		}
	}
	return false
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
	for _, t := range types {
		if typ == t {
			return true
		}
	}
	return false
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
var DefaultParser Parser

func init() {
	for i := Unknown; i <= Series; i++ {
		taginfo.RegisterType(i.String(), int(i))
	}
	DefaultBuilder = NewTagBuilder()
	DefaultParser = NewDefaultParser()
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
// the precedence defined in CompareMap.
func Compare(a, b Release) int {
	var c int
	for _, f := range []func() int{
		compareInt(CompareMap[a.Type], CompareMap[b.Type]),
		compareTitle(a.Artist, b.Artist),
		compareTitle(a.Title, b.Title),
		compareInt(a.Year, b.Year),
		compareInt(a.Month, b.Month),
		compareInt(a.Day, b.Day),
		compareInt(a.Series, b.Series),
		compareInt(a.Episode, b.Episode),
		compareTitle(a.Subtitle, b.Subtitle),
		compareTitle(a.Alt, b.Alt),
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
		if len(av) > 0 && len(bv) > 0 && av[0] == bv[0] && contains([]string{"a", "an", "the"}, av[0]) {
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
