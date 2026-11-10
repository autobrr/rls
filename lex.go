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
		NewRegexpLexer(TagTypeResolution, true),
		NewRegexpSourceLexer(TagTypeCollection, true),
		NewSeriesLexer(
			// s02, S01E01
			`(?i)^s(?P<s>[0-8]?\d)[\-\._ ]?(?:e(?P<e>\d{1,5}))?\b`,
			// S01E02E03, S01E02-E03, S01E03.E04.E05
			`(?i)^s(?P<s>[0-8]?\d)(?P<m>(?:[\-\._ ]?e\d{1,5}){1,5})\b`,
			// S01S02S03
			`(?i)^(?P<S>(?:s[0-8]?\d){2,4})\b`,
			// 2x1, 1x01
			`(?i)^(?P<s>[0-8]?\d)x(?P<e>\d{1,3})\b`,
			// S01 - 02v3, S07-06, s03-5v.9
			`(?i)^s(?P<s>[0-8]?\d)[\-\._ ]{1,3}(?P<e>\d{1,5})(?:[\-\._ ]{1,3}(?P<v>v\d+(?:\.\d+){0,2}))?\b`,
			// Season.01.Episode.02, Series.01.Ep.02, Series.01, Season.01
			`(?i)^(?:series|season|s)[\-\._ ]?(?P<s>[0-8]?\d)(?:[\-\._ ]?(?:episode|ep)(?P<e>\d{1,5}))?\b`,
			// Vol.1.No.2, vol1no2
			`(?i)^vol(?:ume)?[\-\._ ]?(?P<s>\d{1,3})(?:[\-\._ ]?(?:number|no)[\-\._ ]?(?P<e>\d{1,5}))\b`,
			// Episode 15, E009, Ep. 007, Ep.05-07
			`(?i)^e(?:p(?:isode)?[\-\._ ]{1,3})?(?P<e>\d{1,5})(?:[\-\._ ]{1,3}\d{1,3})?\b`,
			// 10v1.7, 13v2
			`(?i)^(?P<e>\d{1,5})(?P<v>v[\-\._ ]?\d+(?:\.\d){0,2})\b`,
			// S01.Disc02, s01D3, Series.01.Disc.02, S02DVD3
			`(?i)^(?:series|season|s)[\-\._ ]?(?P<s>[0-8]?\d)[\-\._ ]?(?P<d>(?:disc|disk|dvd|d)[\-\._ ]?(?:\d{1,3}))\b`,
			// s1957e01
			`(?i)^s(?P<s>(?:19|20)\d\d)e(?P<e>\d{2,4})\b`,
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
						start, i = append(start, NewTag(TagTypeWhitespace, nil, m...)), i+len(m[0])
					}
				}
			}
			if i < n {
				switch src[n-1] {
				case '\t', '\n', '\f', '\r', ' ', 0x90, 0x8f:
					if m := suffix.FindSubmatch(src[i:n]); m != nil {
						end, n = append(end, NewTag(TagTypeWhitespace, nil, m...)), n-len(m[0])
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
	lexers := make([]func([]byte, []byte, int, int) ([]byte, [][]byte, int, int, bool), len(strs))
	for l, s := range strs {
		lexers[l] = NamedCaptureLexer(s)
	}
	return TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			for _, lexer := range lexers {
				s, v, i, n, ok := lexer(src, buf, i, n)
				if !ok {
					continue
				}
				// collect year, month, day
				var year, month, day []byte
				var y, m, d int
				matched := true
				for l := 0; l < len(v); l += 2 {
					f, t := string(v[l]), string(v[l+1])
					if f == "YY" {
						f, t = "2006", "20"+t
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
				if matched {
					return append(
						start,
						NewTag(TagTypeDate, nil, s, year, month, day),
					), end, i, n, true
				}
			}
			return start, end, i, n, false
		},
	}
}

// NewSeriesLexer creates a tag lexer for a series.
func NewSeriesLexer(strs ...string) Lexer {
	var sourcef taginfo.FindFunc
	lexer := NamedCaptureLexer(strs...)
	mlt := regexp.MustCompile(`(?i)s(\d?\d)`)
	mny := regexp.MustCompile(`(?i)[\-\._ ]?e(\d{1,5})`)
	dsc := regexp.MustCompile(`(?i)^disc|disk|dvd|d`)
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			sourcef = findFunc(infos["source"]...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, i, n, ok := lexer(src, buf, i, n); ok {
				// collect series, episode, version
				var series, episode, version, disc, multi, many []byte
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
					default:
						panic(fmt.Errorf("unknown capture group %q", v[l]))
					}
				}
				var tags []Tag
				if len(series) != 0 || len(episode) != 0 {
					if len(version) != 0 {
						s = bytes.TrimSuffix(s, version)
					}
					if len(disc) != 0 {
						s = bytes.TrimSuffix(s, disc)
					}
					l := [][]byte{s, series}
					switch {
					case len(many) == 0:
						l = append(l, episode)
					default:
						for _, m := range mny.FindAllSubmatch(many, -1) {
							l = append(l, m[1])
						}
					}
					tags = append(tags, NewTag(TagTypeSeries, nil, l...))
				}
				if len(version) != 0 {
					tags = append(tags, NewTag(TagTypeVersion, nil, version, version))
				}
				if len(disc) != 0 {
					disctyp := bytes.ToUpper(disc[:len(dsc.FindSubmatch(disc)[0])])
					num := bytes.TrimSpace(disc[len(disctyp):])
					switch string(disctyp) {
					case "DVD":
						tags = append(
							tags,
							NewTag(TagTypeSource, sourcef, disctyp, disctyp),
							NewTag(TagTypeDisc, nil, disc[len(disctyp):], disctyp, num),
						)
					default:
						tags = append(tags, NewTag(TagTypeDisc, nil, disc, disctyp, num))
					}
				}
				// S01S02S03 ...
				if m := mlt.FindAllSubmatch(multi, -1); m != nil {
					for j := 0; j < len(m); j++ {
						tags = append(tags, NewTag(TagTypeSeries, nil, append(m[j], nil)...))
					}
				}
				if len(series) == 4 && (bytes.HasPrefix(series, []byte{'1', '9'}) || bytes.HasPrefix(series, []byte{'2', '0'})) {
					tags = append(tags, NewTag(TagTypeDate, nil, nil, series, nil, nil))
				}
				return append(
					start,
					tags...,
				), end, i, n, true
			}
			return start, end, i, n, false
		},
	}
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
	return TagLexer{
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
						return append(start, NewTag(TagTypeID, nil, src[i:i+len(m[0])], m[1])), end, i + len(m[0]), n, true
					}
				}
			}
			return start, end, i, n, false
		},
	}
}

// NewEpisodeLexer creates a tag lexer for a single episode (`- 2 -`, `- 867 (`, `- 100 [`).
func NewEpisodeLexer() Lexer {
	re := regexp.MustCompile(`^(\d{1,4})(\b|[\._ ]?[\-\[\]\(\)\{\}])`)
	return TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// the match has to start with a digit, and that is far cheaper to
			// rule out than the lookbehind below, which scans the whole prefix
			if i < n && (src[i] < '0' || src[i] > '9') {
				return start, end, i, n, false
			}
			// compare against src, and match "lookbehind": -[\-\._ ]{1,3}$
			if behind(src[:i], '-', "-._ ", 1, 3) {
				if m := re.FindSubmatch(src[i:n]); m != nil && !bytes.HasPrefix(src[i+len(m[1]):], []byte{','}) {
					tags := []Tag{NewTag(TagTypeSeries, nil, m[1], nil, m[1])}
					if len(m[2]) != 0 {
						tags = append(tags, NewTag(TagTypeDelim, nil, m[2], m[2]))
					}
					return append(start, tags...), end, i + len(m[0]), n, true
				}
			}
			return start, end, i, n, false
		},
	}
}

// NewVersionLexer creates a tag lexer for a version.
func NewVersionLexer(strs ...string) Lexer {
	lexer := NamedCaptureLexer(strs...)
	return TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, i, n, ok := lexer(src, buf, i, n); ok {
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
				return append(start, NewTag(TagTypeVersion, nil, s, version)), end, i, n, true
			}
			return start, end, i, n, false
		},
		NotFirst: true,
	}
}

// NewDiscSourceYearLexer creates a tag lexer for the combined disc, source,
// year style tag.
func NewDiscSourceYearLexer(strs ...string) Lexer {
	var sourcef taginfo.FindFunc
	lexer := NamedCaptureLexer(strs...)
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			sourcef = findFunc(infos["source"]...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if _, v, i, n, ok := lexer(src, buf, i, n); ok {
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
					tags = append(tags, NewTag(TagTypeDisc, nil, disc, []byte{'X'}, disc))
				}
				tags = append(
					tags,
					NewTag(TagTypeSource, sourcef, source, source),
					NewTag(TagTypeDate, nil, year, year, nil, nil),
				)
				return append(start, tags...), end, i, n, true
			}
			return start, end, i, n, false
		},
	}
}

// NewDiscLexer creates a tag lexer for a disc.
//
//	n - number
//	t - type
//	x - xXtype
func NewDiscLexer(strs ...string) Lexer {
	var sourcef, sizef taginfo.FindFunc
	lexer, re := NamedCaptureLexer(strs...), regexp.MustCompile(`(?i)^dvd|cd|d|s|x`)
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			sourcef, sizef = findFunc(infos["source"]...), findFunc(infos["size"]...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, j, k, ok := lexer(src, buf, i, n); ok {
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
				if len(typ) != 0 {
					typ = bytes.ToUpper(typ[:len(re.FindSubmatch(s)[0])])
				}
				switch string(typ) {
				case "D", "S":
					return append(start, NewTag(TagTypeDisc, nil, s, typ, c)), end, j, k, true
				case "DVDA", "DVD", "CD":
					return append(start,
						NewTag(TagTypeSource, sourcef, s[:len(typ)], typ, typ),
						NewTag(TagTypeDisc, nil, s[len(typ):], typ, c),
					), end, j, k, true
				case "X":
					if sz := strings.ToUpper(string(s[len(c)+1:])); sz == "DVD9" {
						return append(start,
							NewTag(TagTypeDisc, nil, s[:len(c)+1], typ, c),
							NewTag(TagTypeSize, sizef, s[len(c)+1:], s[len(c)+1:]),
						), end, j, k, true
					}
					return append(start,
						NewTag(TagTypeDisc, nil, s[:len(c)+1], typ, c),
						NewTag(TagTypeSource, sourcef, s[len(c)+1:], s[len(c)+1:]),
					), end, j, k, true
				case "Z":
					return append(start,
						NewTag(TagTypeDisc, nil, s[:len(c)+1], []byte{'X'}, c),
						NewTag(TagTypeSize, sizef, s[len(c)+1:], z),
					), end, j, k, true
				case "":
					return append(start,
						NewTag(TagTypeDisc, nil, s[:len(c)+1], []byte{'X'}, c),
						NewTag(TagTypeSource, sourcef, s[len(c)+1:], x),
					), end, j, k, true
				default:
					panic(fmt.Errorf("unknown type %q", typ))
				}
			}
			return start, end, i, n, false
		},
	}
}

// NewAudioLexer creates a tag lexer for audios.
func NewAudioLexer() Lexer {
	var re *regexp.Regexp
	var audiof, channelsf taginfo.FindFunc
	var lead [256]bool
	var gated bool
	var pf *prefilter
	// next holds the bytes that can follow the audio tag's first run inside
	// the same run: a channel digit or the start of an attached codec
	var next [256]bool
	return TagLexer{
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
			audiof, channelsf = findFunc(audio...), findFunc(channels...)
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
				start = append(start, NewTag(TagTypeAudio, audiof, m[0], m[1]))
				if len(m[2]) != 0 {
					start = append(start, NewTag(TagTypeChannels, channelsf, m[2], m[2]))
				}
				return start, end, i + l, n, true
			}
			return start, end, i, n, false
		},
	}
}

// NewGenreLexer creates a tag lexer for a genre.
func NewGenreLexer() Lexer {
	var genref taginfo.FindFunc
	var re, lb, other *regexp.Regexp
	var pf, pfOther *prefilter
	return TagLexer{
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
			re, lb, other, genref = regexp.MustCompile(`(?i)^`+s), regexp.MustCompile(`(?i)\(\s*`+s+`$`), regexp.MustCompile(`(?i)^(`+strings.Join(tagv, `|`)+`)\b`), findFunc(genre...)
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
						NewTag(TagTypeGenre, genref, m...),
					), end, i + len(m[0]), n, true
				}
			}
			if !pfOther.maybe(buf, i, n) {
				return start, end, i, n, false
			}
			if m = other.FindSubmatch(buf[i:n]); m != nil {
				return append(
					start,
					NewTag(TagTypeGenre, genref, m...),
				), end, i + len(m[0]), n, true
			}
			return start, end, i, n, false
		},
	}
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
	var groupf, otherf taginfo.FindFunc
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
			groupf, otherf = findFunc(group...), findFunc(other...)
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
				end, n = append(end, NewTag(TagTypeOther, nil, src[n-k:n], []byte(v))), n-k
			}
			// special end tags on groups. the pattern opens with a literal
			// underscore, so a name without one cannot match
			if bytes.IndexByte(src[i:n], '_') != -1 {
				if m := special.FindSubmatch(src[i:n]); m != nil {
					end, n = append(end, NewTag(TagTypeOther, otherf, m...)), n-len(m[0])
				}
			}
			// known groups
			if knownGroupTail(src[i:n], lastRuns, lastMax, lastOK) {
				if m := re.FindSubmatch(src[i:n]); m != nil {
					return start, append(end, NewTag(TagTypeGroup, groupf, m...)), i, n - len(m[0]), true
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
						NewTag(TagTypeGroup, nil, src[l+j+1:n], grp),
						NewTag(TagTypeDelim, nil, src[l+j:l+j+1], []byte{delim}),
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
	// lead matches a leading bracket whose contents contain a space. fansub
	// release groups regularly do ('[Anime Time]', '[Mad Le Zisell]'), but the
	// general site pattern disallows spaces because a bracket elsewhere in a
	// name holds a block of tags ('[1080p HEVC]'), not a group. restricting
	// this to offset 0 keeps those apart.
	lead := regexp.MustCompile(`^\[([^\]\t\r\n\f]{1,40})\]`)
	// leadTags rejects a leading bracket that is really a block of tags.
	leadTags := regexp.MustCompile(`(?i)(^|[\-\._ ])(\d{3,4}[ip]|\d{3,4}x\d{3,4}|[xh][\-\. ]?26[45]|hevc|avc|aac|flac|opus|blu[\-\. ]?ray|web[\-\. ]?dl|bd(?:rip)?|dvd|\d{1,2}[\-\. ]?bit|hi10p?|dual[\-\. ]?audio|multi[\-\. ]?subs?)($|[\-\._ ])`)
	return TagLexer{
		Init: func(_ map[string][]*taginfo.Taginfo, re *regexp.Regexp, short map[string]bool) {
			delim, shortTags = re, short
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
					// the value is the trimmed contents; an internal space is
					// what distinguishes a group from a padded single token
					// ('[ www.Speed.cd ]'), which the site pattern handles.
					if t := bytes.TrimSpace(g[1]); bytes.ContainsRune(t, ' ') && !leadTags.Match(t) {
						start = append(start, NewTag(TagTypeMeta, nil, g[0], []byte(`site`), t))
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
						short = len(strs[l*4+1]) == 1 && shortTags[strings.ToUpper(string(v))]
						matched = !prev[k] && !short && !bytes.ContainsAny(v, "\t\r\n\f +")
						seen(k)
					}
				}
				if matched {
					if len(d) != 0 {
						start, d = append(start, NewTag(TagTypeDelim, nil, d, d)), d[:0]
					}
					start = append(start, NewTag(TagTypeMeta, nil, m[0], []byte(k), v))
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
						short = len(strs[l*4+1]) == 1 && shortTags[strings.ToUpper(string(v))]
						matched = !prev[k] && !short && !bytes.ContainsAny(v, "\t\r\n\f +")
						seen(k)
					}
				}
				if matched {
					if len(d) != 0 {
						end, d = append(end, NewTag(TagTypeDelim, nil, d, d)), d[:0]
					}
					end = append(end, NewTag(TagTypeMeta, nil, m[0], []byte(k), v))
					n = n - len(m[0]) + 1
				} else if !delim.Match(src[n-1 : n]) {
					break
				} else {
					// the pending delimiters are the bytes right after n, so
					// extend the slice back by one instead of copying it
					d = src[n-1 : n+len(d)]
				}
			}
			// backtrack remaining
			if len(d) != 0 {
				end = append(end, NewTag(TagTypeDelim, nil, d, d))
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
	var extf taginfo.FindFunc
	var re *regexp.Regexp
	// window bounds how far from the end a match can start: the period plus
	// the longest extension, with every character allowed its widest (?i)
	// fold (three bytes). It is 0 when a row could not be expanded.
	var window int
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			ext := infos["ext"]
			re, extf = regexp.MustCompile(`(?i:\.`+reutil.Taginfo("$", ext...)+`)`), findFunc(ext...)
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
					return start, append(end, NewTag(TagTypeExt, extf, m...)), i, n - len(m[0]), true
				}
				return start, end, i, n, false
			}
			if m := re.FindSubmatch(src[i:n]); m != nil {
				return start, append(end, NewTag(TagTypeExt, extf, m...)), i, n - len(m[0]), true
			}
			return start, end, i, n, false
		},
		Once: true,
	}
}

// NewRegexpLexer creates a tag lexer for a regexp.
func NewRegexpLexer(typ TagType, ignoreCase bool) TagLexer {
	var f taginfo.FindFunc
	var re *regexp.Regexp
	var pf *prefilter
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			info := infos[strings.ToLower(typ.String())]
			s := `^ib`
			if !ignoreCase {
				s = `^b`
			}
			re, f = regexp.MustCompile(reutil.Taginfo(s, info...)), findFunc(info...)
			pf, _ = newPrefilter(info, ignoreCase)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if !pf.maybe(buf, i, n) {
				return start, end, i, n, false
			}
			if m := re.FindSubmatch(buf[i:n]); m != nil {
				return append(start, NewTag(typ, f, append([][]byte{src[i : i+len(m[0])]}, m[1:]...)...)), end, i + len(m[0]), n, true
			}
			return start, end, i, n, false
		},
	}
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
	var f taginfo.FindFunc
	var re *regexp.Regexp
	var pf *prefilter
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			info := infos[strings.ToLower(typ.String())]
			s := `^i`
			if !ignoreCase {
				s = `^`
			}
			re, f = regexp.MustCompile(reutil.Taginfo(s, info...)+`(?:\b|[\-\._ ]`+plusEnd+`)`), findFunc(info...)
			pf, _ = newPrefilter(info, ignoreCase)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if !pf.maybe(src, i, n) {
				return start, end, i, n, false
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
						NewTag(typ, f, [][]byte{v, v}...),
						NewTag(TagTypeDelim, nil, [][]byte{delim, delim}...),
					), end, i + len(m[0]), n, true
				}
				return append(start, NewTag(typ, f, append([][]byte{src[i : i+len(m[0])]}, m[1:]...)...)), end, i + len(m[0]), n, true
			}
			return start, end, i, n, false
		},
	}
}

// NamedCaptureLexer returns a func that matches named capture groups,
// returning as name/value string pairs.
func NamedCaptureLexer(strs ...string) func([]byte, []byte, int, int) ([]byte, [][]byte, int, int, bool) {
	var regexps []*regexp.Regexp
	var indexes [][]int
	var subexps [][]string
	// leads[l] holds the bytes regexps[l] can start with, so a position it
	// cannot match is skipped without entering the regexp engine. gated[l] is
	// false when that set could not be bounded, in which case it always runs.
	var leads [][256]bool
	var gated []bool
	// pairs[l] narrows that to the first two bytes: 'Se' can begin a month
	// name where 'Sh' cannot, though both pass on 'S'.
	var pairs []*pairSet
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
			regexps, indexes, subexps = append(regexps, re), append(indexes, idx), append(subexps, names)
			leads, gated = append(leads, lead), append(gated, ok)
			pairs = append(pairs, newPairSet(strs[l]))
		}
	}
	o := len(regexps)
	return func(src, buf []byte, i, n int) ([]byte, [][]byte, int, int, bool) {
		for l := 0; l < o; l++ {
			// every one of these regexps is ^-anchored, so a match must begin
			// with buf[i]
			if gated[l] && i < n && !leads[l][buf[i]] {
				continue
			}
			if ps := pairs[l]; ps != nil && i+1 < n && buf[i] < 0x80 && buf[i+1] < 0x80 && !ps.has(buf[i], buf[i+1]) {
				continue
			}
			if m := regexps[l].FindSubmatch(buf[i:n]); m != nil {
				// build values
				var v [][]byte
				for j, k := range indexes[l] {
					if len(m[k]) != 0 {
						v = append(v, []byte(subexps[l][j]), m[k])
					}
				}
				if len(v) != 0 {
					return src[i : i+len(m[0])], v, i + len(m[0]), n, true
				}
			}
		}
		return nil, nil, i, n, false
	}
}
