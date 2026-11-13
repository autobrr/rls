package rls

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// numbering is what HasSeason, HasEpisode, Seasons and EpisodeRange give for
// a release.
type numbering struct {
	season, episode bool
	seasons         []int
	first, last     int
	ok              bool
}

func numberingOf(r Release) numbering {
	first, last, ok := r.EpisodeRange()
	return numbering{
		season:  r.HasSeason(),
		episode: r.HasEpisode(),
		seasons: r.Seasons(),
		first:   first,
		last:    last,
		ok:      ok,
	}
}

func TestNumbering(t *testing.T) {
	tests := []struct {
		name string
		exp  numbering
	}{
		// one season, one episode
		{"Show.S01E02.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1}}},
		// a season 0 written
		{"Show.S00E05.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{0}}},
		// an episode 0 written
		{"Show.S01E00.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1}}},
		// episodes 0 and 1
		{"Show.S01E00E01.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1}, first: 0, last: 1, ok: true}},
		// a season pack
		{"Show.S01.1080p.WEB.h264-GRP", numbering{season: true, seasons: []int{1}}},
		// ranges
		{"Show.S01E05E06.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1}, first: 5, last: 6, ok: true}},
		{"Show.S01E02-04.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1}, first: 2, last: 4, ok: true}},
		{"Show.S01E02-E04.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1}, first: 2, last: 4, ok: true}},
		{"Show.S00E02-E05.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{0}, first: 2, last: 5, ok: true}},
		// the two ends of a range that is too wide for SeriesEpisodes to fill
		{"Show.S01E1-E99999.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1}, first: 1, last: 99999, ok: true}},
		// one episode under two numberings
		{"Show.S23E19.Episode.1174.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{23}}},
		// one episode in two seasons is not a range
		{"Show.S01E01.S02E01.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1, 2}}},
		// the episodes are those of the season written first, the one in Series
		{"Show.S02E01.S01E05-E06.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1, 2}}},
		{"Show.S02E01E02.S01E05.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1, 2}, first: 1, last: 2, ok: true}},
		// the same episode twice in a path
		{"Show.S11E11-GRP/Show.S11E11-GRP.mkv", numbering{season: true, episode: true, seasons: []int{11}}},
		// no season, one episode
		{"[Grp] Show - 05 [1080p].mkv", numbering{episode: true}},
		{"[Grp] Show - 00 [1080p].mkv", numbering{episode: true}},
		{"Show.E05.1080p.WEB.h264-GRP", numbering{episode: true}},
		{"Show.Ep.07.1080p.WEB.h264-GRP", numbering{episode: true}},
		{"Show.Episode.15.1080p.WEB.h264-GRP", numbering{episode: true}},
		// no season, a batch
		{"[Grp] Show - 01-12 [1080p]", numbering{episode: true, first: 1, last: 12, ok: true}},
		{"[Grp] Show (01-12) [1080p]", numbering{episode: true, first: 1, last: 12, ok: true}},
		// a season, then a batch in it
		{"[Grp] Show S2 - 01-12 [1080p]", numbering{season: true, episode: true, seasons: []int{2}, first: 1, last: 12, ok: true}},
		// a season, then one episode in it
		{"[Grp] Show 2nd Season - 04 [1080p].mkv", numbering{season: true, episode: true, seasons: []int{2}}},
		{"[Grp] Show Second Season - 04 [1080p].mkv", numbering{season: true, episode: true, seasons: []int{2}}},
		{"[Grp]_Show_Second_Season_03-06_(1280x720_Blu-ray_FLAC)", numbering{season: true, episode: true, seasons: []int{2}, first: 3, last: 6, ok: true}},
		{"[Grp] Show Third Season [BD 1080p]", numbering{season: true, seasons: []int{3}}},
		// the show's other name after its season and episode repeats the
		// season, and a name that opens with the words is a title
		{"[Grp] Show S01E28 (Show Second Season OVA) [1080p]", numbering{season: true, episode: true, seasons: []int{1}}},
		{"Second.Season.Blues.2005.DVDRip.x264-GRP", numbering{}},
		// several seasons, no episode
		{"Show.S01S02S03.1080p.BluRay.x264-GRP", numbering{season: true, seasons: []int{1, 2, 3}}},
		{"Show.S01-S03.1080p.BluRay.x264-GRP", numbering{season: true, seasons: []int{1, 2, 3}}},
		{"Show.S01-S10.1080p.BluRay.x264-GRP", numbering{season: true, seasons: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}},
		{"Show.S00-S02.1080p.BluRay.x264-GRP", numbering{season: true, seasons: []int{0, 1, 2}}},
		{"Show.Season.1-Season.3.1080p.WEB-DL.x264-GRP", numbering{season: true, seasons: []int{1, 2, 3}}},
		{"Show S01 - S03 1080p BluRay x264-GRP", numbering{season: true, seasons: []int{1, 2, 3}}},
		// two seasons that are not a range
		{"Show.S01S03.1080p.BluRay.x264-GRP", numbering{season: true, seasons: []int{1, 3}}},
		// a season and then an episode of a later one is not a range of seasons
		{"Show.S01-S03E05.1080p.BluRay.x264-GRP", numbering{season: true, episode: true, seasons: []int{1, 3}}},
		// season 1 episode 5
		{"Show.1x05.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1}}},
		// the parser reads the year as the season and the round as the episode
		{"Formula1.2024x05.Miami.Grand.Prix.Race.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{2024}}},
		// dated, and a movie
		{"Show.2024.10.13.1080p.WEB.h264-GRP", numbering{}},
		{"Movie.2020.1080p.BluRay.x264-GRP", numbering{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := ParseString(test.name)
			if got := numberingOf(r); !equalNumbering(got, test.exp) {
				t.Errorf("got %+v, expected %+v (%e)", got, test.exp, r)
			}
		})
	}
}

func TestNumberingByHand(t *testing.T) {
	// a caller clears the fields of a parsed release, and the tags still
	// give the season and the episode
	cleared := ParseString("Show.S01E02.1080p.WEB.h264-GRP")
	cleared.Series, cleared.Episode = 0, 0
	tests := []struct {
		name string
		r    Release
		exp  numbering
	}{
		{"zero", Release{}, numbering{}},
		{"series", Release{Series: 3}, numbering{season: true, seasons: []int{3}}},
		{"episode", Release{Episode: 4}, numbering{episode: true}},
		{"series and episode", Release{Series: 1, Episode: 2}, numbering{season: true, episode: true, seasons: []int{1}}},
		{"cleared", cleared, numbering{season: true, episode: true, seasons: []int{1}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := numberingOf(test.r); !equalNumbering(got, test.exp) {
				t.Errorf("got %+v, expected %+v", got, test.exp)
			}
		})
	}
}

func TestNumberingAllocs(t *testing.T) {
	for _, name := range []string{
		"Show.S01E02-E04.1080p.WEB.h264-GRP",
		"Show.S01-S03.1080p.BluRay.x264-GRP",
		"[Grp] Show - 01-12 [1080p]",
	} {
		r := ParseString(name)
		for _, test := range []struct {
			name string
			max  float64
			f    func()
		}{
			{"HasSeason", 0, func() { _ = r.HasSeason() }},
			{"HasEpisode", 0, func() { _ = r.HasEpisode() }},
			{"EpisodeRange", 0, func() { _, _, _ = r.EpisodeRange() }},
		} {
			if n := testing.AllocsPerRun(100, test.f); n > test.max {
				t.Errorf("%s: %s allocates %v times, expected at most %v", name, test.name, n, test.max)
			}
		}
	}
}

func equalNumbering(a, b numbering) bool {
	return a.season == b.season && a.episode == b.episode && slices.Equal(a.seasons, b.seasons) &&
		a.first == b.first && a.last == b.last && a.ok == b.ok
}

// TestEpisodeZero checks that a written episode 0 is an episode: SeriesEpisodes
// lists it, a range starts at it, and with no season it is absolute.
func TestEpisodeZero(t *testing.T) {
	tests := []struct {
		name     string
		exp      [][]int
		absolute bool
	}{
		{"[Group] Show - 00 [720p]", [][]int{{0, 0}}, true},
		{"[Group] Show - 00v2 [720p]", [][]int{{0, 0}}, true},
		{"[Group] Show - Episode 0 [720p]", [][]int{{0, 0}}, true},
		{"Show.E00.1080p.WEB.h264-GRP", [][]int{{0, 0}}, true},
		// a season qualifies it
		{"Show.S01E00.1080p.WEB.h264-GRP", [][]int{{1, 0}}, false},
		{"Show.1x00.HDTV.x264-GRP", [][]int{{1, 0}}, false},
		{"[Group] Show S2 - 00 [1080p]", [][]int{{2, 0}}, false},
		{"Show.S00E00.1080p.WEB.h264-GRP", [][]int{{0, 0}}, false},
		// ranges that start at 0
		{"Show.S01E00E01.1080p.WEB.h264-GRP", [][]int{{1, 0}, {1, 1}}, false},
		{"Show.S01E00-E02.1080p.WEB.h264-GRP", [][]int{{1, 0}, {1, 1}, {1, 2}}, false},
		{"[Group] Show - 00-03 [720p]", [][]int{{0, 0}, {0, 1}, {0, 2}, {0, 3}}, false},
		{"[Group] Show - 00 ~ 02 [720p]", [][]int{{0, 0}, {0, 1}, {0, 2}}, false},
		// a range that ends at 0 is its two ends
		{"Show.S01E03-E00.1080p.WEB.h264-GRP", [][]int{{1, 0}, {1, 3}}, false},
		// half an episode and a range after the 0 are not its number
		{"[Group] Show - 00 (Ep 5.5) [720p]", [][]int{{0, 0}, {0, 5}}, true},
		{"[Group] Show - 00 - E05-07 [720p]", [][]int{{0, 0}, {0, 5}, {0, 6}, {0, 7}}, true},
		// half an episode has no absolute number
		{"[Group] Show - 00.5 [720p]", [][]int{{0, 0}}, false},
		// no episode written
		{"Show.S01.1080p.WEB.h264-GRP", nil, false},
		{"Show.2024.1080p.WEB.h264-GRP", nil, false},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if v := r.SeriesEpisodes(); !slices.EqualFunc(v, test.exp, slices.Equal[[]int]) {
			t.Errorf("%q: expected series episodes %v, got: %v", test.name, test.exp, v)
		}
		if episode, ok := r.Absolute(); episode != 0 || ok != test.absolute {
			t.Errorf("%q: expected absolute 0 %t, got: %d %t", test.name, test.absolute, episode, ok)
		}
		if ok := r.HasEpisode(); ok != (test.exp != nil) {
			t.Errorf("%q: expected has episode %t, got: %t", test.name, test.exp != nil, ok)
		}
	}
	for _, tag := range ParseString("Show.S01E00.1080p.WEB.h264-GRP").Tags() {
		if v := tag.Episodes(); tag.Is(TagTypeSeries) && !slices.Equal(v, []int{0}) {
			t.Errorf("expected episodes [0], got: %v", v)
		}
	}
	// the first episode that is not 0 is the absolute one, as it is Episode
	if episode, ok := ParseString("[Group] Show - 00 - 05 [720p]").Absolute(); episode != 5 || !ok {
		t.Errorf("expected absolute 5 true, got: %d %t", episode, ok)
	}
}

// TestEpisodeRangeTilde checks that a tilde between two episode tags of one
// season makes a range, as a hyphen does.
func TestEpisodeRangeTilde(t *testing.T) {
	tests := []struct {
		name string
		exp  [][]int
	}{
		{"[Grp] Show S04E29~E31 1080p WEB H.264", [][]int{{4, 29}, {4, 30}, {4, 31}}},
		{"Show.S01E02~S01E04.1080p.WEB.h264-GRP", [][]int{{1, 2}, {1, 3}, {1, 4}}},
		{"Show.S01E01~03.1080p.WEB.x264-GRP", [][]int{{1, 1}, {1, 2}, {1, 3}}},
		{"[Grp] Show E05~E07 [1080p]", [][]int{{0, 5}, {0, 6}, {0, 7}}},
		// two seasons are not a range
		{"Show.S01E02~S02E04.1080p.WEB.h264-GRP", [][]int{{1, 2}, {2, 4}}},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if v := r.SeriesEpisodes(); !slices.EqualFunc(v, test.exp, slices.Equal[[]int]) {
			t.Errorf("%q: expected series episodes %v, got: %v", test.name, test.exp, v)
		}
		first, last, ok := r.EpisodeRange()
		if exp := test.exp[0][0] == test.exp[len(test.exp)-1][0]; ok != exp || ok && (first != test.exp[0][1] || last != test.exp[len(test.exp)-1][1]) {
			t.Errorf("%q: expected the range of %v, got: %d to %d, %t", test.name, test.exp, first, last, ok)
		}
	}
}

// TestSeriesEpisodesSeasonZero checks that an episode written with season 0
// stays in it when another season was written before it, and that an episode
// with no season after it is in season 0 too.
func TestSeriesEpisodesSeasonZero(t *testing.T) {
	tests := []struct {
		name string
		exp  [][]int
	}{
		{"[Group] Show - S04E29 (WEB 1080p) | Other Name - E88 - S00E36", [][]int{{0, 36}, {4, 29}, {4, 88}}},
		{"Show.S04E29.S00E36.E37.1080p.WEB.h264-GRP", [][]int{{0, 36}, {0, 37}, {4, 29}}},
		{"Show.S04E29.S00E36-E38.1080p.WEB.h264-GRP", [][]int{{0, 36}, {0, 37}, {0, 38}, {4, 29}}},
		{"Show.S04E01-E03.1080p.WEB.h264-GRP", [][]int{{4, 1}, {4, 2}, {4, 3}}},
		{"Show.S00E02-E04.1080p.WEB.h264-GRP", [][]int{{0, 2}, {0, 3}, {0, 4}}},
		{"Show.S00E02-S00E04.1080p.WEB.h264-GRP", [][]int{{0, 2}, {0, 3}, {0, 4}}},
		{"Show.S01E02-S01E04.1080p.WEB.h264-GRP", [][]int{{1, 2}, {1, 3}, {1, 4}}},
		{"Show.S01E02-S02E04.1080p.WEB.h264-GRP", [][]int{{1, 2}, {2, 4}}},
	}
	for _, test := range tests {
		if v := ParseString(test.name).SeriesEpisodes(); !slices.EqualFunc(v, test.exp, slices.Equal[[]int]) {
			t.Errorf("%q: expected series episodes %v, got: %v", test.name, test.exp, v)
		}
	}
}

// TestSeasonRange checks a range of seasons written with one season word:
// Series is the first season, Seasons gives every season of the range, the
// release is a pack, and the last season is neither the subtitle nor a
// leftover word. An episode written beside the range is in no season.
func TestSeasonRange(t *testing.T) {
	tests := []struct {
		name    string
		seasons []int
		spans   []EpisodeSpan
	}{
		// a bare hyphen or a tilde
		{"Show.S01-03.1080p.BluRay.x264-GRP", []int{1, 2, 3}, nil},
		{"Show.S1-3.1080p.BluRay.x264-GRP", []int{1, 2, 3}, nil},
		{"Show.S01~03.1080p.BluRay.x264-GRP", []int{1, 2, 3}, nil},
		{"Show.Season.1-3.1080p.BluRay.x264-GRP", []int{1, 2, 3}, nil},
		{"Show.Season.01-03.1080p.BluRay.x264-GRP", []int{1, 2, 3}, nil},
		{"Show Series 1-6 1080p BluRay x264-GRP", []int{1, 2, 3, 4, 5, 6}, nil},
		{"[Grp] Show (Seasons 01-02) [BD 1080p] (Batch)", []int{1, 2}, nil},
		{"[Grp] Show (Seasons 1-10) [BD 1080p]", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, nil},
		{"[Grp] Show Seasons 1~7 R1 DVD Remux 480p", []int{1, 2, 3, 4, 5, 6, 7}, nil},
		{"[Grp]_Show_(Season_1-3)_[BD_1080p]", []int{1, 2, 3}, nil},
		// a tilde between two season tags, bare or spaced
		{"Show.S01~S03.1080p.BluRay.x264-GRP", []int{1, 2, 3}, nil},
		{"[Grp] Show S01 ~ S03 [1080p]", []int{1, 2, 3}, nil},
		{"[Grp] Show (Season 01-25+Movies+Specials) [1080p]", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25}, nil},
		// spaces around the hyphen: a season word in a bracket, closed by ')'
		// or '+'
		{"[Grp] Show (Season 1 - 3) [BD 1080p]", []int{1, 2, 3}, nil},
		{"[Grp] Show (Season 01 - 04 + OVAs + Specials) [BD] [1080p]", []int{1, 2, 3, 4}, nil},
		{"[Grp] Show (Season 01 - 04+OVAs) [BD] [1080p]", []int{1, 2, 3, 4}, nil},
		// episodes beside the range are in no season, whichever way the range
		// is written
		{"[Grp] Show Season 1-4 (1-76) [1080p]", []int{1, 2, 3, 4}, []EpisodeSpan{{"", "1", "76"}}},
		{"[Grp] Show 001-367 (Seasons 1-10) [BD 1080p]", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, []EpisodeSpan{{"", "001", "367"}}},
		{"[Grp] Show S01-04 (1-76) [1080p]", []int{1, 2, 3, 4}, []EpisodeSpan{{"", "1", "76"}}},
		{"[Grp] Show S01-S04 (1-76) [1080p]", []int{1, 2, 3, 4}, []EpisodeSpan{{"", "1", "76"}}},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if v := r.Seasons(); !slices.Equal(v, test.seasons) {
			t.Errorf("%q: expected seasons %v, got: %v", test.name, test.seasons, v)
		}
		if r.Type != Series {
			t.Errorf("%q: expected type series, got: %s", test.name, r.Type)
		}
		if r.Series != test.seasons[0] {
			t.Errorf("%q: expected series %d, got: %d", test.name, test.seasons[0], r.Series)
		}
		if v := r.EpisodeSpans(); !slices.Equal(v, test.spans) {
			t.Errorf("%q: expected spans %q, got: %q", test.name, test.spans, v)
		}
		if exp := len(test.spans) != 0; r.HasEpisode() != exp {
			t.Errorf("%q: expected HasEpisode %t", test.name, exp)
		}
		for _, pair := range r.SeriesEpisodes() {
			if pair[0] != 0 {
				t.Errorf("%q: expected the episodes in no season, got: %v", test.name, pair)
				break
			}
		}
		if r.Subtitle != "" {
			t.Errorf("%q: expected no subtitle, got: %q", test.name, r.Subtitle)
		}
		for _, tag := range r.Unused() {
			if strings.Trim(tag.Text(), "0123456789") == "" {
				t.Errorf("%q: expected no number left over, got: %q", test.name, tag.Text())
			}
		}
		if s := fmt.Sprintf("%o", r); s != test.name {
			t.Errorf("%q: expected the name back, got: %q", test.name, s)
		}
	}
	// what looks like a range and is not one. The first season is all that
	// Seasons gives, and where the second number is an episode it is read
	notRanges := []struct {
		name   string
		season int
		spans  []EpisodeSpan
	}{
		// a season and its episode
		{"[Grp] Show Season 3 - 03 [1080p]", 3, []EpisodeSpan{{"3", "03", ""}}},
		{"[Grp] Show Season 1 - 12 [1080p]", 1, []EpisodeSpan{{"1", "12", ""}}},
		{"[Grp] Show Season 2 - 8 [1080p]", 2, []EpisodeSpan{{"2", "8", ""}}},
		{"[Grp] Show S2 - 04 [1080p]", 2, []EpisodeSpan{{"2", "04", ""}}},
		{"[Grp] Show S01 - 04 [1080p]", 1, []EpisodeSpan{{"01", "04", ""}}},
		{"[Grp] Show S01 - 04 (1080p)", 1, []EpisodeSpan{{"01", "04", ""}}},
		{"[Grp] Show S01 - 12 + OVA [1080p]", 1, []EpisodeSpan{{"01", "12", ""}}},
		{"[Grp] Show Season 1 - 12 + OVA [1080p]", 1, []EpisodeSpan{{"1", "12", ""}}},
		{"[Grp] Show (S01 - 12) [1080p]", 1, []EpisodeSpan{{"01", "12", ""}}},
	}
	for _, test := range notRanges {
		r := ParseString(test.name)
		if v := r.Seasons(); !slices.Equal(v, []int{test.season}) {
			t.Errorf("%q: expected season %d alone, got: %v", test.name, test.season, v)
		}
		if v := r.EpisodeSpans(); !slices.Equal(v, test.spans) {
			t.Errorf("%q: expected spans %q, got: %q", test.name, test.spans, v)
		}
	}
	// the second number is not a season: it is not the higher one, it is
	// padded where the season is not, it is 90 or more, or it begins another
	// tag. And a number word in front makes the season word part of a title
	for _, test := range []struct {
		name    string
		seasons []int
	}{
		{"Show.S07-06.720p.HDTV.x264-GRP", []int{7}},
		{"Show.S03-03.720p.HDTV.x264-GRP", []int{3}},
		{"Show.S2-04.720p.WEB.x264-GRP", []int{2}},
		{"Show.S01-99.720p.WEB.x264-GRP", []int{1}},
		{"Show.S01-10bit.720p.WEB.x264-GRP", []int{1}},
		{"Show.S01-720p.WEB.x264-GRP", []int{1}},
		{"The.Four.Seasons.1-2.1080p.BluRay.x264-GRP", nil},
	} {
		if v := ParseString(test.name).Seasons(); !slices.Equal(v, test.seasons) {
			t.Errorf("%q: expected seasons %v, got: %v", test.name, test.seasons, v)
		}
	}
}

// TestSeasonList checks that the seasons of a list are all read when only the
// first is written with a season word or an 'S', and that what looks like a
// list and is not one keeps to the first season.
func TestSeasonList(t *testing.T) {
	tests := []struct {
		name    string
		seasons []int
	}{
		// a plus, with and without spaces
		{"[Grp] Show (Season 1 + 2) [BDRip] [1080p]", []int{1, 2}},
		{"[Grp] Show (Season 1+2+3) (Uncensored) [1080p]", []int{1, 2, 3}},
		{"[Grp] Show (Season 01+02+03+04+05+Movies) [BD] [1080p]", []int{1, 2, 3, 4, 5}},
		{"[Grp] Show [Season 1 + 2 + OVA] [BD 1080p]", []int{1, 2}},
		{"[Grp] Show (S01+02) [BD][1080p]", []int{1, 2}},
		{"[Grp] Show S01+02 (COMPLETE) (BD 1080p)", []int{1, 2}},
		{"[Grp] Show (S01+02+OVAs+Movie) (BD 1080p)", []int{1, 2}},
		// an ampersand or a comma
		{"[Grp] Show (Season 1 & 2) [BD] [1080p]", []int{1, 2}},
		{"[Grp] Show (Season 01 & 02) [BD] [1080p]", []int{1, 2}},
		{"[Grp] Show (Season 1 & 2 + Movie + NCOP) [BD]", []int{1, 2}},
		{"[Grp] Show Season 1,2,3,4,5 - 1080p BluRay", []int{1, 2, 3, 4, 5}},
		{"Show S01 & 02 1080p BluRay x264-GRP", []int{1, 2}},
		// the plural, which is read in front of a list as in front of a range
		{"[Grp] Show - Seasons 1 & 2 V2 - 1080p.AV1", []int{1, 2}},
		{"[Grp] Show (Seasons 1+2) [BD 1080p]", []int{1, 2}},
		{"Show.Seasons.1+2.1080p.BluRay.x264-GRP", []int{1, 2}},
		// a space and a hyphen, a bracket or a resolution after the number
		{"[Grp] Show Season 1 & 2 - Batch [1080p]", []int{1, 2}},
		{"[Grp] Show Season 1 & 2 (Batch) [1080p]", []int{1, 2}},
		{"[Grp] Show Season 1 & 2 1080p BluRay", []int{1, 2}},
		// a word after the number: the number is tight to the list or
		// padded, or the seasons are in the plural
		{"[Grp] Show Season 1+2 Batch [1080p]", []int{1, 2}},
		{"[Grp] Show Season 1 , 2 , 3 [1080p]", []int{1, 2, 3}},
		{"[Grp] [Show Season 1 & 2 ] [1080p]", []int{1, 2}},
		{"[Grp] Show Season 1,2,3 BluRay 1080p", []int{1, 2, 3}},
		{"[Grp] Show S01 + 02 Batch [1080p]", []int{1, 2}},
		{"[Grp] Show Seasons 1 & 2 Batch [1080p]", []int{1, 2}},
		{"Show.S01+02.WEB.1080p.x264-GRP", []int{1, 2}},
		// the end of the name
		{"[Grp] Show Season 1 + 2", []int{1, 2}},
		{"[Grp] Show S01+02.mkv", []int{1, 2}},
		// a title that ends in a number
		{"[Grp] Show 86 Season 1+2 [1080p]", []int{1, 2}},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if v := r.Seasons(); !slices.Equal(v, test.seasons) {
			t.Errorf("%q: expected seasons %v, got: %v", test.name, test.seasons, v)
		}
		if r.Type != Series || r.Series != test.seasons[0] || !strings.HasPrefix(r.Title, "Show") {
			t.Errorf("%q: expected series %d of Show, got: %s %d of %q", test.name, test.seasons[0], r.Type, r.Series, r.Title)
		}
		if r.HasEpisode() {
			t.Errorf("%q: expected no episode, got: %q", test.name, r.EpisodeSpans())
		}
		for _, tag := range r.Unused() {
			if s := tag.Text(); s == "&" || strings.Trim(s, "0123456789") == "" {
				t.Errorf("%q: expected no number left over, got: %q", test.name, s)
			}
		}
		if strings.ContainsAny(r.Subtitle, "0123456789&") {
			t.Errorf("%q: expected no number in the subtitle, got: %q", test.name, r.Subtitle)
		}
		if s := fmt.Sprintf("%o", r); s != test.name {
			t.Errorf("%q: expected the name back, got: %q", test.name, s)
		}
	}
	// the number after the season is not a season: it may count the word
	// after it, it is measured in a unit, it is not the higher one, it is
	// padded where the one before it is not, it is 90 or more, or it goes on
	// as another number or tag. And a number or a number word in front makes
	// the season word part of a title
	for _, test := range []struct {
		name    string
		seasons []int
	}{
		{"[Grp] Show Season 1 + 2 OVAs [1080p]", []int{1}},
		{"[Grp] Show Season 1 & 2 Movies [1080p]", []int{1}},
		{"[Grp] Show Season 1 & 2 BluRay 1080p", []int{1}},
		{"[Grp] Show S01, 10 Bit HEVC [1080p]", []int{1}},
		{"[Grp] Show S01, 12 GB [1080p]", []int{1}},
		{"[Grp] Show S1,12 GB [1080p]", []int{1}},
		{"[Grp] Show S01, 2,5 GB [1080p]", []int{1}},
		{"[Grp] Show S01+24.MB [1080p]", []int{1}},
		{"[Grp] Show S01 + 25 fps [1080p]", []int{1}},
		{"[Grp] Show S01+2 TiB [1080p]", []int{1}},
		{"[Grp] Show S01 & 02 KiB [1080p]", []int{1}},
		{"[Grp] Show S01+2 Episodes [1080p]", []int{1}},
		{"[Grp] Show Season 1+2 OVAs [1080p]", []int{1}},
		{"[Grp] Show S01,2 Specials [1080p]", []int{1}},
		{"[Grp] Show S01 + 02 Discs [1080p]", []int{1}},
		{"[Grp] Show Season 01&45 OVA [1080p]", []int{1}},
		{"[Grp] Show S08,25 Part 2 [1080p]", []int{8}},
		{"Show.S01+02-GRP", []int{1}},
		{"[Grp] Show S1,45 Hours [1080p]", []int{1}},
		{"[Grp] Show S01, 12.2020 [1080p]", []int{1}},
		{"[Grp] Show S01 & 2.5000 [1080p]", []int{1}},
		{"[Grp] Show S01, 24 12 2020 [1080p]", []int{1}},
		{"[Grp] Show S1+2-12", []int{1}},
		{"[Grp] Show Season 1+2-Batch", []int{1}},
		{"[Grp] Show S01+2+03 [1080p]", []int{1, 2}},
		{"[Grp] Show Season 2 + 1 [1080p]", []int{2}},
		{"[Grp] Show S2+04 [1080p]", []int{2}},
		{"[Grp] Show S0+01 [1080p]", []int{0}},
		{"[Grp] Show S01+99 [1080p]", []int{1}},
		{"[Grp] Show S01 + 5.1 [1080p]", []int{1}},
		{"[Grp] Show S1, 10bit [1080p]", []int{1}},
		{"[Grp] Show S01+02-12 [1080p]", []int{1}},
		{"[Grp] Show S01 + 13.95GB [1080p]", []int{1}},
		{"[Grp] Show S01 + 12,5GB [1080p]", []int{1}},
		{"[Grp] Show S01, 25.976fps [1080p]", []int{1}},
		{"[Grp] Show S01 + 2.35:1 [1080p]", []int{1}},
		{"[Grp] Show S01 & 2/3 [1080p]", []int{1}},
		{"Show.S01.1080p.BluRay.x264-GRP", []int{1}},
		{"[Grp] Show Second Season 1+2 [1080p]", []int{2}},
		{"The Four Seasons 1 & 2 1080p BluRay x264-GRP", nil},
		{"The Four-Seasons 1 & 2 1080p BluRay x264-GRP", nil},
		{"The 4 Seasons 1 & 2 1080p BluRay x264-GRP", nil},
		{"The (Four) Seasons 1 & 2 1080p BluRay x264-GRP", nil},
		{"Show.Seasons.1080p.BluRay.x264-GRP", nil},
		{"Artist-Seasons_1_&_2-WEB-FLAC-2020-GRP", nil},
	} {
		r := ParseString(test.name)
		if v := r.Seasons(); !slices.Equal(v, test.seasons) {
			t.Errorf("%q: expected seasons %v, got: %v", test.name, test.seasons, v)
		}
		if s := fmt.Sprintf("%o", r); s != test.name {
			t.Errorf("%q: expected the name back, got: %q", test.name, s)
		}
	}
	// a date after the season keeps its day and month
	for _, name := range []string{
		"Show S01, 24.12.2020 1080p WEB",
		"Show.S01,24.12.2020.1080p.WEB.H264-GRP",
		"Show S01 & 24.12.2020",
	} {
		if r := ParseString(name); r.Year != 2020 || r.Month != 12 || r.Day != 24 || !slices.Equal(r.Seasons(), []int{1}) {
			t.Errorf("%q: expected season 1 and 2020-12-24, got: %v and %d-%d-%d", name, r.Seasons(), r.Year, r.Month, r.Day)
		}
	}
}

// TestSeasonListEpisodes checks that episodes written beside a list of
// seasons with no season of their own are in none of them, as beside a range.
func TestSeasonListEpisodes(t *testing.T) {
	for _, test := range []struct {
		name    string
		seasons []int
		spans   []EpisodeSpan
	}{
		{"[Grp] Show Season 1+2 (01-25) [1080p]", []int{1, 2}, []EpisodeSpan{{"", "01", "25"}}},
		{"[Grp] Show S1+S2 (01-25) [1080p]", []int{1, 2}, []EpisodeSpan{{"", "01", "25"}}},
		{"[Grp] Show S1, S2 (01-25) [1080p]", []int{1, 2}, []EpisodeSpan{{"", "01", "25"}}},
		{"[Grp] Show (Season 01 + Season 02) (01-25) [1080p]", []int{1, 2}, []EpisodeSpan{{"", "01", "25"}}},
		{"[Grp] Show Season 1 & Season 2 (01-25) [1080p]", []int{1, 2}, []EpisodeSpan{{"", "01", "25"}}},
		{"[Grp] Show S1 + S2 - 05 [1080p]", []int{1, 2}, []EpisodeSpan{{"", "05", ""}}},
		// one season, written twice or once
		{"[Grp] Show Season 1 (S01) (01-25) [1080p]", []int{1}, []EpisodeSpan{{"01", "01", "25"}}},
		{"[Grp] Show S2 (01-25) [1080p]", []int{2}, []EpisodeSpan{{"2", "01", "25"}}},
	} {
		r := ParseString(test.name)
		if v := r.Seasons(); !slices.Equal(v, test.seasons) {
			t.Errorf("%q: expected seasons %v, got: %v", test.name, test.seasons, v)
		}
		if v := r.EpisodeSpans(); !slices.Equal(v, test.spans) {
			t.Errorf("%q: expected spans %q, got: %q", test.name, test.spans, v)
		}
		season := atoi(test.spans[0].Season)
		for _, pair := range r.SeriesEpisodes() {
			if pair[0] != season {
				t.Errorf("%q: expected the episodes in season %d, got: %v", test.name, season, pair)
				break
			}
		}
		if first, last, ok := r.EpisodeRange(); ok != (test.spans[0].Last != "") || ok && (first != 1 || last != 25) {
			t.Errorf("%q: expected the range 1 to 25 where one is written, got: %d, %d, %t", test.name, first, last, ok)
		}
	}
}

// TestSeasonZeroFirst checks that Series is the first season the name writes
// when that season is 0, and not the season of a second numbering after it.
func TestSeasonZeroFirst(t *testing.T) {
	tests := []struct {
		name            string
		series, episode int
		typ             Type
	}{
		{"Show.S00E05.S01E02.1080p.WEB.x264-GRP", 0, 5, Episode},
		{"[Grp] Show - S00E02 (BD 1080p) | Other Name (S02E00) (OVA 2)", 0, 2, Episode},
		{"Show.S00-S03.1080p.BluRay.x264-GRP", 0, 0, Series},
		{"Show.S00-03.1080p.BluRay.x264-GRP", 0, 0, Series},
		// the type is the one the later seasons give: a word of a course
		// does not make it one
		{"Lynda.Course.S00-S02.1080p-GRP", 0, 0, Series},
		{"Lynda.Course.S01-S02.1080p-GRP", 1, 0, Series},
		// a season after an episode that has none is the season
		{"[Grp] Show - 05 (S01E05) [1080p]", 1, 5, Episode},
		{"Show.S01E02.S00E05.1080p.WEB.x264-GRP", 1, 2, Episode},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if r.Series != test.series || r.Episode != test.episode {
			t.Errorf("%q: expected series %d episode %d, got: %d and %d", test.name, test.series, test.episode, r.Series, r.Episode)
		}
		if r.Type != test.typ {
			t.Errorf("%q: expected type %s, got: %s", test.name, test.typ, r.Type)
		}
		if !r.HasSeason() {
			t.Errorf("%q: expected a season", test.name)
		}
	}
}

// TestHalfEpisode checks that half an episode keeps its fraction in the tag:
// Episode is the whole number, the fraction is not a word of the name, and the
// number is not an absolute one.
func TestHalfEpisode(t *testing.T) {
	tests := []struct {
		name     string
		episode  int
		fraction string
		subtitle string
	}{
		{"[Group] Show - 21.5 [720p]", 21, ".5", ""},
		{"[Group] Show - 04.1 [720p]", 4, ".1", ""},
		{"[Group] Show - 12.10 [720p]", 12, ".10", ""},
		{"[Group] Show - 21.5 - Recap [720p]", 21, ".5", "Recap"},
		{"[Group] Show - 13.5v2 [720p]", 13, ".5", ""},
		// underscores between the words
		{"[Group]_Show_-_04.1_(DVD)[B6CE8458].mkv", 4, ".1", ""},
		{"[Group]_Show_-_21.5_-_Recap_[720p].mkv", 21, ".5", "Recap"},
		{"[Group] Show - 21.5_[720p]", 21, ".5", ""},
		{"[Group] Show Ep 5.5 [720p]", 5, ".5", ""},
		{"Show.S01E06.5.720p.WEB.x264-GRP", 6, ".5", ""},
		{"Show S01E06.5 The Recap 720p WEB x264-GRP", 6, ".5", "The Recap"},
		// a video tag after the half, where the lexer sees a letter after
		// the dot
		{"Show.S01E06.5.WEB.x264-GRP", 6, ".5", ""},
		{"Show.S01E06.5.HDTV.x264-GRP", 6, ".5", ""},
		{"Show.S01E06.5.x264-GRP", 6, ".5", ""},
		// and a version after it
		{"Show.S01E02.5v2.720p.WEB.x264-GRP", 2, ".5", ""},
		// a number that begins the title of the episode or the channels
		{"Show.S02E09.4.Days.Out.720p.WEB.x264-GRP", 9, "", "4 Days Out"},
		// and the same before a word that is a tag elsewhere
		{"Show.S01E06.3.French.Hens.720p.WEB.x264-GRP", 6, "", "3 French Hens"},
		{"Show.S01E06.2.Complete.Strangers.720p.WEB.x264-GRP", 6, "", "2 Complete Strangers"},
		{"Show.S01E06.5.1.AAC.720p.WEB-GRP", 6, "", ""},
		// a dot that is not a decimal point
		{"[Group] Show Ep. 07 [720p]", 7, "", ""},
		{"[Group] Show - 05.1080p", 5, "", ""},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		var fraction string
		for _, tag := range r.Tags() {
			fraction += tag.fraction()
		}
		_, absolute := r.Absolute()
		if r.Episode != test.episode || fraction != test.fraction || r.Subtitle != test.subtitle {
			t.Errorf("%q: expected episode %d, fraction %q, subtitle %q, got: %d, %q, %q", test.name, test.episode, test.fraction, test.subtitle, r.Episode, fraction, r.Subtitle)
		}
		if absolute && test.fraction != "" {
			t.Errorf("%q: expected no absolute episode", test.name)
		}
		if pairs := r.SeriesEpisodes(); len(pairs) != 1 {
			t.Errorf("%q: expected one episode, got: %v", test.name, pairs)
		}
		if s := fmt.Sprintf("%o", r); s != test.name {
			t.Errorf("%q: expected the name back, got: %q", test.name, s)
		}
	}
	// the dot of 'Ep.' is not a decimal point
	if episode, ok := ParseString("[Group] Show Ep. 07 [720p]").Absolute(); episode != 7 || !ok {
		t.Errorf("expected absolute 7 true, got: %d %t", episode, ok)
	}
}

// TestBracketedRange checks a range of episodes in a bracket of its own that
// stands at the end of the name, where the lexers that read the end have cut
// the closing bracket off before the range is read.
func TestBracketedRange(t *testing.T) {
	tests := []struct {
		name  string
		spans []EpisodeSpan
	}{
		{"[Grp] Show Title [1-13]", []EpisodeSpan{{"", "1", "13"}}},
		{"[Grp] Show Title (01-13)", []EpisodeSpan{{"", "01", "13"}}},
		{"Show Title (01-12).mkv", []EpisodeSpan{{"", "01", "12"}}},
		{"Show_Title [1-13] [.srt]", []EpisodeSpan{{"", "1", "13"}}},
		// in the middle of the name, as before
		{"[Grp] Show Title [01-13] [1080p]", []EpisodeSpan{{"", "01", "13"}}},
		// a range of years is not one of episodes
		{"[Grp] Show Title (2019-2020)", nil},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if v := r.EpisodeSpans(); !slices.Equal(v, test.spans) {
			t.Errorf("%q: expected spans %q, got: %q", test.name, test.spans, v)
		}
		if s := fmt.Sprintf("%o", r); s != test.name {
			t.Errorf("%q: expected the name back, got: %q", test.name, s)
		}
	}
}

// TestHalfEpisodeTags checks that joining the half onto its episode leaves
// the tags the caller gave to Build as they were.
func TestHalfEpisodeTags(t *testing.T) {
	const name = "Show.S01E06.5.WEB.x264-GRP"
	p := NewParser()
	tags, end := p.Parse([]byte(name))
	// the values too, which a shallow copy shares with tags
	before := slices.Clone(tags)
	for i := range before {
		before[i].v = slices.Clone(before[i].v)
	}
	r := p.builder.Build(tags, end)
	if len(r.Tags()) != len(tags)-2 {
		t.Fatalf("expected the release to have two tags fewer than %d, got: %d", len(tags), len(r.Tags()))
	}
	for i, tag := range tags {
		if tag.typ != before[i].typ || !slices.Equal(tag.v, before[i].v) {
			t.Errorf("tag %d: expected %o as %s, got: %o as %s", i, before[i], before[i].typ, tag, tag.typ)
		}
	}
}

// TestGermanSeason checks the German words for a season and an episode, and
// the names in which they are something else.
func TestGermanSeason(t *testing.T) {
	tests := []struct {
		name            string
		series, episode int
		typ             Type
	}{
		{"Der.Denver.Clan.Staffel4.DVD7.German.FS.PAL.DVDR-RSG", 4, 0, Series},
		{"Show.Staffel.2.German.1080p.WEB.x264-GRP", 2, 0, Series},
		{"Show.Staffel02.Folge05.German.720p.HDTV.x264-GRP", 2, 5, Episode},
		{"Show.Staffel.2.Folge.5.German.720p.HDTV.x264-GRP", 2, 5, Episode},
		// the parts of an audio drama are numbered with the word, and its
		// names are music releases
		{"Die_Drei_Fragezeichen-Folge_225-Die_Nacht_der_Hexen-DE-2023-VOiCE", 0, 0, Music},
		{"Artist-Folge_2-DE-2020-GRP", 0, 0, Music},
		// a relay
		{"Leichtathletik.EM.2024.Staffel.4.x.100m.Frauen.German.720p.HDTV.x264-GRP", 0, 0, Movie},
		// a year
		{"Die.Staffel.2020.German.1080p.BluRay.x264-GRP", 0, 0, Movie},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if r.Series != test.series || r.Episode != test.episode || r.Type != test.typ {
			t.Errorf("%q: expected series %d, episode %d, type %s, got: %d, %d, %s", test.name, test.series, test.episode, test.typ, r.Series, r.Episode, r.Type)
		}
		if s := fmt.Sprintf("%o", r); s != test.name {
			t.Errorf("%q: expected the name back, got: %q", test.name, s)
		}
	}
}

func TestEpisodeSpans(t *testing.T) {
	tests := []struct {
		name string
		exp  []EpisodeSpan
	}{
		// one episode, with its leading zero
		{"Show.S01E02.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "02", ""}}},
		{"[Group] Show - 04 [720p]", []EpisodeSpan{{"", "04", ""}}},
		{"[Group] Show - 1169 [720p]", []EpisodeSpan{{"", "1169", ""}}},
		// half an episode
		{"[Group] Show - 21.5 [720p]", []EpisodeSpan{{"", "21.5", ""}}},
		{"[Group] Show - 12.10 [720p]", []EpisodeSpan{{"", "12.10", ""}}},
		{"[Group] Show - 13.5v2 [720p]", []EpisodeSpan{{"", "13.5", ""}}},
		{"Show.S01E06.5.720p.WEB.x264-GRP", []EpisodeSpan{{"01", "06.5", ""}}},
		{"[Group] Show Ep 5.5 [720p]", []EpisodeSpan{{"", "5.5", ""}}},
		// an episode 0
		{"[Group] Show - 00 [720p]", []EpisodeSpan{{"", "00", ""}}},
		{"Show.S01E00.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "00", ""}}},
		{"Show.S00E05.1080p.WEB.h264-GRP", []EpisodeSpan{{"00", "05", ""}}},
		// ranges, inside a tag and between two tags
		{"Show.S01E02-04.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "02", "04"}}},
		{"Show.S01E02-E04.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "02", "04"}}},
		{"Show.S00E02-S00E04.1080p.WEB.h264-GRP", []EpisodeSpan{{"00", "02", "04"}}},
		{"[Group] Show - 00 ~ 12 [720p]", []EpisodeSpan{{"", "00", "12"}}},
		{"[Group] Show (01-12) [720p]", []EpisodeSpan{{"", "01", "12"}}},
		// a range that ends the title, in front of the episodes
		{"[Group] Show 17-26 - 01 ~ 08 [720p]", []EpisodeSpan{{"", "01", "08"}}},
		// a range that starts at 1 is the episodes of a batch, not the title
		{"[Group] Show 01-12 - 13 [720p]", []EpisodeSpan{{"", "01", "12"}, {"", "13", ""}}},
		{"Show.S01E1-E99999.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "1", "99999"}}},
		// episodes that are not a range
		{"Show.S01E05E06.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "05", ""}, {"01", "06", ""}}},
		{"Show.S01E01.S02E01.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "01", ""}, {"02", "01", ""}}},
		{"Show.S01E02-S02E04.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "02", ""}, {"02", "04", ""}}},
		// the season written before the episode, and a season 0 after another
		{"[Group] Show S2 - 04 [720p]", []EpisodeSpan{{"2", "04", ""}}},
		{"[Group] Show 2nd Season - 08 [720p]", []EpisodeSpan{{"2", "08", ""}}},
		{"[Group] Show - S04E29 (WEB 1080p) | Other Name - E88 - S00E36", []EpisodeSpan{{"04", "29", ""}, {"04", "88", ""}, {"00", "36", ""}}},
		// a range goes on over a second hyphen
		{"Show.S01E02-E03-E04.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "02", "04"}}},
		{"Show.S01E02-E04-06.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "02", "06"}}},
		{"Show.S01E01-E02.E04-E05.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "01", "02"}, {"01", "04", "05"}}},
		{"Show.Ep.05-07.1080p.WEB.h264-GRP", []EpisodeSpan{{"", "05", "07"}}},
		{"[Group] Show S2 - 04 ~ 06 [720p]", []EpisodeSpan{{"2", "04", "06"}}},
		{"Show.S01E05-E06.5.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "05", "06.5"}}},
		// an end that is not higher, a hyphen with spaces and a tilde between
		// two tags make no range
		{"Show.S01E02-E01.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "02", ""}, {"01", "01", ""}}},
		{"Show.S01E05-E05.1080p.WEB.h264-GRP", []EpisodeSpan{{"01", "05", ""}, {"01", "05", ""}}},
		{"Show S01E02 - E04 1080p WEB h264-GRP", []EpisodeSpan{{"01", "02", ""}, {"01", "04", ""}}},
		// the season carried over a tag with no episode, and none carried back
		{"Show.S01.1080p.E05.WEB.h264-GRP", []EpisodeSpan{{"01", "05", ""}}},
		{"Show.E05.S01.1080p.WEB.h264-GRP", []EpisodeSpan{{"", "05", ""}}},
		// the channels are not a fraction
		{"Show.S01E06.5.1.AAC.720p.WEB-GRP", []EpisodeSpan{{"01", "06", ""}}},
		// a tilde makes a range as a hyphen does, inside a tag and between two
		{"[Grp] Show S04E29~E31 1080p WEB H.264", []EpisodeSpan{{"04", "29", "31"}}},
		{"Show.S01E01~03.1080p.WEB.x264-GRP", []EpisodeSpan{{"01", "01", "03"}}},
		// a range end of four digits after an episode of three or more
		{"[Grp] Show EP1156-1170 [1080p]", []EpisodeSpan{{"", "1156", "1170"}}},
		{"Show.S01E1001-1004.1080p.WEB.x264-GRP", []EpisodeSpan{{"01", "1001", "1004"}}},
		{"[Grp] Show E998-1002 [1080p]", []EpisodeSpan{{"", "998", "1002"}}},
		// four digits after a lower episode are a year or another number, as
		// are those a thousand episodes on
		{"Show.S01E05-2020.1080p.WEB.x264-GRP", []EpisodeSpan{{"01", "05", ""}}},
		{"[Grp] Show E05-2020 [1080p]", []EpisodeSpan{{"", "05", ""}}},
		{"Show.S01E05-1080.WEB.x264-GRP", []EpisodeSpan{{"01", "05", ""}}},
		{"Show.S01E100-9999.WEB.x264-GRP", []EpisodeSpan{{"01", "100", ""}}},
		// half an episode with a tag or a version right after it
		{"Show.S01E06.5.WEB.x264-GRP", []EpisodeSpan{{"01", "06.5", ""}}},
		{"Show.S01E02.5v2.720p.WEB.x264-GRP", []EpisodeSpan{{"01", "02.5", ""}}},
		// one episode under two numberings, in the order written
		{"Show.S23E19.Episode.1174.1080p.WEB.h264-GRP", []EpisodeSpan{{"23", "19", ""}, {"23", "1174", ""}}},
		// no episode
		{"Show.S01.1080p.WEB.h264-GRP", nil},
		{"Show.S01-S03.1080p.WEB.h264-GRP", nil},
		{"Show.2024.10.13.1080p.WEB.h264-GRP", nil},
		{"Movie.2020.1080p.BluRay.x264-GRP", nil},
	}
	for _, test := range tests {
		if v := ParseString(test.name).EpisodeSpans(); !slices.Equal(v, test.exp) {
			t.Errorf("%q: expected spans %q, got: %q", test.name, test.exp, v)
		}
	}
	// by hand
	if v := (Release{Series: 2, Episode: 5}).EpisodeSpans(); !slices.Equal(v, []EpisodeSpan{{"2", "5", ""}}) {
		t.Errorf("expected the episode set by hand, got: %q", v)
	}
	if v := (Release{Series: 2}).EpisodeSpans(); v != nil {
		t.Errorf("expected no span, got: %q", v)
	}
	// by hand over a parsed season pack, which HasEpisode counts too
	r := ParseString("Show.S01.1080p.WEB.h264-GRP")
	r.Episode = 4
	if v := r.EpisodeSpans(); !slices.Equal(v, []EpisodeSpan{{"1", "4", ""}}) || !r.HasEpisode() {
		t.Errorf("expected the episode set by hand, got: %q", v)
	}
}
