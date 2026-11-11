package rls

import (
	"slices"
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
		// a number that begins the title of the episode or the channels
		{"Show.S02E09.4.Days.Out.720p.WEB.x264-GRP", 9, "", "4 Days Out"},
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
	}
	// the dot of 'Ep.' is not a decimal point
	if episode, ok := ParseString("[Group] Show Ep. 07 [720p]").Absolute(); episode != 7 || !ok {
		t.Errorf("expected absolute 7 true, got: %d %t", episode, ok)
	}
}
