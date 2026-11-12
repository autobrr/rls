package rls

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/autobrr/rls/reutil"
	"github.com/autobrr/rls/taginfo"
)

// LexFunc is the signature for lexer funcs.
//
// The first two arguments are the name and its working copy. The lexers of
// this package make tags whose values point into those two slices where the
// value is a piece of one of them, and copy a value only when it is not. So
// the slices must not change while the tags are in use:
//
//   - A TagParser gives its lexers slices of its own, copied from what the
//     caller gave. A lexer of a caller's that runs in one must not write to
//     either slice, since that would change tags made before it.
//   - A Parser of another kind that calls the lexers of this package must
//     give them slices it does not reuse or change afterwards.
//
// A lexer of a caller's that makes its tags with NewTag gets copies, as before.
type LexFunc func([]byte, []byte, []Tag, []Tag, int, int) ([]Tag, []Tag, int, int, bool)

// Lexer is the interface for lexers.
type Lexer interface {
	Initialize(map[string][]*taginfo.Taginfo, *regexp.Regexp, map[string]bool) (LexFunc, bool, bool)
}

// TagLexer is a tag lexer.
type TagLexer struct {
	Init     func(map[string][]*taginfo.Taginfo, *regexp.Regexp, map[string]bool)
	Lex      LexFunc
	NotFirst bool
	Once     bool
}

// Init satisfies the Lexer interface.
func (lexer TagLexer) Initialize(infos map[string][]*taginfo.Taginfo, delim *regexp.Regexp, short map[string]bool) (LexFunc, bool, bool) {
	if lexer.Init != nil {
		lexer.Init(infos, delim, short)
	}
	return lexer.Lex, lexer.Once, lexer.NotFirst
}

// DefaultLexers returns the default tag tag lexers.
func DefaultLexers() []Lexer {
	return []Lexer{
		// --------------- once ---------------
		NewTrimWhitespaceLexer(),
		NewExtLexer(),
		NewMetaLexer(
			// [[ type:value ]]
			``, `[[`, `]]`, `([a-zA-Z][a-zA-Z0-9_]{0,15}):\s*([^ \t\]]{1,32})`,
			// [REQ]
			`req`, `[`, `]`, `(REQ(?:UEST)?)`,
			// (REQ)
			`req`, `(`, `)`, `(REQ(?:UEST)?)`,
			// {REQ}
			`req`, `{`, `}`, `(REQ(?:UEST)?)`,
			// [ABCD1234], [abcd1234] -- crc32 checksums are written in either case
			`sum`, `[`, `]`, `((?i:[0-9A-F]{8}))`,
			// [site]
			`site`, `[`, `]`, `([^ \t\]]{1,32})`,
			// -={site}=-
			`site`, `-={`, `}=-`, `([^ \t\}]{1,32})`,
			// {{pass}}
			`pass`, `{{`, `}}`, `([^ \t\}]{1,32})`,
		),
		NewGroupLexer(),
		// --------------- multi ---------------
		NewRegexpLexer(TagTypeSize, true),
		NewRegexpLexer(TagTypePlatform, true),
		NewRegexpLexer(TagTypeArch, true),
		NewRegexpLexer(TagTypeSource, true),
		NewFrameRateLexer(
			// 1080p@59.94fps
			`(?i)^(?P<r>\d{3,4}[pi])@(?P<f>\d{2,3}\.\d{1,3})fps\b`,
			// 1080p50, 720p60fps, 1080i25, 1080p@60fps
			`(?i)^(?P<r>\d{3,4}[pi])@?(?P<f>\d{2,3})(?:fps)?\b`,
			// 60fps, 23.976fps
			`(?i)^(?P<f>\d{2,3}(?:\.\d{1,3})?)fps\b`,
		),
		NewRegexpLexer(TagTypeResolution, true),
		NewRegexpSourceLexer(TagTypeCollection, true),
		NewSeriesLexer(
			// s02, S01E01, S01E02-04, S01E02~04, S01E02v2, S01E06.5, S01E06.5v2
			`(?i)^s(?P<s>[0-8]?\d)[\-\._ ]?(?:e(?P<e>\d{1,5})(?:(?P<r>[\-~]\d{1,4})|(?P<x>\.\d)?(?P<v>v\d+)?)?)?\b`,
			// S90E01: a season from 90 up needs its episode to be read as one
			`(?i)^s(?P<s>9\d)[\-\._ ]?e(?P<e>\d{1,5})(?P<v>v\d+)?\b`,
			// S01E02E03, S01E02-E03, S01E03.E04.E05
			`(?i)^s(?P<s>[0-8]?\d)(?P<m>(?:[\-\._ ]?e\d{1,5}){1,5})\b`,
			// S01S02S03
			`(?i)^(?P<S>(?:s[0-8]?\d){2,4})\b`,
			// 2x1, 1x01
			`(?i)^(?P<s>[0-8]?\d)x(?P<e>\d{1,3})\b`,
			// S01 - 02v3, S07-06, s03-5v.9
			`(?i)^s(?P<s>[0-8]?\d)[\-\._ ]{1,3}(?P<e>\d{1,5})(?:[\-\._ ]{1,3}(?P<v>v\d+(?:\.\d+){0,2}))?\b`,
			// Season.01.Episode.02, Series.01.Ep.02, Series.01, Season.01,
			// Staffel.01, and Seasons.01 in front of the end of a range
			`(?i)^(?:series|seasons?|staffel|s)[\-\._ ]?(?P<s>[0-8]?\d)(?:[\-\._ ]?(?:episode|ep)(?P<e>\d{1,5}))?\b`,
			// 2nd Season, 3rd.Season
			`(?i)^(?P<s>[1-9]\d?)(?:st|nd|rd|th)[\-\._ ]season\b`,
			// Second Season, Third.Season
			`(?i)^(?P<o>first|second|third|fourth|fifth|sixth)[\-\._ ]season\b`,
			// Vol.1.No.2, vol1no2
			`(?i)^vol(?:ume)?[\-\._ ]?(?P<s>\d{1,3})(?:[\-\._ ]?(?:number|no)[\-\._ ]?(?P<e>\d{1,5}))\b`,
			// Episode 15, E009, Ep. 007, EP07, Ep07v2, Ep.05-07, Ep 05 ~ 07, Ep 5.5, EP1156-1170, Folge 15
			`(?i)^(?:e(?:p(?:isode)?[\-\._ ]{0,3})?|folge[\-\._ ]{0,3})(?P<e>\d{1,5})(?:(?P<v>v\d+)|(?P<x>\.\d)|(?P<r>[\._ ]{0,2}[\-~][\._ ]{0,2}\d{1,3}|[\-~]\d{4}))?\b`,
			// 10v1.7, 13v2, 24.5v2
			`(?i)^(?P<e>\d{1,5})(?P<x>\.\d)?(?P<v>v[\-\._ ]?\d+(?:\.\d){0,2})\b`,
			// S01.Disc02, s01D3, Series.01.Disc.02, S02DVD3
			`(?i)^(?:series|season|s)[\-\._ ]?(?P<s>[0-8]?\d)[\-\._ ]?(?P<d>(?:disc|disk|dvd|d)[\-\._ ]?(?:\d{1,3}))\b`,
			// s1957e01
			`(?i)^s(?P<s>(?:19|20)\d\d)e(?P<e>\d{2,4})\b`,
			// 2024x05: the year and round of an event. 1920 is the width of
			// a resolution
			`(?i)^(?P<s>19[5-9]\d|20[0-3]\d)x(?P<e>\d\d)\b`,
		),
		NewDiscSourceYearLexer(
			// VLS2004, 2DVD1999, 4CD2003
			`(?i)^(?P<d>[2-9])?(?P<s>cd|ep|lp|dvd|vls|vinyl)(?P<y>(?:19|20)\d\d)\b`,
			// WEB2007
			`(?i)^(?P<s>web)(?P<y>20\d\d)\b`,
		),
		NewDiscLexer(
			// D01, Disc.1
			`(?i)^(?P<t>d)(?:is[ck][\-\._ ])?(?P<c>\d{1,3})\b`,
			// 12DiSCS
			`(?i)^(?P<c>\d{1,3})[\-\._ ]?di(?P<t>s)[ck]s?\b`,
			// CD1, CD30
			`(?i)^(?P<t>cd)[\-\._ ]?(?P<c>\d{1,2})\b`,
			// DVD2, DVD24 -- does not match DVD5/DVD9
			`(?i)^(?P<t>dvd)[\-\._ ]?(?P<c>[1-46-8]|[12]\d)\b`,
			// 2xDVD9
			`(?i)^(?P<c>\d{1,2})(?P<t>x(?:dvd9))\b`,
			// 2DVD9, 6DVD9
			`(?i)^(?P<c>[2-9])(?P<z>dvd9)\b`,
			// 2xVinyl, 3xDVD, 4xCD
			`(?i)^(?P<c>\d{1,2})(?P<t>x(?:cd|ep|lp|dvda|dvd|vls|vinyl)s?)\b`,
			// 2Vinyl, 6DVD
			`(?i)^(?P<c>\d{1,2})(?P<x>(?:cd|ep|lp|dvda|dvd|vls|vinyl)s?)\b`,
			// CDS3
			`(?i)^(?:(?P<x>cd)s)(?P<c>\d{1,2})\b`,
			// 2CDS
			`(?i)^(?P<c>[2-9])(?P<x>cds)\b`,
		),
		NewDateLexer(
			// 2006-01-02
			`(?i)^(?P<2006>(?:19|20)\d{2})[\-\._ ](?P<01>\d{2})[\-\._ ](?P<02>\d{2})\b`,
			// 2006-2007, 2006-07, 2006/07, 1999–2003
			`(?i)^(?P<2006>(?:19|20)\d{2})(?:[\-/]|–)(?P<to>(?:19|20)?\d{2})\b`,
			// 2006
			`(?i)^(?P<2006>(?:19|20)\d{2})\b`,
			// 02-13-2006
			`(?i)^(?P<01>\d{2})[\-\._ ](?P<02>\d{2})[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			// 13-02-2006
			`(?i)^(?P<02>\d{2})[\-\._ ](?P<01>\d{2})[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			// 2nd Jan 2006, 13 Dec 2011, Nov 1999
			`(?i)^(?:(?P<_2>\d{1,2})(?:th|st|nd|rd)?[\-\._ ])?(?P<Jan>Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			// 01-August-1998
			`(?i)^(?P<_2>\d{1,2})[\-\._ ](?P<January>January|February|March|April|May|June|July|August|September|October|November|December)[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			// MAY-30-1992
			`(?i)^(?P<Jan>Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[\-\._ ](?P<_2>\d{1,2})[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			// 17.12.15 (a single digit month or day fails the date check)
			`(?i)^(?P<YY>[12]\d)[\-\._ ](?P<01>\d\d?)[\-\._ ](?P<02>\d\d?)\b`,
		),
		NewVersionLexer(
			// v1.17, v1, v1.2a, v1b
			`(?i)^(version[\-\._ ])?(?P<v>v[\-\._ ]?\d{1,2}(?:[\._ ]\d{1,2}[a-z]?\d*){0,3})\b`,
			// v2012, v20120803, v20120803, v1999.08.08
			`(?i)^(version[\-\._ ])?(?P<v>v[\-\._ ]?(?:19|20)\d\d(?:[\-\._ ]?\d\d?){0,2})\b`,
			// v60009
			`(?i)^(version[\-\._ ])?(?P<v>v[\-\._ ]?\d{4,10})\b`,
			// Version 2004, Version 21H2, Version 22H1
			`(?i)^version[\-\._ ](?P<V>\d{2,}|\d{2}[a-z]{1,2}\d{1,2})\b`,
			// 11.09.1, 100.000.99999999999, 23.3.2.458
			`(?i)^(?P<u>\d{1,3}\.\d{1,3}\.\d{1,16}(\.\d{1,16})?)\b`,
		),
		NewRegexpSourceLexer(TagTypeCodec, true),
		NewRegexpSourceLexer(TagTypeHDR, true),
		NewRegexpLexer(TagTypeBitDepth, true),
		NewAudioLexer(),
		NewRegexpLexer(TagTypeChannels, true),
		NewRegexpLexer(TagTypeOther, true),
		NewRegexpLexer(TagTypeCut, true),
		NewRegexpLexer(TagTypeEdition, true),
		NewRegexpLexer(TagTypeLanguage, false),
		NewRegexpLexer(TagTypeRegion, true),
		NewRegexpLexer(TagTypeContainer, true),
		NewGenreLexer(),
		NewEpisodeRangeLexer(),
		NewIDLexer(),
		NewEpisodeLexer(),
	}
}

// NewTrimWhitespaceLexer creates a tag lexer that matches leading and ending
// whitespace.
func NewTrimWhitespaceLexer() Lexer {
	s := `(\t|\n|\f|\r| |⭐|` + string(rune(0xfe0f)) + `)+`
	prefix := regexp.MustCompile(`^` + s)
	suffix := regexp.MustCompile(s + `$`)
	return TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// both patterns need a whitespace byte, or the first (0xe2, 0xef)
			// or last (0x90, 0x8f) byte of the star or its variation selector,
			// at the edge they are anchored to
			if i < n {
				switch src[i] {
				case '\t', '\n', '\f', '\r', ' ', 0xe2, 0xef:
					if m := prefix.FindSubmatch(src[i:n]); m != nil {
						start, i = append(start, newTag(src, buf, TagTypeWhitespace, nil, m...)), i+len(m[0])
					}
				}
			}
			if i < n {
				switch src[n-1] {
				case '\t', '\n', '\f', '\r', ' ', 0x90, 0x8f:
					if m := suffix.FindSubmatch(src[i:n]); m != nil {
						end, n = append(end, newTag(src, buf, TagTypeWhitespace, nil, m...)), n-len(m[0])
					}
				}
			}
			return start, end, i, n, true
		},
		Once: true,
	}
}

// NewDateLexer creates a tag lexer for a date.
func NewDateLexer(strs ...string) Lexer {
	// one lexer per pattern: a pattern can match text that is not a date in
	// its reading ('13.10.2024' as month 13), and the patterns after it must
	// still get their turn
	lexers := make([]*namedCapture, len(strs))
	for l, s := range strs {
		lexers[l] = newNamedCapture(s)
	}
	return withGate(TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			for _, lexer := range lexers {
				s, v, i, n, ok := lexer.lex(src, buf, i, n)
				if !ok {
					continue
				}
				// collect year, month, day
				var year, month, day, last []byte
				var y, m, d int
				matched := true
				for l := 0; l < len(v); l += 2 {
					f, t := string(v[l]), string(v[l+1])
					if f == "YY" {
						f, t = "2006", "20"+t
					}
					// a range of years ('2024-2025', '1999-2003') has the year
					// it starts in, and keeps the year it ends in. Written with
					// two digits ('2024-25') the second year has to be the
					// next one, and is not the month of a date ('2024-25-01')
					if f == "to" {
						to := atoi(t)
						switch {
						case len(t) == 4 && to > y:
						case len(t) == 2 && to == (y+1)%100 && !(i+1 < n && src[i] == '-' && src[i+1] >= '0' && src[i+1] <= '9'):
							to = y + 1
						default:
							matched = false
						}
						last = strconv.AppendInt(nil, int64(to), 10)
						continue
					}
					if t, err := time.Parse(f, t); err == nil {
						switch f {
						case "06", "2006":
							y, year = t.Year(), []byte(strconv.Itoa(t.Year()))
						case "_1", "01", "Jan", "January":
							m, month = int(t.Month()), fmt.Appendf(nil, "%02d", t.Month())
						case "_2", "02":
							d, day = t.Day(), fmt.Appendf(nil, "%02d", t.Day())
						default:
							panic(fmt.Errorf("unknown capture group %q", f))
						}
					} else {
						matched = false
					}
				}
				// the day has to exist in that month of that year
				if m != 0 && d != 0 && time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC).Day() != d {
					matched = false
				}
				if matched && last != nil {
					return append(
						start,
						newTag(src, buf, TagTypeDate, nil, s, year, month, day, last),
					), end, i, n, true
				}
				if matched {
					return append(
						start,
						newTag(src, buf, TagTypeDate, nil, s, year, month, day),
					), end, i, n, true
				}
			}
			return start, end, i, n, false
		},
	}, func() *lexGate { return namedCaptureGate(lexers...) })
}

// NewSeriesLexer creates a tag lexer for a series.
func NewSeriesLexer(strs ...string) Lexer {
	var sourcef *finder
	lexer := newNamedCapture(strs...)
	mlt := regexp.MustCompile(`(?i)s(\d?\d)`)
	mny := regexp.MustCompile(`(?i)[\-\._ ]?e(\d{1,5})`)
	dsc := regexp.MustCompile(`(?i)^(?:disc|disk|dvd|d)`)
	return withGate(TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			sourcef = newFinder(TagTypeSource, infos["source"]...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, i, n, ok := lexer.lex(src, buf, i, n); ok {
				// collect series, episode, version
				var series, episode, version, disc, multi, many, rng, dec []byte
				var ordinal bool
				for l := 0; l < len(v); l += 2 {
					switch string(v[l]) {
					case "s":
						series = v[l+1]
					case "e":
						episode = v[l+1]
					case "v":
						version = v[l+1]
					case "d":
						disc = v[l+1]
					case "S":
						multi = []byte(v[l+1])
					case "m":
						many = []byte(v[l+1])
					case "r":
						rng = v[l+1]
					case "x":
						dec = v[l+1]
					case "o":
						// the season as a word
						series, ordinal = ordinalSeason(v[l+1]), true
					default:
						panic(fmt.Errorf("unknown capture group %q", v[l]))
					}
				}
				// '2x05' is a season and episode after a title, but not when
				// it opens the name ('3x3.Eyes.1991') or a year follows it
				// ('Movie.5x5.2020'), where it belongs to the title
				if len(series) != 0 && len(s) > len(series) && (s[len(series)] == 'x' || s[len(series)] == 'X') {
					k := len(start) - 1
					for k >= 0 && start[k].Is(TagTypeDelim, TagTypeWhitespace, TagTypeMeta) {
						k--
					}
					if k < 0 || firstYearEnd(buf[i:min(n, i+6)]) != -1 {
						return start, end, i - len(s), n, false
					}
				}
				// 'Second Season' is a season after a title. It is a title
				// itself when it opens the name ('Second.Season.Blues.2005')
				// or goes on as a word ("Second Season's End")
				if ordinal {
					k := len(start) - 1
					for k >= 0 && start[k].Is(TagTypeDelim, TagTypeWhitespace, TagTypeMeta) {
						k--
					}
					if k < 0 || i < n && src[i] == '\'' {
						return start, end, i - len(s), n, false
					}
				}
				// '2nd Season' or 'Second Season' after a season or an episode
				// is the show's other name repeating it ('S00E28 (Show 2nd
				// Season OVA)')
				if len(series) != 0 && len(episode) == 0 && (ordinal || s[0] >= '1' && s[0] <= '9') && (s[len(s)-1] == 'n' || s[len(s)-1] == 'N') &&
					slices.ContainsFunc(start, func(tag Tag) bool { return tag.Is(TagTypeSeries) }) {
					return start, end, i - len(s), n, false
				}
				// 'EP1168' is an episode, but 'EP2004' is a music release and
				// its year, which the disc source year lexer reads
				if len(series) == 0 && len(episode) == 4 && len(s) > 5 && (s[1] == 'p' || s[1] == 'P') && s[2] == episode[0] &&
					(episode[0] == '1' && episode[1] == '9' || episode[0] == '2' && episode[1] == '0') {
					return start, end, i - len(s), n, false
				}
				// 'Folge 05' is an episode after 'Staffel 02'. On its own it
				// numbers the parts of an audio drama, whose names are music
				// releases ('Artist-Folge_225-Title-DE-2023-GRP'). And
				// 'Staffel 4 x 100m' is a relay
				if len(s) > 5 {
					switch s[0] {
					case 'f', 'F':
						k := len(start) - 1
						for k >= 0 && start[k].Is(TagTypeDelim, TagTypeWhitespace) {
							k--
						}
						if k < 0 || !start[k].Is(TagTypeSeries) || start[k].val(1) == "" || start[k].wroteEpisode() {
							return start, end, i - len(s), n, false
						}
					case 's', 'S':
						if k := i + 1; (s[1] == 't' || s[1] == 'T') && k+1 < n && (src[k] == 'x' || src[k] == 'X') && isAnyDelim(rune(src[i])) && isAnyDelim(rune(src[k+1])) {
							return start, end, i - len(s), n, false
						}
					}
				}
				var tags []Tag
				// a season with no episode and a higher number after a hyphen
				// or a tilde are the ends of a range of seasons ('S01-03',
				// 'Season 1-3', 'Seasons 1~7', '(Season 1 - 3)'), written as
				// two tags the way 'S01-S03' is, for Release.Seasons to fill
				// in.
				//
				// Higher numbers after a plus, an ampersand or a comma are the
				// other seasons of a list ('Season 1+2+3', 'S01+02', 'Seasons
				// 1 & 2'). Each is a tag of its own, and the text of a tag
				// runs up to the next number, so that it ends in what sets
				// the two apart.
				//
				// The plural is read in front of a range or a list and
				// nowhere else
				if len(series) != 0 && len(episode) == 0 && len(version) == 0 && len(disc) == 0 && len(multi) == 0 {
					plural := len(s) > 7 && (s[6] == 's' || s[6] == 'S') && (s[1] == 'e' || s[1] == 'E')
					if sep, last := seasonRangeEnd(src, s, series, i, n); last != 0 {
						tags = append(
							tags,
							newTag(src, buf, TagTypeSeries, nil, src[i-len(s):i+sep], series, nil),
							newTag(src, buf, TagTypeSeries, nil, src[i+sep:i+sep+last], src[i+sep:i+sep+last], nil),
						)
						return append(start, tags...), end, i + sep + last, n, true
					}
					// from is where the tag at hand starts, and j where its
					// number ends
					from, j, num := i-len(s), i, series
					if c := src[j-1]; c < '0' || c > '9' {
						j--
					}
					for {
						a, b := seasonListNext(src, num, j, n, plural)
						if b == 0 {
							break
						}
						// a number or a number word in front of the season
						// word makes it a season of its own or a title
						// ('Second Season 1+2', 'The Four Seasons 1 & 2', 'The
						// 4 Seasons 1 & 2'), which is looked at once, when a
						// list follows. A title can end in a number in front
						// of the singular ('Show 86 Season 1+2')
						if len(tags) == 0 {
							l := from
							for l > 0 && strings.IndexByte(" _.-)]", src[l-1]) != -1 {
								l--
							}
							if afterNumberWord(src, l) || plural && l > 0 && src[l-1] >= '0' && src[l-1] <= '9' {
								break
							}
						}
						tags = append(tags, newTag(src, buf, TagTypeSeries, nil, src[from:a], num, nil))
						from, j, num = a, b, src[a:b]
					}
					switch {
					case len(tags) != 0:
						tags = append(tags, newTag(src, buf, TagTypeSeries, nil, src[from:j], num, nil))
						return append(start, tags...), end, j, n, true
					case plural:
						return start, end, i - len(s), n, false
					}
				}
				// a range end ('E05-07') is the last episode of the range when
				// it is higher than the first. Only a bare hyphen or a tilde
				// is sure to mean a range, which the tag records as a '-'
				// between the two episodes for Tag.Episodes to fill in: across
				// ' - ' the two ends are all that is known. A dot and a digit
				// after the number make it a decimal instead, in a name that
				// separates its words with spaces ('E03 - 99.419 Specials').
				//
				// What is not a range end stays inside an 'Ep' tag, where no
				// other lexer can take it for an episode, and is handed back
				// after 'S01E02'.
				var more [][]byte
				if k := len(rng); k != 0 {
					tail := s[len(s)-k:]
					for k > 0 && tail[k-1] >= '0' && tail[k-1] <= '9' {
						k--
					}
					first, last := atoi(string(episode)), atoi(string(tail[k:]))
					decimal := bytes.IndexByte(tail[:k], ' ') != -1 && i+1 < n && src[i] == '.' && src[i+1] >= '0' && src[i+1] <= '9'
					// four digits end a range of episodes that are numbered
					// that high ('EP1156-1170', 'E998-1002'). After a lower
					// episode they are a year or another number
					// ('E05-2020', 'E05-1080'), and are handed back
					switch {
					case len(tail)-k == 4 && (len(episode) < 3 || last-first >= 1000):
						s, i = s[:len(s)-len(tail)], i-len(tail)
					case !decimal && last > first:
						if k == 1 && tail[0] == '-' || bytes.IndexByte(tail[:k], '~') != -1 {
							more = append(more, []byte{'-'})
						}
						more = append(more, tail[k:])
					case len(series) != 0:
						s, i = s[:len(s)-len(tail)], i-len(tail)
					}
				}
				// a decimal episode ('Ep 5.5', 'S01E06.5') stays inside the
				// tag, unless its digit begins the channels ('E05.5.1.AAC')
				// or, after a season and episode, a dot and a word follow it:
				// there the digit is the first word of the episode's title
				// ('S02E09.4.Days.Out')
				if len(dec) != 0 && len(version) == 0 && i+1 < n && src[i] == '.' {
					switch c := src[i+1]; {
					case c >= '0' && c <= '9' && (i+2 == n || src[i+2] < '0' || src[i+2] > '9'),
						len(series) != 0 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'):
						s, i = s[:len(s)-len(dec)], i-len(dec)
					}
				}
				// version and disc end the match. They were read from buf,
				// where '_', ',' and '+' are spaces, so their text is cut
				// from src by length
				vtext, dtext := version, disc
				if len(version) != 0 {
					s, vtext = s[:len(s)-len(version)], s[len(s)-len(version):]
				}
				if len(disc) != 0 {
					s, dtext = s[:len(s)-len(disc)], s[len(s)-len(disc):]
				}
				if len(series) != 0 || len(episode) != 0 {
					l := [][]byte{s, series}
					switch {
					case len(many) == 0:
						l = append(append(l, episode), more...)
					default:
						for _, m := range mny.FindAllSubmatch(many, -1) {
							l = append(l, m[1])
						}
					}
					tags = append(tags, newTag(src, buf, TagTypeSeries, nil, l...))
				}
				if len(version) != 0 {
					tags = append(tags, newTag(src, buf, TagTypeVersion, nil, vtext, version))
				}
				if len(disc) != 0 {
					disctyp := bytes.ToUpper(disc[:len(dsc.FindSubmatch(disc)[0])])
					num := bytes.TrimSpace(disc[len(disctyp):])
					switch string(disctyp) {
					case "DVD":
						tags = append(
							tags,
							newTag(src, buf, TagTypeSource, sourcef, dtext[:len(disctyp)], disctyp),
							newTag(src, buf, TagTypeDisc, nil, dtext[len(disctyp):], disctyp, num),
						)
					default:
						tags = append(tags, newTag(src, buf, TagTypeDisc, nil, dtext, disctyp, num))
					}
				}
				// S01S02S03 ...
				if m := mlt.FindAllSubmatch(multi, -1); m != nil {
					for j := 0; j < len(m); j++ {
						tags = append(tags, newTag(src, buf, TagTypeSeries, nil, append(m[j], nil)...))
					}
				}
				if len(series) == 4 && (bytes.HasPrefix(series, []byte{'1', '9'}) || bytes.HasPrefix(series, []byte{'2', '0'})) {
					tags = append(tags, newTag(src, buf, TagTypeDate, nil, nil, series, nil, nil))
				}
				return append(
					start,
					tags...,
				), end, i, n, true
			}
			return start, end, i, n, false
		},
	}, lexer.gate)
}

// ordinalSeason returns the number of a season written as a word, ie '2' for
// 'Second'.
func ordinalSeason(word []byte) []byte {
	for i, s := range [...]string{"first", "second", "third", "fourth", "fifth", "sixth"} {
		if strings.EqualFold(string(word), s) {
			return []byte{'1' + byte(i)}
		}
	}
	return nil
}

// seasonRangeEnd looks for the last season of a range after a season that
// was read up to i as s, and returns the length of what separates the two and
// of the number, or 0 and 0 when no range follows.
//
// The two are set apart by a hyphen or a tilde, and the last season is the
// higher number. What looks the same and is a season and its episode is left
// to the other patterns:
//
//   - a number padded with a zero after a season that is not ('S2-04')
//   - spaces around the hyphen ('Season 3 - 03', 'S01 - 04'), unless the
//     season is a word in a bracket and a ')' or a '+' ends the pair
//     ('(Season 1 - 3)', '(Season 01 - 04 + OVAs)')
//   - a number word in front of the season, which makes it a season of its
//     own or a title ('Second Season 03-06', 'The Four Seasons 1-4')
func seasonRangeEnd(src, s, series []byte, i, n int) (int, int) {
	space := func(j int) bool { return j < n && (src[j] == ' ' || src[j] == '_') }
	digit := func(j int) bool { return j < n && src[j] >= '0' && src[j] <= '9' }
	j, spaced := i, false
	switch c := s[len(s)-1]; {
	// the series pattern took the hyphen
	case c == '-':
	case c >= '0' && c <= '9':
		if space(j) {
			j, spaced = j+1, true
		}
		if j == n || src[j] != '-' && src[j] != '~' {
			return 0, 0
		}
		if j++; space(j) {
			j, spaced = j+1, true
		}
	default:
		return 0, 0
	}
	k := j
	for digit(k) {
		k++
	}
	// as a season is, the number is below 90
	if k == j || k-j > 2 || k-j == 2 && src[j] == '9' || atoi(string(src[j:k])) <= atoi(string(series)) || src[j] == '0' && series[0] != '0' {
		return 0, 0
	}
	// the number ends at a delimiter, and is not the start of a range of
	// episodes ('S1-01-12')
	if k < n {
		switch c := src[k]; {
		case c|0x20 >= 'a' && c|0x20 <= 'z', c >= 0x80, (c == '-' || c == '~') && digit(k+1):
			return 0, 0
		}
	}
	// the start of the season, less the spaces in front of it
	l := i - len(s)
	for l > 0 && (src[l-1] == ' ' || src[l-1] == '_' || src[l-1] == '.') {
		l--
	}
	if spaced {
		m := k
		for space(m) {
			m++
		}
		if m == n || src[m] != ')' && src[m] != '+' || s[1]|0x20 != 'e' || l == 0 || src[l-1] != '(' && src[l-1] != '[' {
			return 0, 0
		}
	}
	if afterNumberWord(src, l) {
		return 0, 0
	}
	return j - i, k - j
}

// afterNumberWord reports whether the word that ends at l in src is a number
// or an ordinal, which makes the season after it a season of its own or a
// title ('Second Season', 'The Four Seasons').
func afterNumberWord(src []byte, l int) bool {
	m := l
	for m > 0 && src[m-1]|0x20 >= 'a' && src[m-1]|0x20 <= 'z' {
		m--
	}
	// the longest of the words has seven letters
	if l-m < 3 || l-m > 7 {
		return false
	}
	for _, w := range [...]string{
		"first", "second", "third", "fourth", "fifth", "sixth", "seventh", "eighth", "ninth", "tenth", "final",
		"two", "three", "four", "five", "six", "seven", "eight", "nine", "ten",
	} {
		if len(w) == l-m && strings.EqualFold(w, string(src[m:l])) {
			return true
		}
	}
	return false
}

// seasonListNext looks for the next season of a list after a season whose
// number prev ends at j, and returns where the next number starts and ends in
// src, or 0 and 0 when the list does not go on. plural is whether the season
// the list starts with was written 'Seasons'.
//
// A plus, an ampersand or a comma sets the two apart, with or without spaces,
// and the next season is the higher number, below 90 as a season is. It is
// read when the list plainly goes on or ends after it:
//
//   - at a closing bracket, at the next plus, ampersand or comma, or at the
//     end of the name ('(Season 1 + 2)', 'Season 1+2+3', 'Season 1 & 2 +
//     Movie')
//   - in front of spaces or a dot and then one of those, an opening bracket, a
//     hyphen with a space after it or a resolution ('Season 1 + 2 + OVA',
//     'S01+02 (Complete)', 'Season 1 & 2 - Batch', 'Season 1 & 2 1080p')
//   - in front of spaces or a dot and then a word, when no space stands on
//     either side of what sets the number apart, or the number is padded with
//     a zero, and the word is not one it would count ('Season 1,2,3 Batch',
//     'S01 + 02 Batch', 'S01+02.WEB'), or when the seasons are in the plural
//     ('Seasons 1 & 2 Batch')
//
// Anything else after the number leaves it alone, since it may count the word
// after it or be the start of another number: 'Season 1 + 2 OVAs', 'S01+2
// Episodes', 'S01, 10 Bit', 'S01, 24.12.2020', 'S01+02-12', 'S01 & 5.1'. A
// unit in the place of the word does too ('S1,12 GB', 'Seasons 1+4 TiB'), and
// so does a zero in front of the number when the one before it has a single
// digit ('S2+04').
//
// When the rest of the name was read already and starts with a hyphen, the
// number is left alone as well ('S01+02-GRP'), as the season in front of a
// range is.
//
// A number word in front of the first season makes it no list at all ('The
// Four Seasons 1 & 2'), which is the caller's to check.
func seasonListNext(src, prev []byte, j, n int, plural bool) (int, int) {
	end := len(src)
	space := func(k int) bool { return k < end && (src[k] == ' ' || src[k] == '_') }
	digit := func(k int) bool { return k < end && src[k] >= '0' && src[k] <= '9' }
	letter := func(k int) bool { return k < end && (src[k]|0x20 >= 'a' && src[k]|0x20 <= 'z' || src[k] >= 0x80) }
	// resolution reports whether three or four digits and a 'p' or an 'i'
	// start at k
	resolution := func(k int) bool {
		m := k
		for digit(m) {
			m++
		}
		return (m-k == 3 || m-k == 4) && m < end && (src[m]|0x20 == 'p' || src[m]|0x20 == 'i')
	}
	k := j
	for space(k) {
		k++
	}
	if k >= n || src[k] != '+' && src[k] != '&' && src[k] != ',' {
		return 0, 0
	}
	// tight is whether no space stands on either side of it
	tight := k == j && !space(k+1)
	for k++; space(k); k++ {
	}
	a := k
	for k < n && digit(k) {
		k++
	}
	if k == a || k-a > 2 || k-a == 2 && src[a] == '9' || atoi(string(src[a:k])) <= atoi(string(prev)) || src[a] == '0' && len(prev) != 2 {
		return 0, 0
	}
	// m is where what follows the number starts
	m := k
	switch {
	case k == end:
		return a, k
	case k == n:
		// the rest of the name is read already, as the group when a hyphen
		// starts it ('S1+2-12')
		if src[k] == '-' {
			return 0, 0
		}
		return a, k
	case src[k] == ')', src[k] == ']', src[k] == '}', src[k] == '+', src[k] == '&':
		return a, k
	case src[k] == ',':
		// a comma and digits in front of a unit are the decimals of a size
		// ('12,5GB', '2,5 GB')
		for m++; digit(m); m++ {
		}
		if m == k+1 {
			return a, k
		}
		for space(m) {
			m++
		}
		if e := wordEnd(src, m); e != m && isUnit(src[m:e]) || m < end && letter(m) && m > 0 && digit(m-1) {
			return 0, 0
		}
		return a, k
	case src[k] == '.':
		m++
	case space(k):
		for space(m) {
			m++
		}
	default:
		return 0, 0
	}
	switch {
	case m == end, strings.IndexByte("()[]{}+&,", src[m]) != -1, src[m] == '-' && space(m+1), resolution(m):
		return a, k
	case !letter(m):
		return 0, 0
	}
	// a word: the number may count it
	word := src[m:wordEnd(src, m)]
	switch {
	case isUnit(word):
		return 0, 0
	case plural:
		return a, k
	case !tight && src[a] != '0', isCounted(word):
		return 0, 0
	}
	return a, k
}

// isCounted reports whether the word is one that a number in front of it
// counts, in a name that lists what a batch holds: a word in the plural ('2
// OVAs', '24 Episodes'), or one of those that are written in the singular too
// ('2 OVA', '25 Part 2').
func isCounted(word []byte) bool {
	if n := len(word); n > 2 && word[n-1]|0x20 == 's' {
		return true
	}
	for _, w := range [...]string{"ova", "oad", "ona", "oav", "movie", "film", "special", "episode", "ep", "part", "disc", "cd", "dvd", "bd", "pv"} {
		if len(w) == len(word) && strings.EqualFold(w, string(word)) {
			return true
		}
	}
	return false
}

// wordEnd returns the end of the run of ASCII letters that starts at i in
// src.
func wordEnd(src []byte, i int) int {
	for i < len(src) && src[i]|0x20 >= 'a' && src[i]|0x20 <= 'z' {
		i++
	}
	return i
}

// isUnit reports whether the word is a unit that a number in front of it is
// measured in, ie the 'GB' of '12 GB' or the 'bit' of '10 bit'.
func isUnit(word []byte) bool {
	if len(word) < 2 || len(word) > 7 {
		return false
	}
	for _, u := range [...]string{
		"gb", "mb", "kb", "tb", "gib", "mib", "kib", "tib", "bit", "bits", "fps", "hz", "khz", "kbps", "mbps", "ch",
		"min", "mins", "minutes", "hr", "hrs", "hour", "hours",
	} {
		if len(u) == len(word) && strings.EqualFold(u, string(word)) {
			return true
		}
	}
	return false
}

// NewIDLexer creates a tag lexer for a music id.
func NewIDLexer() Lexer {
	alpha, digit, ws := regexp.MustCompile(`[A-Z]`), regexp.MustCompile(`\d`), regexp.MustCompile(`[\-\._ ]`)
	// the id pattern is ^([A-Z\d\-\_\. ]{2,24})\)
	var idLead [256]bool
	for _, c := range []byte(`ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_. `) {
		idLead[c] = true
	}
	re := regexp.MustCompile(`^([A-Z\d\-\_\. ]{2,24})\)`)
	return withGate(TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// as in the episode lexer, rule out the position on its first byte
			// before scanning the prefix for the lookbehind
			if i < n && !idLead[buf[i]] {
				return start, end, i, n, false
			}
			// lookbehind: \([\._ ]{0,2}$
			if behind(src[:i], '(', "._ ", 0, 2) {
				if m := re.FindSubmatch(buf[i:n]); m != nil {
					a, d, w := alpha.FindAllSubmatch(m[1], -1), digit.FindAllSubmatch(m[1], -1), ws.FindAllSubmatch(m[1], -1)
					switch {
					// ensure enough at least # of alphanumerics
					case a == nil && len(d) > 4 && len(w) < 4,
						len(a) > 1 && len(d) > 1 && (len(a)+len(d) > 4) && len(w) < 4:
						return append(start, newTag(src, buf, TagTypeID, nil, src[i:i+len(m[0])], m[1])), end, i + len(m[0]), n, true
					}
				}
			}
			return start, end, i, n, false
		},
	}, func() *lexGate { return leadGate(idLead, true) })
}

// NewEpisodeLexer creates a tag lexer for a single episode (`- 2 -`, `- 867 (`, `- 100 [`).
func NewEpisodeLexer() Lexer {
	// the number, half an episode ('21.5') and what ends it
	re := regexp.MustCompile(`^(\d{1,4})(\.\d{1,2})?(\b|[\._ ]?[\-\[\]\(\)\{\}])`)
	return withGate(TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// the match has to start with a digit, and that is far cheaper to
			// rule out than the lookbehind below, which scans the whole prefix
			if i < n && (src[i] < '0' || src[i] > '9') {
				return start, end, i, n, false
			}
			// compare against src, and match "lookbehind": -[\-\._ ]{1,3}$
			if behind(src[:i], '-', "-._ ", 1, 3) {
				if m := re.FindSubmatch(src[i:n]); m != nil && !bytes.HasPrefix(src[i+len(m[1]):], []byte{','}) {
					tags := []Tag{newTag(src, buf, TagTypeSeries, nil, src[i:i+len(m[1])+len(m[2])], nil, m[1])}
					if len(m[3]) != 0 {
						tags = append(tags, newTag(src, buf, TagTypeDelim, nil, m[3], m[3]))
					}
					return append(start, tags...), end, i + len(m[0]), n, true
				}
			}
			return start, end, i, n, false
		},
	}, func() *lexGate { return bytesGate(digitBytes) })
}

// NewEpisodeRangeLexer creates a tag lexer for the episodes of a batch, written
// as a range of bare numbers: '(01-12)', '[01-12]', '- 01 ~ 12', '- 01-12' and
// 'Title 10-12 [1080p]'.
//
// The tag has no episode and the two ends of the range after it, so the
// release is a pack and not its first episode: Episode stays 0, and
// Tag.Episodes and Release.SeriesEpisodes give the episodes in the range.
func NewEpisodeRangeLexer() Lexer {
	re := regexp.MustCompile(`^(\d{1,4})( ?[\-~] ?)(\d{1,4})`)
	year := func(b []byte) bool {
		return len(b) == 4 && (b[0] == '1' && b[1] == '9' || b[0] == '2' && b[1] == '0')
	}
	return withGate(TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if i == 0 || i >= n || src[i] < '0' || src[i] > '9' {
				return start, end, i, n, false
			}
			m := re.FindSubmatch(src[i:n])
			if m == nil || year(m[1]) || year(m[3]) || atoi(string(m[1])) >= atoi(string(m[3])) {
				return start, end, i, n, false
			}
			// what follows the range, and whether the hyphen is a bare one.
			// closer is the byte after it in the whole name, which is the
			// bracket when the lexers that read the end of the name have
			// cut it off ('[Grp] Show [01-12]', 'Show (01-12).mkv')
			var next, closer byte
			if j := i + len(m[0]); j < n {
				next = src[j]
			}
			if j := i + len(m[0]); j < len(src) {
				closer = src[j]
			}
			tight, free := len(m[2]) == 1 && m[2][0] == '-', next == 0 || next == ' ' || next == '_' || next == '[' || next == '('
			var ok bool
			switch prev := src[i-1]; {
			// (01-12), [01-12]
			case prev == '(' && closer == ')', prev == '[' && closer == ']':
				ok = true
			// - 01 ~ 12, - 01-12, and not '- 1 - 12', where a title that
			// ends in a number is followed by an episode
			case behind(src[:i], '-', "-._ ", 1, 3):
				ok = free && (tight || bytes.IndexByte(m[2], '~') != -1)
			// Title 10-12 [1080p]
			case (prev == ' ' || prev == '_') && i > 1:
				c := src[i-2]
				ok = (c|0x20 >= 'a' && c|0x20 <= 'z' || c == '!' || c == '?') &&
					free && tight && len(m[1]) == len(m[3]) && len(m[1]) > 1
			}
			if !ok {
				return start, end, i, n, false
			}
			return append(start, newTag(src, buf, TagTypeSeries, nil, m[0], nil, nil, m[1], []byte{'-'}, m[3])), end, i + len(m[0]), n, true
		},
	}, func() *lexGate { return bytesGate(digitBytes) })
}

// NewVersionLexer creates a tag lexer for a version.
func NewVersionLexer(strs ...string) Lexer {
	lexer := newNamedCapture(strs...)
	return withGate(TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, i, n, ok := lexer.lex(src, buf, i, n); ok {
				var version []byte
				for l := 0; l < len(v); l += 2 {
					switch string(v[l]) {
					case "v":
						version = bytes.ToLower(s)
						if bytes.HasPrefix(version, []byte("version")) {
							version = version[len("version")+1:]
						}
						version = bytes.Replace(version, []byte{' '}, []byte{'.'}, -1)
					case "V":
						version = v[l+1]
					case "u":
						version = append([]byte{'v'}, v[l+1]...)
					default:
						panic(fmt.Errorf("unknown capture group %q", v[l]))
					}
				}
				return append(start, newTag(src, buf, TagTypeVersion, nil, s, version)), end, i, n, true
			}
			return start, end, i, n, false
		},
		NotFirst: true,
	}, lexer.gate)
}

// NewDiscSourceYearLexer creates a tag lexer for the combined disc, source,
// year style tag.
func NewDiscSourceYearLexer(strs ...string) Lexer {
	var sourcef *finder
	lexer := newNamedCapture(strs...)
	return withGate(TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			sourcef = newFinder(TagTypeSource, infos["source"]...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if _, v, i, n, ok := lexer.lex(src, buf, i, n); ok {
				// collect series, episode, version
				var disc, source, year []byte
				for l := 0; l < len(v); l += 2 {
					switch string(v[l]) {
					case "d":
						disc = v[l+1]
					case "s":
						source = v[l+1]
					case "y":
						year = v[l+1]
					default:
						panic(fmt.Errorf("unknown capture group %q", v[l]))
					}
				}
				var tags []Tag
				if len(disc) != 0 {
					tags = append(tags, newTag(src, buf, TagTypeDisc, nil, disc, []byte{'X'}, disc))
				}
				tags = append(
					tags,
					newTag(src, buf, TagTypeSource, sourcef, source, source),
					newTag(src, buf, TagTypeDate, nil, year, year, nil, nil),
				)
				return append(start, tags...), end, i, n, true
			}
			return start, end, i, n, false
		},
	}, lexer.gate)
}

// NewFrameRateLexer creates a tag lexer for a frame rate, written on its own
// ('60fps', '23.976fps') or onto a resolution ('1080p50', '720p60fps'), which
// gives the two tags.
//
//	r - resolution
//	f - frames per second
func NewFrameRateLexer(strs ...string) Lexer {
	var resolutionf *finder
	lexer := NamedCaptureLexer(strs...)
	return withGate(TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			resolutionf = newFinder(TagTypeResolution, infos["resolution"]...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// the patterns are digits and then 'fps', or a 'p' or 'i' with a
			// digit after it or after an '@', which few of the numbers in a
			// name have. The longest number in them is 7 bytes ('119.880'),
			// and looking no further keeps a long run of digits linear
			k := i
			for k < n && k-i < 8 && (buf[k] >= '0' && buf[k] <= '9' || buf[k] == '.') {
				k++
			}
			if k == i || k+1 >= n {
				return start, end, i, n, false
			}
			switch buf[k] | 0x20 {
			case 'f':
			case 'p', 'i':
				if l := k + 1; buf[l] == '@' && l+1 < n {
					k = l
				}
				if buf[k+1] < '0' || buf[k+1] > '9' {
					return start, end, i, n, false
				}
			default:
				return start, end, i, n, false
			}
			s, v, j, m, ok := lexer(src, buf, i, n)
			if !ok {
				return start, end, i, n, false
			}
			var resolution, rate []byte
			for l := 0; l < len(v); l += 2 {
				switch string(v[l]) {
				case "r":
					resolution = v[l+1]
				case "f":
					rate = v[l+1]
				default:
					panic(fmt.Errorf("unknown capture group %q", v[l]))
				}
			}
			// a rate is 23.976 and up: the 10 in '1080p10' is a bit depth.
			// With no 'fps' after it only a common rate is one
			fps, err := strconv.ParseFloat(string(rate), 64)
			if err != nil || fps < 23 || fps > 240 {
				return start, end, i, n, false
			}
			if c := s[len(s)-1]; c != 's' && c != 'S' {
				switch fps {
				case 24, 25, 30, 48, 50, 60, 90, 100, 120, 144, 240:
				default:
					return start, end, i, n, false
				}
			}
			if len(resolution) != 0 {
				if resolutionf.find(string(resolution)) == nil {
					return start, end, i, n, false
				}
				text := s[:len(resolution)]
				start, s = append(start, newTag(src, buf, TagTypeResolution, resolutionf, text, text)), s[len(resolution):]
			}
			return append(start, newTag(src, buf, TagTypeFrameRate, nil, s, rate)), end, j, m, true
		},
		// the lexer looks for a run of digits and periods first
	}, func() *lexGate { return bytesGate(digitBytes + ".") })
}

// NewDiscLexer creates a tag lexer for a disc.
//
//	n - number
//	t - type
//	x - xXtype
func NewDiscLexer(strs ...string) Lexer {
	var sourcef, sizef *finder
	lexer, re := newNamedCapture(strs...), regexp.MustCompile(`(?i)^(?:dvd|cd|d|s|x)`)
	return withGate(TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			sourcef, sizef = newFinder(TagTypeSource, infos["source"]...), newFinder(TagTypeSize, infos["size"]...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, j, k, ok := lexer.lex(src, buf, i, n); ok {
				var typ, c, x, z []byte
				for l := 0; l < len(v); l += 2 {
					switch string(v[l]) {
					case "c":
						c = v[l+1]
					case "t":
						typ = bytes.ToUpper(v[l+1])
					case "x":
						x = bytes.ToUpper(v[l+1])
					case "z":
						typ, z = []byte("Z"), bytes.ToUpper(v[l+1])
					default:
						panic(fmt.Errorf("unknown capture group %q", v[l]))
					}
				}
				// the type is the keyword its capture begins with ('x' of
				// 'xdvd9')
				if m := re.Find(typ); m != nil {
					typ = typ[:len(m)]
				}
				switch string(typ) {
				case "D", "S":
					return append(start, newTag(src, buf, TagTypeDisc, nil, s, typ, c)), end, j, k, true
				case "DVDA", "DVD", "CD":
					return append(start,
						newTag(src, buf, TagTypeSource, sourcef, s[:len(typ)], typ, typ),
						newTag(src, buf, TagTypeDisc, nil, s[len(typ):], typ, c),
					), end, j, k, true
				case "X":
					if sz := strings.ToUpper(string(s[len(c)+1:])); sz == "DVD9" {
						return append(start,
							newTag(src, buf, TagTypeDisc, nil, s[:len(c)+1], typ, c),
							newTag(src, buf, TagTypeSize, sizef, s[len(c)+1:], s[len(c)+1:]),
						), end, j, k, true
					}
					return append(start,
						newTag(src, buf, TagTypeDisc, nil, s[:len(c)+1], typ, c),
						newTag(src, buf, TagTypeSource, sourcef, s[len(c)+1:], s[len(c)+1:]),
					), end, j, k, true
				case "Z":
					return append(start,
						newTag(src, buf, TagTypeDisc, nil, s[:len(c)+1], []byte{'X'}, c),
						newTag(src, buf, TagTypeSize, sizef, s[len(c)+1:], z),
					), end, j, k, true
				case "":
					return append(start,
						newTag(src, buf, TagTypeDisc, nil, s[:len(c)+1], []byte{'X'}, c),
						newTag(src, buf, TagTypeSource, sourcef, s[len(c)+1:], x),
					), end, j, k, true
				default:
					panic(fmt.Errorf("unknown type %q", typ))
				}
			}
			return start, end, i, n, false
		},
	}, lexer.gate)
}

// NewAudioLexer creates a tag lexer for audios.
func NewAudioLexer() Lexer {
	var re *regexp.Regexp
	var audiof, channelsf *finder
	var lead [256]bool
	var gated bool
	var pf *prefilter
	// next holds the bytes that can follow the audio tag's first run inside
	// the same run: a channel digit or the start of an attached codec
	var next [256]bool
	return withGate(TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			audio, channels := infos["audio"], infos["channels"]
			if p, ok := newPrefilter(audio, true); ok && !p.hasLead {
				var cl [256]bool
				if a, b := leadSet(reutil.Taginfo(``, channels...)); b {
					if c, d := leadSet(reutil.Taginfo(``, infos["codec"]...)); d {
						for x := 0; x < 256; x++ {
							cl[x] = a[x] || c[x]
						}
						pf, next = p, cl
					}
				}
			}
			var v []string
			for _, info := range channels {
				v = append(v, strings.ReplaceAll(info.Tag(), `.`, `[\._ ]?`))
			}
			codec := reutil.Taginfo(``, infos["codec"]...)
			re = regexp.MustCompile(reutil.Taginfo(`^i`, audio...) + `(?:[\-\._ ]?(` + strings.Join(v, "|") + `))?(?:\b|[\-\._ ]|(` + codec + `(?:\b|[\-\._ ]))` + plusEnd + `)`)
			audiof, channelsf = newFinder(TagTypeAudio, audio...), newFinder(TagTypeChannels, channels...)
			lead, gated = leadSet(re.String())
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// NB: gated on the first byte only. The run based prefilter is not
			// sound here: the channels suffix is optional but greedy, so a
			// match's first run can span audio and channel digits ('DDP5.1' ->
			// 'DDP5'), which the audio rows alone cannot produce.
			if gated && !lead[src[i]] {
				return start, end, i, n, false
			}
			if !pf.maybePrefix(src, i, n, &next) {
				return start, end, i, n, false
			}
			if m := re.FindSubmatch(src[i:n]); m != nil {
				// 'ES' and 'EX' extend the audio tag before them ('DTS-ES',
				// 'DD5.1.EX'). On their own they are a language or a word.
				if len(m[1]) == 2 && (string(m[1]) == "ES" || string(m[1]) == "EX") {
					k := len(start) - 1
					for k >= 0 && start[k].Is(TagTypeDelim, TagTypeWhitespace) {
						k--
					}
					if k < 0 || !start[k].Is(TagTypeAudio, TagTypeChannels) {
						return start, end, i, n, false
					}
				}
				// Leave an attached codec and its delimiter for the next lexer.
				if len(m[3]) != 0 {
					m[0] = bytes.TrimSuffix(m[0], m[3])
				}
				l := len(m[0])
				if len(m[2]) == 0 && l != len(m[1]) && !tagDelim(m[0][len(m[1])]) {
					// See plusEnd.
					l = len(m[1])
					m[0] = m[0][:l]
				}
				if len(m[2]) != 0 {
					// The terminator consumes a delimiter that \b does not see
					// as a boundary ('AAC2.0_'). Leave it for the delimiter
					// lexer, or the audio tag keeps the channels as well.
					if !bytes.HasSuffix(m[0], m[2]) {
						l--
						m[0] = m[0][:l]
					}
					m[0] = bytes.TrimSuffix(m[0], m[2])
				}
				start = append(start, newTag(src, buf, TagTypeAudio, audiof, m[0], m[1]))
				if len(m[2]) != 0 {
					start = append(start, newTag(src, buf, TagTypeChannels, channelsf, m[2], m[2]))
				}
				return start, end, i + l, n, true
			}
			return start, end, i, n, false
		},
	}, func() *lexGate { return leadGate(lead, gated) })
}

// NewGenreLexer creates a tag lexer for a genre.
func NewGenreLexer() Lexer {
	var genref *finder
	var re, lb, other *regexp.Regexp
	var pf, pfOther *prefilter
	return withGate(TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			genre := infos["genre"]
			// build regexp for (Genre)
			var v, tagv []string
			for _, info := range genre {
				v = append(v, info.RE())
				if other := info.Other(); other != "" {
					tagv = append(tagv, other)
				}
			}
			s := `\(?(` + strings.Join(v, `|`) + `)\s*\)`
			re, lb, other, genref = regexp.MustCompile(`(?i)^`+s), regexp.MustCompile(`(?i)\(\s*`+s+`$`), regexp.MustCompile(`(?i)^(`+strings.Join(tagv, `|`)+`)\b`), newFinder(TagTypeGenre, genre...)
			// re ends on a ')' and other on a word boundary, so as with the
			// regexp lexers neither can end part way through a run
			if pf, _ = newPrefilterRE(v, true); pf != nil {
				pf.fold = true
			}
			if pfOther, _ = newPrefilterRE(tagv, true); pfOther != nil {
				pfOther.fold = true
			}
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			var m [][]byte
			// compare against src, and match "lookbehind"
			if pf.maybe(src, i, n) {
				if m = re.FindSubmatch(src[i:n]); m != nil && lb.Match(src[:i+len(m[0])]) {
					return append(
						start,
						newTag(src, buf, TagTypeGenre, genref, m...),
					), end, i + len(m[0]), n, true
				}
			}
			if !pfOther.maybe(buf, i, n) {
				return start, end, i, n, false
			}
			if m = other.FindSubmatch(buf[i:n]); m != nil {
				return append(
					start,
					newTag(src, buf, TagTypeGenre, genref, m...),
				), end, i + len(m[0]), n, true
			}
			return start, end, i, n, false
		},
	}, func() *lexGate { return prefilterGate(pf, pfOther) })
}

// spacedGroup bounds a group name whose words are space- or underscore-separated.
var spacedGroup = regexp.MustCompile(`(?i)^[a-z_ ]{2,10}$`)

// dottedGroup bounds a group name of three or more single letters separated by
// periods (ie, 'E.N.D').
var dottedGroup = regexp.MustCompile(`(?i)^(?:[a-z]\.){2,}[a-z]$`)

// NewGroupLexer creates a tag lexer for a group.
func NewGroupLexer() Lexer {
	const delim, invalid = '-', ` _.()[]{}+`
	bracket := regexp.MustCompile(`^[\]\)\}]`)
	var groupf, otherf *finder
	var re, special, tags *regexp.Regexp
	var shortTags map[string]bool
	// lastRuns holds the final word-run of every spelling a known group row
	// can take. A known group is anchored to the end of the name, so the name
	// has to end in one of them for re to match; lastOK is false when a row
	// could not be expanded, in which case re always runs.
	lastRuns, lastMax, lastOK := make(map[string]struct{}), 0, true
	// inTag reports whether the hyphen at h joins the words around it into a
	// known tag (ie, 'WEB-DL' in 'WEB-DL HEVC'), so it does not start a group.
	//
	// spans holds every spelling those tags can take, in upper case, so the
	// word is looked up before tags runs: nearly every name ends in '-GROUP',
	// and the regexp on each of them costs a fifth of the whole parse. A row
	// that cannot be expanded goes in patterns with the bytes it can begin
	// with and a literal its matches must contain, and those few rows have
	// their own regexp in rest; spansOK is false when one has no such
	// literal, in which case tags always runs.
	spans, spansOK := make(map[string]struct{}), true
	var patterns []patterned
	var rest *regexp.Regexp
	inTag := func(src []byte, h int) bool {
		a, b := bytes.LastIndexAny(src[:h], invalid)+1, len(src)
		if k := bytes.IndexAny(src[h+1:], invalid); k != -1 {
			b = h + 1 + k
		}
		var key [64]byte
		if w := src[a:b]; spansOK && len(w) <= len(key) {
			for k, c := range w {
				// (?i) folds onto non-ASCII runes; leave those to the regexp
				if c >= 0x80 {
					return tags.Match(w)
				}
				if c >= 'a' && c <= 'z' {
					c -= 32
				}
				key[k] = c
			}
			if _, ok := spans[string(key[:len(w)])]; ok {
				return tags.Match(w)
			}
			return slices.ContainsFunc(patterns, func(row patterned) bool {
				return row.lead[w[0]] && hasLiteral(w, row.lit, row.fold)
			}) && rest.Match(w)
		}
		return tags.Match(src[a:b])
	}
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, short map[string]bool) {
			var v []string
			var known, unexpanded []*taginfo.Taginfo
			group, other := infos["group"], infos["other"]
			for _, info := range other {
				if s := info.Other(); s != "" {
					v = append(v, s)
				}
			}
			// only tags that can span a hyphen
			for typ, ti := range infos {
				for _, info := range ti {
					if typ != "group" && strings.Contains(info.RE(), "-") {
						known = append(known, info)
					}
				}
			}
			for _, info := range known {
				exp, err := expandRE(info.RE(), true)
				if err != nil {
					lit, fold := requiredLiteral(info.RE(), true)
					if len(lit) == 0 {
						spansOK = false
						break
					}
					row := patterned{lit: lit, fold: fold}
					if !firstBytes(info.RE(), &row.lead) {
						spansOK = false
						break
					}
					// the rows match case insensitively
					for c := 'a'; c <= 'z'; c++ {
						if row.lead[c] || row.lead[c-32] {
							row.lead[c], row.lead[c-32] = true, true
						}
					}
					patterns, unexpanded = append(patterns, row), append(unexpanded, info)
					continue
				}
				for _, s := range exp {
					spans[strings.ToUpper(s)] = struct{}{}
				}
			}
			groupf, otherf = newFinder(TagTypeGroup, group...), newFinder(TagTypeOther, other...)
			for _, info := range group {
				exp, err := expandRE(info.RE(), true)
				if err != nil {
					lastOK = false
					break
				}
				for _, s := range exp {
					j := len(s)
					for j > 0 && runByte(s[j-1]) {
						j--
					}
					if j == len(s) {
						lastOK = false
					}
					lastRuns[strings.ToUpper(s[j:])] = struct{}{}
					if len(s)-j > lastMax {
						lastMax = len(s) - j
					}
				}
			}
			re, special = regexp.MustCompile(`(?i)[\-\._ ]+`+reutil.Taginfo(`$`, group...)), regexp.MustCompile(`(?i)_(`+strings.Join(v, `|`)+`)$`)
			tags = regexp.MustCompile(reutil.Taginfo(`i^$`, known...))
			rest = regexp.MustCompile(reutil.Taginfo(`i^$`, unexpanded...))
			shortTags = short
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// usenet repost suffixes, which follow the group
			for {
				k, v := repostSuffix(src[i:n])
				if k == 0 {
					break
				}
				end, n = append(end, newTag(src, buf, TagTypeOther, nil, src[n-k:n], []byte(v))), n-k
			}
			// special end tags on groups. the pattern opens with a literal
			// underscore, so a name without one cannot match
			if bytes.IndexByte(src[i:n], '_') != -1 {
				if m := special.FindSubmatch(src[i:n]); m != nil {
					end, n = append(end, newTag(src, buf, TagTypeOther, otherf, m...)), n-len(m[0])
				}
			}
			// known groups
			if knownGroupTail(src[i:n], lastRuns, lastMax, lastOK) {
				if m := re.FindSubmatch(src[i:n]); m != nil {
					return start, append(end, newTag(src, buf, TagTypeGroup, groupf, m...)), i, n - len(m[0]), true
				}
			}
			// clamp to the first year
			l := i
			if e := firstYearEnd(buf[l:n]); e != -1 {
				l += e
			}
			// locate delimiter and check valid group
			if j := bytes.LastIndexByte(buf[l:n], delim); j != -1 {
				s := src[l+j+1 : n]
				if grp := bytes.Trim(s, " \t_"); len(grp) != 0 &&
					(!bytes.ContainsAny(s, invalid) || (len(s) <= 14 && (spacedGroup.Match(grp) || dottedGroup.Match(grp)))) &&
					!shortTags[string(grp)] && !(lowerCase(grp) && shortTags[strings.ToUpper(string(grp))]) &&
					(len(end) == 0 || !bracket.MatchString(end[len(end)-1].Text())) &&
					!inTag(src[:n], l+j) {
					return start, append(
						end,
						newTag(src, buf, TagTypeGroup, nil, src[l+j+1:n], grp),
						newTag(src, buf, TagTypeDelim, nil, src[l+j:l+j+1], []byte{delim}),
					), i, l + j, false
				}
			}
			return start, end, i, n, false
		},
		Once: true,
	}
}

// reposts are the suffixes usenet indexers and reposters append after the
// release group ('-GRP-Obfuscated'), keyed in lower case. The list is the
// unambiguous part of the one Sonarr strips before it reads a group.
var reposts = map[string]string{
	"4planet":                "4Planet",
	"altezachen":             "AlteZachen",
	"alternativetorequested": "AlternativeToRequested",
	"asrequested":            "AsRequested",
	"buymore":                "BUYMORE",
	"chamele0n":              "Chamele0n",
	"gerov":                  "GEROV",
	"nzbgeek":                "NZBGeek",
	"obfuscated":             "Obfuscated",
	"postbot":                "postbot",
	"repackpost":             "RePACKPOST",
	"scrambled":              "Scrambled",
	"whiterev":               "WhiteRev",
	"xpost":                  "xpost",
	"z0ids3n":                "Z0iDS3N",
}

// repostSuffix returns the length of a repost suffix on the end of s with its
// leading delimiter, and the suffix as it is usually written, or 0 when s
// does not end in one. A '.sample' counts as one.
func repostSuffix(s []byte) (int, string) {
	j := len(s)
	for j > 0 && runByte(s[j-1]) {
		j--
	}
	var key [24]byte
	// the suffix follows something, so there is a byte before the delimiter
	if j < 2 || j == len(s) || len(s)-j > len(key) {
		return 0, ""
	}
	w := key[:len(s)-j]
	for k, c := range s[j:] {
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		w[k] = c
	}
	switch s[j-1] {
	case '-':
		if v, ok := reposts[string(w)]; ok {
			return len(w) + 1, v
		}
		if bytes.HasPrefix(w, []byte("rakuv")) {
			return len(w) + 1, "Rakuv"
		}
	case '.':
		if string(w) == "sample" {
			return len(w) + 1, "Sample"
		}
	}
	return 0, ""
}

// lowerCase reports whether b has no upper case ASCII letter. The short tags
// are stored in upper case, and a group candidate in mixed case ('VFi') is a
// name, where one written all in lower case ('web-dl', 'dts-hd') is a tag
// from a lower-cased release.
func lowerCase(b []byte) bool {
	for _, c := range b {
		if c >= 'A' && c <= 'Z' {
			return false
		}
	}
	return true
}

// metaWindow bounds how far back from the end a meta suffix is searched for.
// The longest value any pattern accepts is 16+32 bytes plus brackets.
const metaWindow = 160

// firstYearEnd returns the end of the first match of \b(19|20)\d{2}\b in b,
// or -1.
func firstYearEnd(b []byte) int {
	word := func(c byte) bool {
		return c == '_' || runByte(c)
	}
	for p := 0; p+4 <= len(b); p++ {
		if ((b[p] == '1' && b[p+1] == '9') || (b[p] == '2' && b[p+1] == '0')) &&
			b[p+2] >= '0' && b[p+2] <= '9' && b[p+3] >= '0' && b[p+3] <= '9' &&
			(p == 0 || !word(b[p-1])) && (p+4 == len(b) || !word(b[p+4])) {
			return p + 4
		}
	}
	return -1
}

// behind reports whether b ends in open followed by min to max bytes of set,
// which is what the lookbehind regexps `open[set]{min,max}$` tested by
// scanning the whole of b.
func behind(b []byte, open byte, set string, min, max int) bool {
	for k := 0; k <= max; k++ {
		j := len(b) - 1 - k
		if j < 0 {
			return false
		}
		if k >= min && b[j] == open {
			return true
		}
		if strings.IndexByte(set, b[j]) == -1 {
			return false
		}
	}
	return false
}

// knownGroupTail reports whether s could end in a known group, by testing its
// final word-run against the final runs the group rows can produce.
func knownGroupTail(s []byte, runs map[string]struct{}, max int, ok bool) bool {
	if !ok {
		return true
	}
	j := len(s)
	for j > 0 && runByte(s[j-1]) {
		j--
	}
	// (?i) folds 'k' and 's' onto the non-ASCII runes U+212A and U+017F, which
	// are not run bytes; leave anything next to a non-ASCII byte to the regexp
	if j > 0 && s[j-1] >= 0x80 {
		return true
	}
	if j == len(s) || len(s)-j > max {
		return false
	}
	var key [64]byte
	if len(s)-j > len(key) {
		return true
	}
	for a, c := range s[j:] {
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		key[a] = c
	}
	_, found := runs[string(key[:len(s)-j])]
	return found
}

// NewMetaLexer creates a tag lexer for a file's meta data.
//
// The arguments come in fours: the key, the opening literal, the closing
// literal and a regexp for the value. The regexp has one capture group, or
// two, and then the first group is the key.
//
// A pattern is tried at the start of a name and at its end. At the end only
// the last 160 bytes are searched, so a pattern whose opening literal, value
// and closing literal together can be longer than that is not found there.
func NewMetaLexer(strs ...string) Lexer {
	if len(strs)%4 != 0 {
		panic("must be divisible by 4")
	}
	o := len(strs) / 4
	// build prefix, suffix
	prefix, suffix, hasTwo := make([]*regexp.Regexp, o), make([]*regexp.Regexp, o), make([]bool, o)
	for l := 0; l < o; l++ {
		s := `\s*\Q` + strs[l*4+1] + `\E\s*` + strs[l*4+3] + `\s*\Q` + strs[l*4+2] + `\E\s*`
		prefix[l], suffix[l] = regexp.MustCompile(`^`+s), regexp.MustCompile(s+`$`)
		if n := prefix[l].NumSubexp(); n != 1 && n != 2 {
			panic("must have exactly 1 or 2 capture groups")
		}
		hasTwo[l] = prefix[l].NumSubexp() == 2
	}
	// a prefix pattern can only match when the first non-space byte is the
	// first byte of its opener, and a suffix pattern only when the text before
	// the trailing spaces ends in its closer. Checking that first keeps all
	// eight regexps out of the common case of a name with no brackets.
	openFirst, closer, gate := make([]byte, o), make([][]byte, o), true
	for l := 0; l < o; l++ {
		op, cl := strs[l*4+1], strs[l*4+2]
		if op == "" || cl == "" || metaSpace(op[0]) || metaSpace(cl[len(cl)-1]) {
			gate = false
			continue
		}
		openFirst[l], closer[l] = op[0], []byte(cl)
	}
	var delim *regexp.Regexp
	var shortTags map[string]bool
	// lead matches a leading bracket whose contents contain a space or a
	// plus. fansub release groups regularly do ('[Anime Time]', '[Mad Le
	// Zisell]', '[SubsPlus+]'), but the general site pattern disallows both
	// because a bracket elsewhere in a name holds a block of tags ('[1080p
	// HEVC]', '[DVD+Web-Dl]'), not a group. restricting this to offset 0
	// keeps those apart.
	lead := regexp.MustCompile(`^\[([^\]\t\r\n\f]{1,40})\]`)
	// leadTags rejects a bracket that is really a block of tags. A word of it
	// may have an '@' after it, as a resolution has in front of a frame rate
	// ('[1080p@60fps]').
	leadTags := regexp.MustCompile(`(?i)(^|[\-\._ ])(\d{3,4}[ip]|\d{3,4}x\d{3,4}|[xh][\-\. ]?26[45]|hevc|avc|aac|flac|opus|blu[\-\. ]?ray|web[\-\. ]?dl|bd(?:rip)?|dvd|\d{1,2}[\-\. ]?bit|hi10p?|dual[\-\. ]?audio|multi[\-\. ]?subs?)($|[\-\._ @])`)
	// onlyResolution matches a bracket that is a resolution and nothing else.
	// The bracket that opens a name is its group even when a tag is part of
	// it ('[Anime-HEVC] Show'), but not when it is this and the title follows
	// ('[1080p] Show - 01')
	onlyResolution := regexp.MustCompile(`(?i)^(?:\d{3,4}[ip]|\d{3,4}x\d{3,4})$`)
	// words holds the words of the tag names, in upper case. A bracket further
	// into the name that holds one of them, a version or a list of two-letter
	// codes is not the site ('[REMASTERED]', '[v2]', '[MX-EN-PT]')
	words := make(map[string]struct{})
	notSite := func(v []byte) bool {
		if leadTags.Match(v) {
			return true
		}
		var key [32]byte
		if len(v) > len(key) {
			return false
		}
		version, codes := len(v) > 1 && (v[0] == 'v' || v[0] == 'V'), len(v) > 7 && len(v)%3 == 2
		for k, c := range v {
			version = version && (k == 0 || c >= '0' && c <= '9')
			codes = codes && (k%3 == 2 && c == '-' || k%3 != 2 && c >= 'A' && c <= 'Z')
			if c >= 'a' && c <= 'z' {
				c -= 32
			}
			key[k] = c
		}
		_, ok := words[string(key[:len(v)])]
		return ok || version || codes
	}
	// plus holds the spellings, in upper case, of the tags that end in '+'
	// ('HDR10+', 'DD+', 'ATV+'). A '+' is a delimiter, and one on the end of
	// the name is stripped below with the rest, unless it closes one of these.
	plus, plusMax := make(map[string]struct{}), 0
	plusTag := func(b []byte) bool {
		var key [24]byte
		key[len(key)-1] = '+'
		for k := len(b) - 2; k >= 0 && len(b)-k <= min(plusMax, len(key)); k-- {
			c := b[k]
			if c >= 'a' && c <= 'z' {
				c -= 32
			}
			key[len(key)-len(b)+k] = c
			// the tag has to begin the word it is in
			if k != 0 && runByte(b[k-1]) {
				continue
			}
			if _, ok := plus[string(key[len(key)-len(b)+k:])]; ok {
				return true
			}
		}
		return false
	}
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, re *regexp.Regexp, short map[string]bool) {
			delim, shortTags = re, short
			for typ, v := range infos {
				for _, info := range v {
					// a group's name is what the site may well be
					if typ != "group" {
						for field := range strings.FieldsFuncSeq(info.Tag(), isAnyDelim) {
							if !strings.Contains(field, "$") {
								words[strings.ToUpper(field)] = struct{}{}
							}
						}
					}
					if !strings.Contains(info.RE(), "+") {
						continue
					}
					exp, err := expandRE(info.RE(), true)
					if err != nil {
						continue
					}
					for _, s := range exp {
						if strings.HasSuffix(s, "+") {
							plus[strings.ToUpper(s)], plusMax = struct{}{}, max(plusMax, len(s))
						}
					}
				}
			}
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			var l int
			var d []byte
			var m [][]byte
			var k string
			var v []byte
			var matched, short bool
			var prev map[string]bool
			seen := func(k string) {
				if prev == nil {
					prev = make(map[string]bool, o)
				}
				prev[k] = true
			}
			// leading bracket release group containing a space
			if i == 0 {
				if g := lead.FindSubmatch(src[i:n]); g != nil {
					// the value is the trimmed contents; an internal space or
					// a plus is what distinguishes a group from a padded
					// single token ('[ www.Speed.cd ]'), which the site
					// pattern handles.
					// tags after a plus are a note on the release, and the
					// group is the name in front of it ('[Group + Multi
					// Subs]')
					t := bytes.TrimSpace(g[1])
					if name, _, ok := bytes.Cut(t, []byte(" + ")); ok && leadTags.Match(t) && !leadTags.Match(name) && !notSite(name) {
						start = append(start, newTag(src, buf, TagTypeMeta, nil, g[0], []byte(`site`), name))
						i += len(g[0])
						seen(`site`)
					} else if bytes.ContainsAny(t, " +") && !leadTags.Match(t) {
						start = append(start, newTag(src, buf, TagTypeMeta, nil, g[0], []byte(`site`), t))
						i += len(g[0])
						seen(`site`)
					}
				}
			}
			// prefixes
			// nb is the first non-space byte at or after i; it only has to be
			// found again once i has moved past it
			nb := i - 1
			for ; i < n; i++ {
				matched = false
				if nb < i {
					for nb = i; nb < n && metaSpace(src[nb]); nb++ {
					}
				}
				c, any := byte(0), nb < n
				if any {
					c = src[nb]
				}
				for l = 0; l < o && !matched; l++ {
					if gate && (!any || openFirst[l] != c) {
						continue
					}
					if m = prefix[l].FindSubmatch(src[i:n]); m != nil {
						if k, v = strs[l*4], m[1]; hasTwo[l] {
							k, v = string(m[1]), m[2]
						}
						// a bracket of tags is not the site ('[1080p]',
						// '[HEVC-10bit]'), and does not use up its slot. The
						// bracket that opens the name is the group even
						// then ('[Anime-HEVC] Show'), unless it is a
						// resolution alone. With a bracket after it the name
						// is a row of brackets, where skipping the first
						// would leave the group unread ('[1080p][Group]')
						if k == "site" && (len(start) != 0 && notSite(v) ||
							onlyResolution.Match(v) && !(i+len(m[0]) < n && src[i+len(m[0])] == '[')) {
							continue
						}
						short = len(strs[l*4+1]) == 1 && shortTags[strings.ToUpper(string(v))]
						// a second bracket with text after it names a second
						// group ('[A] [B] Show - 01'), where one with a
						// bracket after it is the title ('[A][Show][01]') and
						// one of digits a year or an episode ('[A] [2003]')
						second := k == "site" && prev[k] && i+len(m[0]) < n && src[i+len(m[0])] != '[' &&
							bytes.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' }) != -1
						matched = (!prev[k] || second) && !short && !bytes.ContainsAny(v, "\t\r\n\f +")
						seen(k)
					}
				}
				if matched {
					if len(d) != 0 {
						start, d = append(start, newTag(src, buf, TagTypeDelim, nil, d, d)), d[:0]
					}
					start = append(start, newTag(src, buf, TagTypeMeta, nil, m[0], []byte(k), v))
					i = i + len(m[0]) - 1
				} else if !delim.Match(src[i : i+1]) {
					break
				} else {
					d = append(d, src[i])
				}
			}
			// backtrack remaining
			if len(d) != 0 {
				i, d = i-len(d), d[:0]
			}
			// suffixes
			for ; i < n; n-- {
				matched = false
				// a suffix ends in its closing literal, optionally followed by
				// whitespace, and is short: bound the scan to the tail
				lo := max(i, n-metaWindow)
				tail := bytes.TrimRight(src[lo:n], "\t\n\f\r ")
				for l = 0; l < o && !matched; l++ {
					if gate && !bytes.HasSuffix(tail, closer[l]) {
						continue
					}
					if m = suffix[l].FindSubmatch(src[lo:n]); m != nil {
						if k, v = strs[l*4], m[1]; hasTwo[l] {
							k, v = string(m[1]), m[2]
						}
						// a bracket of tags is not the site ('[1080p]',
						// '[HEVC-10bit]'), and does not use up its slot
						if k == "site" && notSite(v) {
							continue
						}
						short = len(strs[l*4+1]) == 1 && shortTags[strings.ToUpper(string(v))]
						matched = !prev[k] && !short && !bytes.ContainsAny(v, "\t\r\n\f +")
						seen(k)
					}
				}
				if matched {
					if len(d) != 0 {
						end, d = append(end, newTag(src, buf, TagTypeDelim, nil, d, d)), d[:0]
					}
					end = append(end, newTag(src, buf, TagTypeMeta, nil, m[0], []byte(k), v))
					n = n - len(m[0]) + 1
				} else if !delim.Match(src[n-1:n]) || src[n-1] == '+' && plusTag(src[i:n]) {
					break
				} else {
					// the pending delimiters are the bytes right after n, so
					// extend the slice back by one instead of copying it
					d = src[n-1 : n+len(d)]
				}
			}
			// backtrack remaining
			if len(d) != 0 {
				end = append(end, newTag(src, buf, TagTypeDelim, nil, d, d))
			}
			return start, end, i, n, true
		},
		Once: true,
	}
}

// metaSpace reports whether c is matched by \s.
func metaSpace(c byte) bool {
	switch c {
	case '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

// NewExtLexer creates a tag lexer for a file's extension.
func NewExtLexer() Lexer {
	var extf *finder
	var re *regexp.Regexp
	// window bounds how far from the end a match can start: the period plus
	// the longest extension, with every character allowed its widest (?i)
	// fold (three bytes). It is 0 when a row could not be expanded.
	var window int
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			ext := infos["ext"]
			re, extf = regexp.MustCompile(`(?i:\.`+reutil.Taginfo("$", ext...)+`)`), newFinder(TagTypeExt, ext...)
			for _, info := range ext {
				exp, err := expandRE(info.RE(), true)
				if err != nil {
					window = 0
					break
				}
				for _, s := range exp {
					if w := 1 + 3*len(s); w > window {
						window = w
					}
				}
			}
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// every match ends at n, so only the tail can hold one, and it has
			// to contain the period the pattern opens with
			if window != 0 {
				i0 := i
				if n-i > window {
					i0 = n - window
				}
				if bytes.IndexByte(src[i0:n], '.') == -1 {
					return start, end, i, n, false
				}
				if m := re.FindSubmatch(src[i0:n]); m != nil {
					return start, append(end, newTag(src, buf, TagTypeExt, extf, m...)), i, n - len(m[0]), true
				}
				return start, end, i, n, false
			}
			if m := re.FindSubmatch(src[i:n]); m != nil {
				return start, append(end, newTag(src, buf, TagTypeExt, extf, m...)), i, n - len(m[0]), true
			}
			return start, end, i, n, false
		},
		Once: true,
	}
}

// NewRegexpLexer creates a tag lexer for a regexp.
func NewRegexpLexer(typ TagType, ignoreCase bool) TagLexer {
	var f, otherf *finder
	var re *regexp.Regexp
	var pf *prefilter
	var mt *matcher
	bdremux := []byte("bdremux")
	return withGate(TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			info := infos[strings.ToLower(typ.String())]
			s := `^ib`
			if !ignoreCase {
				s = `^b`
			}
			re, f = regexp.MustCompile(reutil.Taginfo(s, info...)), newFinder(typ, info...)
			pf, _ = newPrefilter(info, ignoreCase)
			mt = newMatcher(info, ignoreCase, false)
			// the split below needs the two rows it stands for
			if typ == TagTypeSource && ignoreCase && f.find("BD") != nil {
				if o := newFinder(TagTypeOther, infos["other"]...); o.find("Remux") != nil {
					otherf = o
				}
			}
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// 'BDRemux' is a source and what was done to it in one word,
			// which no row can give as two tags. As the first word of a name
			// it is left to the title
			if k := i + len(bdremux); otherf != nil && k <= n && buf[i]|0x20 == 'b' && bytes.EqualFold(buf[i:k], bdremux) && (k == n || !runByte(buf[k])) &&
				slices.ContainsFunc(start, func(tag Tag) bool { return !tag.Is(TagTypeDelim, TagTypeWhitespace, TagTypeMeta) }) {
				return append(
					start,
					newTag(src, buf, TagTypeSource, f, src[i:i+2], src[i:i+2]),
					newTag(src, buf, TagTypeOther, otherf, src[i+2:k], src[i+2:k]),
				), end, k, n, true
			}
			if !pf.maybe(buf, i, n) {
				return start, end, i, n, false
			}
			// the row by its spellings, and the alternation when the matcher
			// cannot tell
			if row, _, e, sub, ok := mt.match(buf, i, n); ok {
				if row == -1 {
					return start, end, i, n, false
				}
				var v [20][]byte
				return append(start, newTag(src, buf, typ, f, mt.values(v[:], src[i:e], buf[i:e], row, sub)...)), end, e, n, true
			}
			if m := re.FindSubmatch(buf[i:n]); m != nil {
				return append(start, newTag(src, buf, typ, f, append([][]byte{src[i : i+len(m[0])]}, m[1:]...)...)), end, i + len(m[0]), n, true
			}
			return start, end, i, n, false
		},
	}, func() *lexGate {
		g := prefilterGate(pf)
		if otherf != nil && !g.always {
			// the word the lexer splits before it asks its prefilter
			g.runs[strings.ToUpper(string(bdremux))] = struct{}{}
		}
		return g
	})
}

// plusEnd extends a tag terminator for the tags that end in '+' ('HDR10+',
// 'DD+', 'ATV+'). After a '+' there is a word boundary only when a word byte
// follows, so the tag also has to be allowed to end the lex window, which is
// where the group lexer leaves it in 'HDR10+-GRP', and to stand before any
// other non-word rune ('HDR10+]'). That rune is matched only to test it: the
// lexers hand it back, unlike a delimiter.
const plusEnd = `|$|\W`

// tagDelim reports whether c is a delimiter that a tag terminator consumes.
func tagDelim(c byte) bool {
	return c == '-' || c == '.' || c == '_' || c == ' '
}

// NewRegexpSourceLexer creates a tag lexer for a regexp.
func NewRegexpSourceLexer(typ TagType, ignoreCase bool) TagLexer {
	var f *finder
	var re *regexp.Regexp
	var pf *prefilter
	var mt *matcher
	return withGate(TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			info := infos[strings.ToLower(typ.String())]
			s := `^i`
			if !ignoreCase {
				s = `^`
			}
			re, f = regexp.MustCompile(reutil.Taginfo(s, info...)+`(?:\b|[\-\._ ]`+plusEnd+`)`), newFinder(typ, info...)
			pf, _ = newPrefilter(info, ignoreCase)
			mt = newMatcher(info, ignoreCase, true)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if !pf.maybe(src, i, n) {
				return start, end, i, n, false
			}
			// the row by its spellings, and the alternation when the matcher
			// cannot tell
			if row, value, e, sub, ok := mt.match(src, i, n); ok {
				switch {
				case row == -1:
					return start, end, i, n, false
				case e != value && !tagDelim(src[value]):
					// See plusEnd.
					e = value
				}
				if e != value {
					return append(
						start,
						newTag(src, buf, typ, f, src[i:value], src[i:value]),
						newTag(src, buf, TagTypeDelim, nil, src[value:e], src[value:e]),
					), end, e, n, true
				}
				var v [20][]byte
				return append(start, newTag(src, buf, typ, f, mt.values(v[:], src[i:e], src[i:e], row, sub)...)), end, e, n, true
			}
			if m := re.FindSubmatch(src[i:n]); m != nil {
				if len(m[0]) != len(m[1]) && !tagDelim(m[0][len(m[1])]) {
					// See plusEnd.
					m[0] = m[0][:len(m[1])]
				}
				if len(m[0]) != len(m[1]) {
					v, delim := src[i:i+len(m[1])], src[i+len(m[1]):i+len(m[0])]
					return append(
						start,
						newTag(src, buf, typ, f, [][]byte{v, v}...),
						newTag(src, buf, TagTypeDelim, nil, [][]byte{delim, delim}...),
					), end, i + len(m[0]), n, true
				}
				return append(start, newTag(src, buf, typ, f, append([][]byte{src[i : i+len(m[0])]}, m[1:]...)...)), end, i + len(m[0]), n, true
			}
			return start, end, i, n, false
		},
	}, func() *lexGate { return prefilterGate(pf) })
}

// digitBytes are the bytes a number begins with.
const digitBytes = "0123456789"

// NamedCaptureLexer returns a func that matches named capture groups,
// returning as name/value string pairs.
func NamedCaptureLexer(strs ...string) func([]byte, []byte, int, int) ([]byte, [][]byte, int, int, bool) {
	return newNamedCapture(strs...).lex
}

// namedCapture is what NamedCaptureLexer matches with.
type namedCapture struct {
	regexps []*regexp.Regexp
	indexes [][]int
	subexps [][]string
	// leads[l] holds the bytes regexps[l] can start with, so a position it
	// cannot match is skipped without entering the regexp engine. gated[l] is
	// false when that set could not be bounded, in which case it always runs.
	leads [][256]bool
	gated []bool
	// dfa gives the regexps that can match at a position, of which the others
	// are not run. It is nil when the regexps have no automaton.
	dfa *prefixDFA
	// pairs[l] narrows leads[l] to the first two bytes, for regexps without
	// an automaton: 'Se' can begin a month name where 'Sh' cannot, though
	// both pass on 'S'.
	pairs []*pairSet
}

// newNamedCapture builds the regexps of strs that have a named capture group.
func newNamedCapture(strs ...string) *namedCapture {
	nc := &namedCapture{}
	var kept []string
	for l := 0; l < len(strs); l++ {
		// build regexp and collect subexp indexes
		re := regexp.MustCompile(strs[l])
		var idx []int
		var names []string
		for j, name := range re.SubexpNames() {
			if name != "" {
				idx, names = append(idx, j), append(names, name)
			}
		}
		if len(idx) != 0 {
			lead, ok := leadSet(strs[l])
			nc.regexps, nc.indexes, nc.subexps = append(nc.regexps, re), append(nc.indexes, idx), append(nc.subexps, names)
			nc.leads, nc.gated = append(nc.leads, lead), append(nc.gated, ok)
			kept = append(kept, strs[l])
		}
	}
	if nc.dfa = newPrefixDFA(kept...); nc.dfa == nil {
		for _, s := range kept {
			nc.pairs = append(nc.pairs, newPairSet(s))
		}
	}
	return nc
}

// lex matches the regexps in turn at i in buf, and returns the text of the
// first match in src with the names and values of its groups.
func (nc *namedCapture) lex(src, buf []byte, i, n int) ([]byte, [][]byte, int, int, bool) {
	var can uint32
	if nc.dfa != nil {
		if can = nc.dfa.mask(buf, i, n); can == 0 {
			return nil, nil, i, n, false
		}
	}
	for l, re := range nc.regexps {
		switch {
		case nc.dfa != nil:
			if can&(1<<l) == 0 {
				continue
			}
		// every one of these regexps is ^-anchored, so a match must begin
		// with buf[i]
		case nc.gated[l] && i < n && !nc.leads[l][buf[i]]:
			continue
		default:
			if ps := nc.pairs[l]; ps != nil && i+1 < n && buf[i] < 0x80 && buf[i+1] < 0x80 && !ps.has(buf[i], buf[i+1]) {
				continue
			}
		}
		if m := re.FindSubmatch(buf[i:n]); m != nil {
			// build values
			var v [][]byte
			for j, k := range nc.indexes[l] {
				if len(m[k]) != 0 {
					v = append(v, []byte(nc.subexps[l][j]), m[k])
				}
			}
			if len(v) != 0 {
				return src[i : i+len(m[0])], v, i + len(m[0]), n, true
			}
		}
	}
	return nil, nil, i, n, false
}

// gate is the gate of a lexer that returns false when lex does: the bytes
// the regexps can begin with.
func (nc *namedCapture) gate() *lexGate {
	return namedCaptureGate(nc)
}

// namedCaptureGate is the gate of a lexer that returns false when none of
// ncs matches.
func namedCaptureGate(ncs ...*namedCapture) *lexGate {
	g := &lexGate{}
	for _, nc := range ncs {
		for l, lead := range nc.leads {
			if !nc.gated[l] {
				return &lexGate{always: true}
			}
			for c, ok := range lead {
				g.lead[c] = g.lead[c] || ok
			}
		}
	}
	return g
}
