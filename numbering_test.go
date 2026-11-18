package rls

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// numbering is what a Numbering gives for a release: whether it has a season
// and an episode, its seasons, and the range of its episodes.
type numbering struct {
	season, episode bool
	seasons         []int
	first, last     int
	ok              bool
}

func numberingOf(r Release) numbering {
	n := r.Numbering
	_, season := n.Season()
	_, episode := n.Episode()
	first, last, ok := episodeRange(n)
	return numbering{
		season:  season,
		episode: episode,
		seasons: n.Seasons,
		first:   first,
		last:    last,
		ok:      ok,
	}
}

// ep returns one episode, or one range of episodes, of a season.
func ep(season, first, last int) Ep {
	return Ep{Season: season, First: first, Last: last}
}

// episodeRange returns the first and the last episode when n gives more than
// one ('S01E05E06', 'S01E02-04', '01-12'). The episodes are those of the
// season the first episode is in, so 'S01E01.S02E01' is not a range, and
// neither is one episode under two numberings ('S23E19.Episode.1174').
func episodeRange(n Numbering) (first, last int, ok bool) {
	for i, e := range n.Episodes {
		switch {
		case i == 0:
			first, last = e.First, e.Last
		case e.Season == n.Episodes[0].Season:
			first, last = min(first, e.First, e.Last), max(last, e.First, e.Last)
		}
	}
	if first == last {
		return 0, 0, false
	}
	return first, last, true
}

// pairs returns each season and episode that n.All yields, once and in rising
// order, with NoSeason for an episode that has no season.
func pairs(n Numbering) [][]int {
	var v [][]int
	for season, episode := range n.All() {
		v = append(v, []int{season, episode})
	}
	slices.SortFunc(v, slices.Compare[[]int])
	return slices.CompactFunc(v, slices.Equal[[]int])
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
		// the two ends of a range that is too wide for All to fill
		{"Show.S01E1-E99999.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1}, first: 1, last: 99999, ok: true}},
		// one episode under two numberings
		{"Show.S23E19.Episode.1174.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{23}}},
		// one episode in two seasons is not a range
		{"Show.S01E01.S02E01.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{1, 2}}},
		// the episodes are those of the season written first, the one Season
		// gives, and the seasons are in the order written
		{"Show.S02E01.S01E05-E06.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{2, 1}}},
		{"Show.S02E01E02.S01E05.1080p.WEB.h264-GRP", numbering{season: true, episode: true, seasons: []int{2, 1}, first: 1, last: 2, ok: true}},
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

// TestNumberingByHand checks that a Numbering made by hand gives what its
// maker put in it, and that the methods read the value alone: a release has
// the numbering it holds, whatever its tags say.
func TestNumberingByHand(t *testing.T) {
	type answers struct {
		season, episode, absolute int
		seasoned, numbered, abs   bool
		shape                     Shape
		pairs                     [][]int
	}
	parsed := ParseString("Show.S01E02.1080p.WEB.h264-GRP")
	// a caller clears the numbering of a parsed release, or replaces it to
	// make a pack read as one of its episodes
	cleared, replaced := parsed, ParseString("Show.S01.1080p.WEB.h264-GRP")
	cleared.Numbering = Numbering{}
	replaced.Numbering.Episodes = []Ep{ep(1, 4, 4)}
	tests := []struct {
		name string
		n    Numbering
		exp  answers
	}{
		{"zero", Numbering{}, answers{}},
		{"season", Numbering{Scheme: SchemeSeasoned, Seasons: []int{3}}, answers{season: 3, seasoned: true, shape: ShapeSeason}},
		{"seasons", Numbering{Scheme: SchemeSeasoned, Seasons: []int{3, 1}}, answers{season: 3, seasoned: true, shape: ShapeSeasons}},
		{"season 0", Numbering{Seasons: []int{0}}, answers{seasoned: true, shape: ShapeSeason}},
		{
			"episode", Numbering{Scheme: SchemeAbsolute, Episodes: []Ep{ep(NoSeason, 4, 4)}},
			answers{episode: 4, numbered: true, absolute: 4, abs: true, shape: ShapeEpisode, pairs: [][]int{{NoSeason, 4}}},
		},
		{
			"episode 0", Numbering{Episodes: []Ep{ep(NoSeason, 0, 0)}},
			answers{numbered: true, abs: true, shape: ShapeEpisode, pairs: [][]int{{NoSeason, 0}}},
		},
		{
			"season and episode", Numbering{Scheme: SchemeSeasoned, Seasons: []int{1}, Episodes: []Ep{ep(1, 2, 2)}},
			answers{season: 1, seasoned: true, episode: 2, numbered: true, shape: ShapeEpisode, pairs: [][]int{{1, 2}}},
		},
		// the season of an episode is a season, with nothing in Seasons
		{
			"episode in a season", Numbering{Episodes: []Ep{ep(2, 5, 5)}},
			answers{season: 2, seasoned: true, episode: 5, numbered: true, shape: ShapeEpisode, pairs: [][]int{{2, 5}}},
		},
		// the zero Ep is in season 0, which qualifies its episode as the
		// specials do
		{
			"zero ep", Numbering{Episodes: []Ep{{First: 5, Last: 5}}},
			answers{seasoned: true, episode: 5, numbered: true, shape: ShapeEpisode, pairs: [][]int{{0, 5}}},
		},
		{
			"range", Numbering{Episodes: []Ep{ep(NoSeason, 1, 12)}},
			answers{episode: 1, numbered: true, shape: ShapeEpisodes, pairs: [][]int{{NoSeason, 1}, {NoSeason, 2}, {NoSeason, 3}, {NoSeason, 4}, {NoSeason, 5}, {NoSeason, 6}, {NoSeason, 7}, {NoSeason, 8}, {NoSeason, 9}, {NoSeason, 10}, {NoSeason, 11}, {NoSeason, 12}}},
		},
		{
			"half an episode", Numbering{Episodes: []Ep{{Season: NoSeason, First: 21, Last: 21, Frac: 5}}},
			answers{episode: 21, numbered: true, shape: ShapeEpisode, pairs: [][]int{{NoSeason, 21}}},
		},
		{
			"second numbering", Numbering{Seasons: []int{23}, Episodes: []Ep{ep(23, 19, 19)}, Alt: []Ep{ep(NoSeason, 1174, 1174)}},
			answers{season: 23, seasoned: true, episode: 19, numbered: true, absolute: 1174, abs: true, shape: ShapeEpisode, pairs: [][]int{{23, 19}}},
		},
		{
			"the episode again", Numbering{Seasons: []int{2}, Episodes: []Ep{ep(2, 6, 6)}, Alt: []Ep{ep(NoSeason, 6, 6)}},
			answers{season: 2, seasoned: true, episode: 6, numbered: true, shape: ShapeEpisode, pairs: [][]int{{2, 6}}},
		},
		// an extra is no regular episode: it is in Episodes with its Kind,
		// and Episode, Absolute, Shape and All pass over it
		{"an extra", Numbering{Episodes: []Ep{{Season: NoSeason, First: 2, Last: 2, Kind: EpOVA}}}, answers{}},
		{"extras", Numbering{Episodes: []Ep{{Season: NoSeason, First: 3, Last: 3, Kind: EpSpecial}, {Season: NoSeason, First: 1, Last: 2, Kind: EpOP}}}, answers{}},
		{
			"an extra of a season", Numbering{Seasons: []int{2}, Episodes: []Ep{{Season: 2, First: 1, Last: 1, Kind: EpOVA}}},
			answers{season: 2, seasoned: true, shape: ShapeSeason},
		},
		{
			"an extra and an episode", Numbering{Episodes: []Ep{{Season: NoSeason, First: 2, Last: 2, Kind: EpSpecial}, ep(NoSeason, 7, 7)}},
			answers{episode: 7, numbered: true, absolute: 7, abs: true, shape: ShapeEpisode, pairs: [][]int{{NoSeason, 7}}},
		},
		{
			"an episode and extras", Numbering{Episodes: []Ep{ep(NoSeason, 7, 7), {Season: NoSeason, First: 1, Last: 1, Kind: EpPV}, {Season: NoSeason, First: 1, Last: 1, Kind: EpED}}},
			answers{episode: 7, numbered: true, absolute: 7, abs: true, shape: ShapeEpisode, pairs: [][]int{{NoSeason, 7}}},
		},
		{
			"an extra and a range", Numbering{Episodes: []Ep{{Season: NoSeason, First: 1, Last: 1, Kind: EpOVA}, ep(NoSeason, 1, 3)}},
			answers{episode: 1, numbered: true, shape: ShapeEpisodes, pairs: [][]int{{NoSeason, 1}, {NoSeason, 2}, {NoSeason, 3}}},
		},
		// more than one episode has no absolute number
		{
			"two episodes", Numbering{Episodes: []Ep{ep(NoSeason, 0, 0), ep(NoSeason, 5, 5)}},
			answers{numbered: true, shape: ShapeEpisodes, pairs: [][]int{{NoSeason, 0}, {NoSeason, 5}}},
		},
		{
			"an episode and a range", Numbering{Episodes: []Ep{ep(NoSeason, 13, 13), ep(NoSeason, 1, 2)}},
			answers{episode: 13, numbered: true, shape: ShapeEpisodes, pairs: [][]int{{NoSeason, 1}, {NoSeason, 2}, {NoSeason, 13}}},
		},
		// an Ep that ends below its start is one episode
		{
			"last below first", Numbering{Episodes: []Ep{ep(NoSeason, 5, 3)}},
			answers{episode: 5, numbered: true, absolute: 5, abs: true, shape: ShapeEpisode, pairs: [][]int{{NoSeason, 5}}},
		},
		{"daily", Numbering{Scheme: SchemeDaily}, answers{shape: ShapeDaily}},
		{"daily with a season", Numbering{Scheme: SchemeDaily, Seasons: []int{1}}, answers{season: 1, seasoned: true, shape: ShapeDaily}},
		{"cleared", cleared.Numbering, answers{}},
		{
			"replaced", replaced.Numbering,
			answers{season: 1, seasoned: true, episode: 4, numbered: true, shape: ShapeEpisode, pairs: [][]int{{1, 4}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got answers
			got.season, got.seasoned = test.n.Season()
			got.episode, got.numbered = test.n.Episode()
			got.absolute, got.abs = test.n.Absolute()
			got.shape, got.pairs = test.n.Shape(), pairs(test.n)
			if !reflect.DeepEqual(got, test.exp) {
				t.Errorf("got %+v, expected %+v", got, test.exp)
			}
		})
	}
	// the tags of the two releases are those of the parse
	if s := fmt.Sprintf("%o", cleared); s != "Show.S01E02.1080p.WEB.h264-GRP" {
		t.Errorf("expected the name back, got: %q", s)
	}
	// what Compare and Event read is the numbering the release holds
	if Compare(parsed, cleared) <= 0 {
		t.Errorf("expected the release with no numbering to sort before the one with an episode")
	}
	event := Release{Type: Episode, Title: "Formula1", Numbering: Numbering{Seasons: []int{2024}, Episodes: []Ep{ep(2024, 5, 5)}}}
	if ev, ok := event.Event(); !ok || ev.Round != 5 {
		t.Errorf("expected round 5 of an event, got: %+v %t", ev, ok)
	}
}

// TestNumberingAllocs checks that the methods of a Numbering allocate nothing,
// since they are asked of every release a caller holds.
func TestNumberingAllocs(t *testing.T) {
	for _, name := range []string{
		"Show.S01E02-E04.1080p.WEB.h264-GRP",
		"Show.S01-S03.1080p.BluRay.x264-GRP",
		"[Grp] Show - 01-12 [1080p]",
		"Show.S23E19.Episode.1174.1080p.WEB.h264-GRP",
	} {
		n := ParseString(name).Numbering
		for _, test := range []struct {
			name string
			f    func()
		}{
			{"Season", func() { _, _ = n.Season() }},
			{"Episode", func() { _, _ = n.Episode() }},
			{"Absolute", func() { _, _ = n.Absolute() }},
			{"Shape", func() { _ = n.Shape() }},
			{"All", func() {
				for season, episode := range n.All() {
					_, _ = season, episode
				}
			}},
		} {
			if n := testing.AllocsPerRun(100, test.f); n != 0 {
				t.Errorf("%s: %s allocates %v times, expected none", name, test.name, n)
			}
		}
	}
}

func equalNumbering(a, b numbering) bool {
	return a.season == b.season && a.episode == b.episode && slices.Equal(a.seasons, b.seasons) &&
		a.first == b.first && a.last == b.last && a.ok == b.ok
}

// TestEpisodeZero checks that a written episode 0 is an episode: All yields
// it, a range starts at it, and with no season it is absolute.
func TestEpisodeZero(t *testing.T) {
	tests := []struct {
		name     string
		exp      [][]int
		absolute bool
	}{
		{"[Group] Show - 00 [720p]", [][]int{{NoSeason, 0}}, true},
		{"[Group] Show - 00v2 [720p]", [][]int{{NoSeason, 0}}, true},
		{"[Group] Show - Episode 0 [720p]", [][]int{{NoSeason, 0}}, true},
		{"Show.E00.1080p.WEB.h264-GRP", [][]int{{NoSeason, 0}}, true},
		// a season qualifies it
		{"Show.S01E00.1080p.WEB.h264-GRP", [][]int{{1, 0}}, false},
		{"Show.1x00.HDTV.x264-GRP", [][]int{{1, 0}}, false},
		{"[Group] Show S2 - 00 [1080p]", [][]int{{2, 0}}, false},
		{"Show.S00E00.1080p.WEB.h264-GRP", [][]int{{0, 0}}, false},
		// ranges that start at 0
		{"Show.S01E00E01.1080p.WEB.h264-GRP", [][]int{{1, 0}, {1, 1}}, false},
		{"Show.S01E00-E02.1080p.WEB.h264-GRP", [][]int{{1, 0}, {1, 1}, {1, 2}}, false},
		{"[Group] Show - 00-03 [720p]", [][]int{{NoSeason, 0}, {NoSeason, 1}, {NoSeason, 2}, {NoSeason, 3}}, false},
		{"[Group] Show - 00 ~ 02 [720p]", [][]int{{NoSeason, 0}, {NoSeason, 1}, {NoSeason, 2}}, false},
		// a range that ends at 0 is its two ends
		{"Show.S01E03-E00.1080p.WEB.h264-GRP", [][]int{{1, 0}, {1, 3}}, false},
		// with half an episode or a range after the 0 the name gives more
		// than one episode, and has no absolute number
		{"[Group] Show - 00 (Ep 5.5) [720p]", [][]int{{NoSeason, 0}, {NoSeason, 5}}, false},
		{"[Group] Show - 00 - E05-07 [720p]", [][]int{{NoSeason, 0}, {NoSeason, 5}, {NoSeason, 6}, {NoSeason, 7}}, false},
		// half an episode has no absolute number
		{"[Group] Show - 00.5 [720p]", [][]int{{NoSeason, 0}}, false},
		// no episode written
		{"Show.S01.1080p.WEB.h264-GRP", nil, false},
		{"Show.2024.1080p.WEB.h264-GRP", nil, false},
	}
	for _, test := range tests {
		n := ParseString(test.name).Numbering
		if v := pairs(n); !slices.EqualFunc(v, test.exp, slices.Equal[[]int]) {
			t.Errorf("%q: expected series episodes %v, got: %v", test.name, test.exp, v)
		}
		if episode, ok := n.Absolute(); episode != 0 || ok != test.absolute {
			t.Errorf("%q: expected absolute 0 %t, got: %d %t", test.name, test.absolute, episode, ok)
		}
		if _, ok := n.Episode(); ok != (test.exp != nil) {
			t.Errorf("%q: expected an episode %t, got: %t", test.name, test.exp != nil, ok)
		}
	}
	for _, tag := range ParseString("Show.S01E00.1080p.WEB.h264-GRP").Tags() {
		if v := tag.Episodes(); tag.Is(TagTypeSeries) && !slices.Equal(v, []int{0}) {
			t.Errorf("expected episodes [0], got: %v", v)
		}
	}
	// two episodes have no absolute number, and Episode gives the first,
	// which is 0 and an episode
	n := ParseString("[Group] Show - 00 - 05 [720p]").Numbering
	if episode, ok := n.Absolute(); episode != 0 || ok {
		t.Errorf("expected no absolute episode, got: %d %t", episode, ok)
	}
	if episode, ok := n.Episode(); episode != 0 || !ok {
		t.Errorf("expected episode 0 true, got: %d %t", episode, ok)
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
		{"[Grp] Show E05~E07 [1080p]", [][]int{{NoSeason, 5}, {NoSeason, 6}, {NoSeason, 7}}},
		// two seasons are not a range
		{"Show.S01E02~S02E04.1080p.WEB.h264-GRP", [][]int{{1, 2}, {2, 4}}},
	}
	for _, test := range tests {
		n := ParseString(test.name).Numbering
		if v := pairs(n); !slices.EqualFunc(v, test.exp, slices.Equal[[]int]) {
			t.Errorf("%q: expected series episodes %v, got: %v", test.name, test.exp, v)
		}
		first, last, ok := episodeRange(n)
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
		if v := pairs(ParseString(test.name).Numbering); !slices.EqualFunc(v, test.exp, slices.Equal[[]int]) {
			t.Errorf("%q: expected series episodes %v, got: %v", test.name, test.exp, v)
		}
	}
}

// TestSeriesEpisodesSecondNumbering checks that an episode written without a
// season beside one written with a season and an episode is a second
// numbering of that episode and not another episode, and that an episode
// written twice is listed once.
func TestSeriesEpisodesSecondNumbering(t *testing.T) {
	tests := []struct {
		name    string
		exp     [][]int
		episode int
	}{
		{"Formula.E.DRIVER.S02E01.Episode.1.1080p.AMZN.WEB-DL.DDP2.0.H.264-FFG.mkv", [][]int{{2, 1}}, 1},
		{"Show S01E01 Episode 1 1080p WEB H264-GRP", [][]int{{1, 1}}, 1},
		{"Show.S01E01.Ep.1.1080p.WEB.h264-GRP", [][]int{{1, 1}}, 1},
		{"Show.1x01.Episode.1.HDTV.x264-GRP", [][]int{{1, 1}}, 1},
		{"Show.S23E19.Episode.1174.1080p.WEB.h264-GRP", [][]int{{23, 19}}, 19},
		// the second numbering disagrees, and the season's episode is the one
		{"Show.S02E01.Episode.3.1080p.WEB.h264-GRP", [][]int{{2, 1}}, 1},
		{"[Group] Show (2024) - 05 (S01E05) (BD 1080p x264 8-bit EAC-3) [Dual Audio]", [][]int{{1, 5}}, 5},
		{"Fairy Tail - S06E32 - Tartaros Arc Iron Fist of the Fire Dragon [Episode 83]", [][]int{{6, 32}}, 32},
		// a second numbering starts no range
		{"Show.S01E01.Episode.5-E07.1080p.WEB.h264-GRP", [][]int{{1, 1}, {1, 7}}, 1},
		// the same episode written twice
		{"Show.S01E01.S01E01.1080p.WEB.h264-GRP", [][]int{{1, 1}}, 1},
		{"Show.S11E11-GRP/Show.S11E11-GRP.mkv", [][]int{{11, 11}}, 11},
		// episodes that are distinct
		{"Show.S01E01E02.1080p.WEB.h264-GRP", [][]int{{1, 1}, {1, 2}}, 1},
		{"Show.S01E01-E03.1080p.WEB.h264-GRP", [][]int{{1, 1}, {1, 2}, {1, 3}}, 1},
		{"Show.S01E01.S01E02.1080p.WEB.h264-GRP", [][]int{{1, 1}, {1, 2}}, 1},
		{"Show.S01E05.E06.1080p.WEB.h264-GRP", [][]int{{1, 5}, {1, 6}}, 5},
		{"[Group] Show - S04E29 (WEB 1080p) | Other Name - E88 - S00E36", [][]int{{0, 36}, {4, 29}, {4, 88}}, 29},
		// a season with no episode leaves the episode after it in the season,
		// unless the name writes that season's episode too
		{"[Grp] Show S2 - 04 [1080p].mkv", [][]int{{2, 4}}, 4},
		{"Show.S01E01.S02.Episode.2.1080p.WEB.h264-GRP", [][]int{{1, 1}, {2, 2}}, 1},
		// no season written
		{"Show.Episode.1.1080p.WEB.h264-GRP", [][]int{{NoSeason, 1}}, 1},
		// a part is not an episode
		{"Show.S01E01.Part.1.1080p.WEB.h264-GRP", [][]int{{1, 1}}, 1},
		{"Show.S01E02.Part.2.1080p.WEB.h264-GRP", [][]int{{1, 2}}, 2},
	}
	for _, test := range tests {
		n := ParseString(test.name).Numbering
		if v := pairs(n); !slices.EqualFunc(v, test.exp, slices.Equal[[]int]) {
			t.Errorf("%q: expected series episodes %v, got: %v", test.name, test.exp, v)
		}
		if episode, _ := n.Episode(); episode != test.episode {
			t.Errorf("%q: expected episode %d, got: %d", test.name, test.episode, episode)
		}
	}
	// season 2 writes its episode in '(S02E01)', so the 13 after 'S2' is a
	// second numbering
	name := "[Grp] Show S2 - 13 (S02E01) [1080p].mkv"
	n := ParseString(name).Numbering
	if v, exp := pairs(n), [][]int{{2, 1}}; !slices.EqualFunc(v, exp, slices.Equal[[]int]) {
		t.Errorf("%q: expected series episodes %v, got: %v", name, exp, v)
	}
	if exp := []Ep{ep(NoSeason, 13, 13)}; !slices.Equal(n.Alt, exp) {
		t.Errorf("%q: expected the second numbering %v, got: %v", name, exp, n.Alt)
	}
}

// TestAbsoluteSecondNumbering checks that an episode written without a season
// beside one written with a season and an episode is the absolute episode,
// and that Episode gives the season's episode wherever the two are written.
func TestAbsoluteSecondNumbering(t *testing.T) {
	tests := []struct {
		name     string
		episode  int
		absolute int
		ok       bool
	}{
		{"Show.S23E19.Episode.1174.1080p.WEB.h264-GRP", 19, 1174, true},
		{"Fairy Tail - S06E32 - Tartaros Arc Iron Fist of the Fire Dragon [Episode 83]", 32, 83, true},
		{"[Grp] Show - 1174 (S23E19) [1080p]", 19, 1174, true},
		{"[Group] Show (2024) - 13 (S02E01) (BD 1080p x264 8-bit EAC-3) [Dual Audio]", 1, 13, true},
		{"[Grp] Show S2 - 13 (S02E01) [1080p].mkv", 1, 13, true},
		// the season's episode written again is no absolute number
		{"Formula.E.DRIVER.S02E01.Episode.1.1080p.AMZN.WEB-DL.DDP2.0.H.264-FFG.mkv", 1, 0, false},
		{"Noragami - S02E06 - What Must Be Done [Episode 6]", 6, 0, false},
		{"[Group] Show (2024) - 01 (S01E01) (BD 1080p x264 8-bit EAC-3) [Dual Audio]", 1, 0, false},
		// a special is qualified by its season 0
		{"Show.S00E05.Episode.12.1080p.WEB.h264-GRP", 5, 0, false},
		// a continuation is another episode, and a range or half an episode
		// has no absolute number
		{"Show.S01E05.E06.1080p.WEB.h264-GRP", 5, 0, false},
		{"Show.S01E01.Episode.5-7.1080p.WEB.h264-GRP", 1, 0, false},
		{"Show.S01E01.Episode.5.5.1080p.WEB.h264-GRP", 1, 0, false},
		// the end of a range that a version follows
		{"[Judas] Shingeki no Kyojin (Attack on Titan) - S04E29-31v2 [1080p][HEVC x265 10bit][Multi-Subs] (Weekly)", 29, 0, false},
		{"Show.S04E29-31v2.1080p.WEB.H264-GRP", 29, 0, false},
		{"Show.S04E29~31v2.1080p.WEB.H264-GRP", 29, 0, false},
		{"Show.S01E998-1002v2.1080p.WEB.H264-GRP", 998, 0, false},
		// four digits after a lower episode are another number, as they are
		// with no version after them, and so is an episode written with a word
		{"Show.S23E19-1174v2.1080p.WEB.H264-GRP", 19, 1174, true},
		{"Show.S01E05-2020v2.1080p.WEB.H264-GRP", 5, 2020, true},
		{"Show.S01E05-Episode.10.1080p.WEB.H264-GRP", 5, 10, true},
		// a season with no episode leaves the episode after it in the season
		{"[Grp] Show S2 - 04 [1080p].mkv", 4, 0, false},
		{"Show.S01E01.S02.Episode.2.1080p.WEB.h264-GRP", 1, 0, false},
		// no season written
		{"[Group] One Piece - 1169 [1080p].mkv", 1169, 1169, true},
	}
	for _, test := range tests {
		n := ParseString(test.name).Numbering
		if episode, _ := n.Episode(); episode != test.episode {
			t.Errorf("%q: expected episode %d, got: %d", test.name, test.episode, episode)
		}
		if absolute, ok := n.Absolute(); absolute != test.absolute || ok != test.ok {
			t.Errorf("%q: expected absolute %d %t, got: %d %t", test.name, test.absolute, test.ok, absolute, ok)
		}
	}
}

// TestSeasonRange checks a range of seasons written with one season word:
// Season gives the first season, Seasons every season of the range, the
// release is a pack, and the last season is neither the subtitle nor a
// leftover word. An episode written beside the range is in no season.
func TestSeasonRange(t *testing.T) {
	tests := []struct {
		name    string
		seasons []int
		spans   []Ep
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
		{"[Grp] Show Season 1-4 (1-76) [1080p]", []int{1, 2, 3, 4}, []Ep{ep(NoSeason, 1, 76)}},
		{"[Grp] Show 001-367 (Seasons 1-10) [BD 1080p]", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, []Ep{ep(NoSeason, 1, 367)}},
		{"[Grp] Show S01-04 (1-76) [1080p]", []int{1, 2, 3, 4}, []Ep{ep(NoSeason, 1, 76)}},
		{"[Grp] Show S01-S04 (1-76) [1080p]", []int{1, 2, 3, 4}, []Ep{ep(NoSeason, 1, 76)}},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		n := r.Numbering
		if !slices.Equal(n.Seasons, test.seasons) {
			t.Errorf("%q: expected seasons %v, got: %v", test.name, test.seasons, n.Seasons)
		}
		if r.Type != Series {
			t.Errorf("%q: expected type series, got: %s", test.name, r.Type)
		}
		if season, ok := n.Season(); season != test.seasons[0] || !ok {
			t.Errorf("%q: expected season %d, got: %d %t", test.name, test.seasons[0], season, ok)
		}
		if !slices.Equal(n.Episodes, test.spans) {
			t.Errorf("%q: expected episodes %v, got: %v", test.name, test.spans, n.Episodes)
		}
		// a range of seasons alone is several seasons, and with a range of
		// episodes beside it the release is its episodes
		if exp := map[bool]Shape{false: ShapeSeasons, true: ShapeEpisodes}[len(test.spans) != 0]; n.Shape() != exp {
			t.Errorf("%q: expected shape %s, got: %s", test.name, exp, n.Shape())
		}
		for season := range n.All() {
			if season != NoSeason {
				t.Errorf("%q: expected the episodes in no season, got one in season %d", test.name, season)
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
		spans  []Ep
	}{
		// a season and its episode
		{"[Grp] Show Season 3 - 03 [1080p]", 3, []Ep{ep(3, 3, 3)}},
		{"[Grp] Show Season 1 - 12 [1080p]", 1, []Ep{ep(1, 12, 12)}},
		{"[Grp] Show Season 2 - 8 [1080p]", 2, []Ep{ep(2, 8, 8)}},
		{"[Grp] Show S2 - 04 [1080p]", 2, []Ep{ep(2, 4, 4)}},
		{"[Grp] Show S01 - 04 [1080p]", 1, []Ep{ep(1, 4, 4)}},
		{"[Grp] Show S01 - 04 (1080p)", 1, []Ep{ep(1, 4, 4)}},
		{"[Grp] Show S01 - 12 + OVA [1080p]", 1, []Ep{ep(1, 12, 12)}},
		{"[Grp] Show Season 1 - 12 + OVA [1080p]", 1, []Ep{ep(1, 12, 12)}},
		{"[Grp] Show (S01 - 12) [1080p]", 1, []Ep{ep(1, 12, 12)}},
	}
	for _, test := range notRanges {
		n := ParseString(test.name).Numbering
		if !slices.Equal(n.Seasons, []int{test.season}) {
			t.Errorf("%q: expected season %d alone, got: %v", test.name, test.season, n.Seasons)
		}
		if !slices.Equal(n.Episodes, test.spans) {
			t.Errorf("%q: expected episodes %v, got: %v", test.name, test.spans, n.Episodes)
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
		if v := ParseString(test.name).Numbering.Seasons; !slices.Equal(v, test.seasons) {
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
		n := r.Numbering
		if !slices.Equal(n.Seasons, test.seasons) {
			t.Errorf("%q: expected seasons %v, got: %v", test.name, test.seasons, n.Seasons)
		}
		if season, _ := n.Season(); r.Type != Series || season != test.seasons[0] || !strings.HasPrefix(r.Title, "Show") {
			t.Errorf("%q: expected series %d of Show, got: %s %d of %q", test.name, test.seasons[0], r.Type, season, r.Title)
		}
		if n.Episodes != nil || n.Alt != nil || n.Shape() != ShapeSeasons {
			t.Errorf("%q: expected seasons and no episode, got: %v %v, %s", test.name, n.Episodes, n.Alt, n.Shape())
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
		if v := r.Numbering.Seasons; !slices.Equal(v, test.seasons) {
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
		if r := ParseString(name); r.Year != 2020 || r.Month != 12 || r.Day != 24 || !slices.Equal(r.Numbering.Seasons, []int{1}) {
			t.Errorf("%q: expected season 1 and 2020-12-24, got: %v and %d-%d-%d", name, r.Numbering.Seasons, r.Year, r.Month, r.Day)
		}
	}
}

// TestSeasonListEpisodes checks that episodes written beside a list of
// seasons with no season of their own are in none of them, as beside a range.
func TestSeasonListEpisodes(t *testing.T) {
	for _, test := range []struct {
		name    string
		seasons []int
		spans   []Ep
	}{
		{"[Grp] Show Season 1+2 (01-25) [1080p]", []int{1, 2}, []Ep{ep(NoSeason, 1, 25)}},
		{"[Grp] Show S1+S2 (01-25) [1080p]", []int{1, 2}, []Ep{ep(NoSeason, 1, 25)}},
		{"[Grp] Show S1, S2 (01-25) [1080p]", []int{1, 2}, []Ep{ep(NoSeason, 1, 25)}},
		{"[Grp] Show (Season 01 + Season 02) (01-25) [1080p]", []int{1, 2}, []Ep{ep(NoSeason, 1, 25)}},
		{"[Grp] Show Season 1 & Season 2 (01-25) [1080p]", []int{1, 2}, []Ep{ep(NoSeason, 1, 25)}},
		{"[Grp] Show S1 + S2 - 05 [1080p]", []int{1, 2}, []Ep{ep(NoSeason, 5, 5)}},
		// one season, written twice or once
		{"[Grp] Show Season 1 (S01) (01-25) [1080p]", []int{1}, []Ep{ep(1, 1, 25)}},
		{"[Grp] Show S2 (01-25) [1080p]", []int{2}, []Ep{ep(2, 1, 25)}},
	} {
		n := ParseString(test.name).Numbering
		if !slices.Equal(n.Seasons, test.seasons) {
			t.Errorf("%q: expected seasons %v, got: %v", test.name, test.seasons, n.Seasons)
		}
		if !slices.Equal(n.Episodes, test.spans) {
			t.Errorf("%q: expected episodes %v, got: %v", test.name, test.spans, n.Episodes)
		}
		for season := range n.All() {
			if season != test.spans[0].Season {
				t.Errorf("%q: expected the episodes in season %d, got one in season %d", test.name, test.spans[0].Season, season)
				break
			}
		}
		if first, last, ok := episodeRange(n); ok != (test.spans[0].First != test.spans[0].Last) || ok && (first != 1 || last != 25) {
			t.Errorf("%q: expected the range 1 to 25 where one is written, got: %d, %d, %t", test.name, first, last, ok)
		}
	}
}

// TestSeasonZeroFirst checks that Season gives the first season the name
// writes when that season is 0, and not the season of a second numbering
// after it.
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
		season, ok := r.Numbering.Season()
		if episode, _ := r.Numbering.Episode(); season != test.series || episode != test.episode {
			t.Errorf("%q: expected series %d episode %d, got: %d and %d", test.name, test.series, test.episode, season, episode)
		}
		if r.Type != test.typ {
			t.Errorf("%q: expected type %s, got: %s", test.name, test.typ, r.Type)
		}
		if !ok {
			t.Errorf("%q: expected a season", test.name)
		}
	}
}

// TestHalfEpisode checks that half an episode keeps its fraction in the tag
// and in Frac: Episode gives the whole number, the fraction is not a word of
// the name, and the number is not an absolute one.
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
		n := r.Numbering
		var fraction string
		for _, tag := range r.Tags() {
			fraction += tag.fraction()
		}
		episode, _ := n.Episode()
		_, absolute := n.Absolute()
		if episode != test.episode || fraction != test.fraction || r.Subtitle != test.subtitle {
			t.Errorf("%q: expected episode %d, fraction %q, subtitle %q, got: %d, %q, %q", test.name, test.episode, test.fraction, test.subtitle, episode, fraction, r.Subtitle)
		}
		if absolute && test.fraction != "" {
			t.Errorf("%q: expected no absolute episode", test.name)
		}
		if n.Shape() != ShapeEpisode {
			t.Errorf("%q: expected one episode, got: %v", test.name, n.Episodes)
		}
		// Frac is the digits of the fraction as a number, without the zeros
		// that end them: '.10' is the tenth that '.1' is
		if frac := strings.TrimRight(strings.TrimPrefix(test.fraction, "."), "0"); len(n.Episodes) != 1 || int(n.Episodes[0].Frac) != atoi(frac) {
			t.Errorf("%q: expected the fraction %q in Frac, got: %v", test.name, frac, n.Episodes)
		}
		if s := fmt.Sprintf("%o", r); s != test.name {
			t.Errorf("%q: expected the name back, got: %q", test.name, s)
		}
	}
	// the dot of 'Ep.' is not a decimal point
	if episode, ok := ParseString("[Group] Show Ep. 07 [720p]").Numbering.Absolute(); episode != 7 || !ok {
		t.Errorf("expected absolute 7 true, got: %d %t", episode, ok)
	}
}

// TestBracketedRange checks a range of episodes in a bracket of its own that
// stands at the end of the name, where the lexers that read the end have cut
// the closing bracket off before the range is read.
func TestBracketedRange(t *testing.T) {
	tests := []struct {
		name  string
		spans []Ep
	}{
		{"[Grp] Show Title [1-13]", []Ep{ep(NoSeason, 1, 13)}},
		{"[Grp] Show Title (01-13)", []Ep{ep(NoSeason, 1, 13)}},
		{"Show Title (01-12).mkv", []Ep{ep(NoSeason, 1, 12)}},
		{"Show_Title [1-13] [.srt]", []Ep{ep(NoSeason, 1, 13)}},
		// in the middle of the name, as before
		{"[Grp] Show Title [01-13] [1080p]", []Ep{ep(NoSeason, 1, 13)}},
		// a range of years is not one of episodes
		{"[Grp] Show Title (2019-2020)", nil},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if v := r.Numbering.Episodes; !slices.Equal(v, test.spans) {
			t.Errorf("%q: expected episodes %v, got: %v", test.name, test.spans, v)
		}
		if s := fmt.Sprintf("%o", r); s != test.name {
			t.Errorf("%q: expected the name back, got: %q", test.name, s)
		}
	}
}

// TestAudioRange checks that the bit depth and the sample rate of a record
// are not a range of episodes, and that the same numbers are in a name with a
// resolution.
func TestAudioRange(t *testing.T) {
	tests := []struct {
		name          string
		typ           Type
		artist, title string
		spans         []Ep
	}{
		{"Mira Calder - Beneath Quiet Skies (2002) HDTracks 2011 [FLAC 24-96]", Music, "Mira Calder", "Beneath Quiet Skies", nil},
		{"Mira Calder - Beneath Quiet Skies (2002) [FLAC 24-48]", Music, "Mira Calder", "Beneath Quiet Skies", nil},
		{"Mira Calder - Beneath Quiet Skies (2002) [FLAC 16-44]", Music, "Mira Calder", "Beneath Quiet Skies", nil},
		{"Mira Calder - Beneath Quiet Skies (2002) [WEB FLAC 24-96]", Music, "Mira Calder", "Beneath Quiet Skies", nil},
		{"Mira Calder - Beneath Quiet Skies (2002) (24-192) [FLAC]", Music, "Mira Calder", "Beneath Quiet Skies", nil},
		// with no audio tag that is known
		{"Mira Calder - Beneath Quiet Skies (2002) [WAV 24-96]", Music, "Mira Calder", "Beneath Quiet Skies", nil},
		{"Mira Calder - Beneath Quiet Skies (2002) [Vinyl 24-96]", Music, "Mira Calder", "Beneath Quiet Skies", nil},
		// after the audio of a concert
		{"Mira Calder - Live in Oslo (2011) [1080p FLAC 24-96]", Movie, "", "Mira Calder - Live in Oslo", nil},
		{"Mira Calder - Live in Oslo (2011) [1080p FLAC (24-96)]", Movie, "", "Mira Calder - Live in Oslo", nil},
		{"Mira Calder - Live in Oslo (2011) [1080p FLAC(24-96)]", Movie, "", "Mira Calder - Live in Oslo", nil},
		// the episodes of a batch, in a name with a resolution
		{"[Grp] Show Title - 24-48 [1080p]", Series, "", "Show Title", []Ep{ep(NoSeason, 24, 48)}},
		{"[Grp] Show Title 24-48 [BD 1080p FLAC]", Series, "", "Show Title", []Ep{ep(NoSeason, 24, 48)}},
		{"[Grp] Show Title (16-44) [720p]", Series, "", "Show Title", []Ep{ep(NoSeason, 16, 44)}},
		// after the audio, in a bracket that follows the one the audio is in
		{"[Grp] Show Title [BD 1080p FLAC] (24-48)", Series, "", "Show Title", []Ep{ep(NoSeason, 24, 48)}},
		{"[Grp] Show Title [1080p FLAC] [24-48]", Series, "", "Show Title", []Ep{ep(NoSeason, 24, 48)}},
		// a range that is no bit depth and sample rate
		{"[Grp] Show Title - 24-50 [FLAC]", Series, "", "Show Title", []Ep{ep(NoSeason, 24, 50)}},
		{"[Grp] Show Title - 01-48 [FLAC]", Series, "", "Show Title", []Ep{ep(NoSeason, 1, 48)}},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if r.Type != test.typ || r.Artist != test.artist || r.Title != test.title {
			t.Errorf("%q: expected type %s, artist %q and title %q, got: %s, %q and %q", test.name, test.typ, test.artist, test.title, r.Type, r.Artist, r.Title)
		}
		if v := r.Numbering.Episodes; !slices.Equal(v, test.spans) {
			t.Errorf("%q: expected episodes %v, got: %v", test.name, test.spans, v)
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
		season, _ := r.Numbering.Season()
		if episode, _ := r.Numbering.Episode(); season != test.series || episode != test.episode || r.Type != test.typ {
			t.Errorf("%q: expected series %d, episode %d, type %s, got: %d, %d, %s", test.name, test.series, test.episode, test.typ, season, episode, r.Type)
		}
		if s := fmt.Sprintf("%o", r); s != test.name {
			t.Errorf("%q: expected the name back, got: %q", test.name, s)
		}
	}
}

// TestSeasonInEpisodeTitle checks that a season word written after an episode
// and a word of its title is a word of that title, and that one right after
// the episode, in a bracket, in front of the episode or too far from it is
// still a season.
func TestSeasonInEpisodeTitle(t *testing.T) {
	tests := []struct {
		name, subtitle string
		seasons        []int
	}{
		{"Show.S01E00.Meet.the.Queens.of.Season.1.1080p.WEB.h264-GRP", "Meet the Queens of Season 1", []int{1}},
		{"Show.S02E05.The.Season.1.Recap.1080p.WEB.h264-GRP", "The Season 1 Recap", []int{2}},
		{"Show.S03E00.Road.to.Season.3.720p.HDTV.x264-GRP", "Road to Season 3", []int{3}},
		{"Show.S01E01.Pilot.Series.2.1080p.WEB.h264-GRP", "Pilot Series 2", []int{1}},
		{"Show.S01E05.Episode.Title.Staffel.1.German.1080p.WEB.h264-GRP", "Episode Title Staffel 1", []int{1}},
		{"Show.S02E01.Season.2.Premiere.1080p.WEB.h264-GRP", "Premiere", []int{2}},
		{"Show S01E02 Title (Season 1) 1080p WEB h264-GRP", "Title", []int{1}},
		{"Show S01E01 + Season 2 1080p WEB h264-GRP", "", []int{1, 2}},
		{"Show S01E01 Title (Includes Season 2) 1080p WEB h264-GRP", "Title", []int{1, 2}},
		{"Show.S01E01.Title.(Includes.Season.2).1080p.WEB.h264-GRP", "Title", []int{1, 2}},
		{"Show.Season.1.S01E01.Pilot.1080p.WEB.h264-GRP", "Pilot", []int{1}},
		// a range or a list after the title is the numbering
		{"Show.S01E01.Pilot.Seasons.2-3.1080p.WEB.h264-GRP", "Pilot", []int{1, 2, 3}},
		{"Show.S01E01.Pilot.Season.2-3.1080p.WEB.h264-GRP", "Pilot", []int{1, 2, 3}},
		{"Show.S01E01.Pilot.Season.2+3.1080p.WEB.h264-GRP", "Pilot", []int{1, 2, 3}},
		// after the date of a daily episode the season stays one and is also
		// a word of the title, where a range or a list ends the title
		{"Show.2024.05.06.Title.Season.1.1080p.WEB.h264-GRP", "Title Season 1", []int{1}},
		{"Show 2024-05-06 Title Season 1 1080p WEB h264-GRP", "Title Season 1", []int{1}},
		{"Show.2024.05.06.Tournament.Season.40.720p.HDTV.x264-GRP", "Tournament Season 40", []int{40}},
		{"Show.2024.05.06.The.Season.1.Recap.720p.HDTV.x264-GRP", "The Season 1 Recap", []int{1}},
		{"Show.2024.05.06.Recap.Seasons.1-2.720p.HDTV.x264-GRP", "Recap", []int{1, 2}},
		{"Show.2024.05.06.Season.1.720p.HDTV.x264-GRP", "", []int{1}},
		{"Show.2024.05.06.Title.Season.1.Episode.5.720p.HDTV.x264-GRP", "Title", []int{1}},
		{"Show.2024.05.06.Title.Season.1.Season.2.720p.HDTV.x264-GRP", "Title", []int{1, 2}},
		{"Show.S01E01." + strings.Repeat("Word.", 20) + "Season.2.1080p.WEB.h264-GRP", strings.TrimSpace(strings.Repeat("Word ", 20)) + " Season 2", []int{1}},
		{"Show.S01E01." + strings.Repeat("Word.", 40) + "Season.2.1080p.WEB.h264-GRP", strings.TrimSpace(strings.Repeat("Word ", 40)), []int{1, 2}},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if r.Subtitle != test.subtitle {
			t.Errorf("%q: expected subtitle %q, got: %q", test.name, test.subtitle, r.Subtitle)
		}
		if v := r.Numbering.Seasons; !slices.Equal(v, test.seasons) {
			t.Errorf("%q: expected seasons %v, got: %v", test.name, test.seasons, v)
		}
		if s := fmt.Sprintf("%o", r); s != test.name {
			t.Errorf("%q: expected the name back, got: %q", test.name, s)
		}
	}
	// a season after a plus that follows the title is still one. The plus
	// stays in the subtitle ('Title +'), which this rule does not decide
	for _, name := range []string{
		"Show S01E01 Title + Season 2 1080p WEB h264-GRP",
		"Show.S01E01.Title+Season.2.1080p.WEB.h264-GRP",
	} {
		r := ParseString(name)
		if v := r.Numbering.Seasons; !slices.Equal(v, []int{1, 2}) {
			t.Errorf("%q: expected seasons [1 2], got: %v", name, v)
		}
		if s := fmt.Sprintf("%o", r); s != name {
			t.Errorf("%q: expected the name back, got: %q", name, s)
		}
	}
}

// TestEpisodes checks the episodes a name gives, in the order it writes them
// and with a range as one Ep, and the second numberings beside them.
func TestEpisodes(t *testing.T) {
	tests := []struct {
		name string
		exp  []Ep
		alt  []Ep
	}{
		// one episode
		{"Show.S01E02.1080p.WEB.h264-GRP", []Ep{ep(1, 2, 2)}, nil},
		{"[Group] Show - 04 [720p]", []Ep{ep(NoSeason, 4, 4)}, nil},
		{"[Group] Show - 1169 [720p]", []Ep{ep(NoSeason, 1169, 1169)}, nil},
		// half an episode
		{"[Group] Show - 21.5 [720p]", []Ep{{Season: NoSeason, First: 21, Last: 21, Frac: 5}}, nil},
		{"[Group] Show - 12.10 [720p]", []Ep{{Season: NoSeason, First: 12, Last: 12, Frac: 1}}, nil},
		{"[Group] Show - 12.50 [720p]", []Ep{{Season: NoSeason, First: 12, Last: 12, Frac: 5}}, nil},
		{"[Group] Show - 12.5 [720p]", []Ep{{Season: NoSeason, First: 12, Last: 12, Frac: 5}}, nil},
		// a zero in front of the fraction is not kept
		{"[Group] Show - 12.05 [720p]", []Ep{{Season: NoSeason, First: 12, Last: 12, Frac: 5}}, nil},
		{"[Group] Show - 13.5v2 [720p]", []Ep{{Season: NoSeason, First: 13, Last: 13, Frac: 5}}, nil},
		{"Show.S01E06.5.720p.WEB.x264-GRP", []Ep{{Season: 1, First: 6, Last: 6, Frac: 5}}, nil},
		{"[Group] Show Ep 5.5 [720p]", []Ep{{Season: NoSeason, First: 5, Last: 5, Frac: 5}}, nil},
		// an episode 0
		{"[Group] Show - 00 [720p]", []Ep{ep(NoSeason, 0, 0)}, nil},
		{"Show.S01E00.1080p.WEB.h264-GRP", []Ep{ep(1, 0, 0)}, nil},
		{"Show.S00E05.1080p.WEB.h264-GRP", []Ep{ep(0, 5, 5)}, nil},
		// ranges, inside a tag and between two tags
		{"Show.S01E02-04.1080p.WEB.h264-GRP", []Ep{ep(1, 2, 4)}, nil},
		{"Show.S01E02-E04.1080p.WEB.h264-GRP", []Ep{ep(1, 2, 4)}, nil},
		{"Show.S00E02-S00E04.1080p.WEB.h264-GRP", []Ep{ep(0, 2, 4)}, nil},
		{"[Group] Show - 00 ~ 12 [720p]", []Ep{ep(NoSeason, 0, 12)}, nil},
		{"[Group] Show (01-12) [720p]", []Ep{ep(NoSeason, 1, 12)}, nil},
		// a range that ends the title, in front of the episodes
		{"[Group] Show 17-26 - 01 ~ 08 [720p]", []Ep{ep(NoSeason, 1, 8)}, nil},
		// a range that starts at 1 is the episodes of a batch, not the title
		{"[Group] Show 01-12 - 13 [720p]", []Ep{ep(NoSeason, 1, 12), ep(NoSeason, 13, 13)}, nil},
		{"Show.S01E1-E99999.1080p.WEB.h264-GRP", []Ep{ep(1, 1, 99999)}, nil},
		// episodes that are not a range
		{"Show.S01E05E06.1080p.WEB.h264-GRP", []Ep{ep(1, 5, 5), ep(1, 6, 6)}, nil},
		{"Show.S01E01.S02E01.1080p.WEB.h264-GRP", []Ep{ep(1, 1, 1), ep(2, 1, 1)}, nil},
		{"Show.S01E02-S02E04.1080p.WEB.h264-GRP", []Ep{ep(1, 2, 2), ep(2, 4, 4)}, nil},
		// the season written before the episode, and a season 0 after another
		{"[Group] Show S2 - 04 [720p]", []Ep{ep(2, 4, 4)}, nil},
		{"[Group] Show 2nd Season - 08 [720p]", []Ep{ep(2, 8, 8)}, nil},
		{"[Group] Show - S04E29 (WEB 1080p) | Other Name - E88 - S00E36", []Ep{ep(4, 29, 29), ep(4, 88, 88), ep(0, 36, 36)}, nil},
		// a range goes on over a second hyphen
		{"Show.S01E02-E03-E04.1080p.WEB.h264-GRP", []Ep{ep(1, 2, 4)}, nil},
		{"Show.S01E02-E04-06.1080p.WEB.h264-GRP", []Ep{ep(1, 2, 6)}, nil},
		{"Show.S01E01-E02.E04-E05.1080p.WEB.h264-GRP", []Ep{ep(1, 1, 2), ep(1, 4, 5)}, nil},
		{"Show.Ep.05-07.1080p.WEB.h264-GRP", []Ep{ep(NoSeason, 5, 7)}, nil},
		{"[Group] Show S2 - 04 ~ 06 [720p]", []Ep{ep(2, 4, 6)}, nil},
		{"Show.S01E05-E06.5.1080p.WEB.h264-GRP", []Ep{{Season: 1, First: 5, Last: 6, Frac: 5}}, nil},
		// an end that is not higher and a hyphen with spaces between two tags
		// make no range, and the same episode twice is listed once
		{"Show.S01E02-E01.1080p.WEB.h264-GRP", []Ep{ep(1, 2, 2), ep(1, 1, 1)}, nil},
		{"Show.S01E05-E05.1080p.WEB.h264-GRP", []Ep{ep(1, 5, 5)}, nil},
		{"Show S01E02 - E04 1080p WEB h264-GRP", []Ep{ep(1, 2, 2), ep(1, 4, 4)}, nil},
		// the season carried over a tag with no episode, and none carried back
		{"Show.S01.1080p.E05.WEB.h264-GRP", []Ep{ep(1, 5, 5)}, nil},
		{"Show.E05.S01.1080p.WEB.h264-GRP", []Ep{ep(NoSeason, 5, 5)}, nil},
		// the channels are not a fraction
		{"Show.S01E06.5.1.AAC.720p.WEB-GRP", []Ep{ep(1, 6, 6)}, nil},
		// a tilde makes a range as a hyphen does, inside a tag and between two
		{"[Grp] Show S04E29~E31 1080p WEB H.264", []Ep{ep(4, 29, 31)}, nil},
		// the end of a range that a version follows, and a second numbering
		// after a hyphen, which is no range
		{"Show.S04E29-31v2.1080p.WEB.H264-GRP", []Ep{ep(4, 29, 31)}, nil},
		{"Show.S23E19-1174v2.1080p.WEB.H264-GRP", []Ep{ep(23, 19, 19)}, []Ep{ep(NoSeason, 1174, 1174)}},
		{"Show.S01E05-Episode.10.1080p.WEB.H264-GRP", []Ep{ep(1, 5, 5)}, []Ep{ep(NoSeason, 10, 10)}},
		{"Show.S01E01~03.1080p.WEB.x264-GRP", []Ep{ep(1, 1, 3)}, nil},
		// a range end of four digits after an episode of three or more
		{"[Grp] Show EP1156-1170 [1080p]", []Ep{ep(NoSeason, 1156, 1170)}, nil},
		{"Show.S01E1001-1004.1080p.WEB.x264-GRP", []Ep{ep(1, 1001, 1004)}, nil},
		{"[Grp] Show E998-1002 [1080p]", []Ep{ep(NoSeason, 998, 1002)}, nil},
		// four digits after a lower episode are a year or another number, as
		// are those a thousand episodes on
		{"Show.S01E05-2020.1080p.WEB.x264-GRP", []Ep{ep(1, 5, 5)}, nil},
		{"[Grp] Show E05-2020 [1080p]", []Ep{ep(NoSeason, 5, 5)}, nil},
		{"Show.S01E05-1080.WEB.x264-GRP", []Ep{ep(1, 5, 5)}, nil},
		{"Show.S01E100-9999.WEB.x264-GRP", []Ep{ep(1, 100, 100)}, nil},
		// half an episode with a tag or a version right after it
		{"Show.S01E06.5.WEB.x264-GRP", []Ep{{Season: 1, First: 6, Last: 6, Frac: 5}}, nil},
		{"Show.S01E02.5v2.720p.WEB.x264-GRP", []Ep{{Season: 1, First: 2, Last: 2, Frac: 5}}, nil},
		// one episode under two numberings: the second one has no season
		{"Show.S23E19.Episode.1174.1080p.WEB.h264-GRP", []Ep{ep(23, 19, 19)}, []Ep{ep(NoSeason, 1174, 1174)}},
		{"[Grp] Show - 1174 (S23E19) [1080p]", []Ep{ep(23, 19, 19)}, []Ep{ep(NoSeason, 1174, 1174)}},
		{"Noragami - S02E06 - What Must Be Done [Episode 6]", []Ep{ep(2, 6, 6)}, []Ep{ep(NoSeason, 6, 6)}},
		// a second numbering starts no range, ends none, and is a range of
		// its own when it writes one
		{"Show.S01E01.Episode.5-E07.1080p.WEB.h264-GRP", []Ep{ep(1, 1, 1), ep(1, 7, 7)}, []Ep{ep(NoSeason, 5, 5)}},
		{"Show.S01E01.Episode.5-7.1080p.WEB.h264-GRP", []Ep{ep(1, 1, 1)}, []Ep{ep(NoSeason, 5, 7)}},
		{"Show.S01E01.Episode.5.5.1080p.WEB.h264-GRP", []Ep{ep(1, 1, 1)}, []Ep{{Season: NoSeason, First: 5, Last: 5, Frac: 5}}},
		// the same episode written twice, in a path
		{"Show.S11E11-GRP/Show.S11E11-GRP.mkv", []Ep{ep(11, 11, 11)}, nil},
		// no episode
		{"Show.S01.1080p.WEB.h264-GRP", nil, nil},
		{"Show.S01-S03.1080p.WEB.h264-GRP", nil, nil},
		{"Show.2024.10.13.1080p.WEB.h264-GRP", nil, nil},
		{"Movie.2020.1080p.BluRay.x264-GRP", nil, nil},
	}
	for _, test := range tests {
		n := ParseString(test.name).Numbering
		if !slices.Equal(n.Episodes, test.exp) || !slices.Equal(n.Alt, test.alt) {
			t.Errorf("%q: expected episodes %v and second numbering %v, got: %v and %v", test.name, test.exp, test.alt, n.Episodes, n.Alt)
		}
	}
}

// TestNumberingValue checks the whole Numbering of names that between them
// have every scheme and shape, and what its methods give for it.
func TestNumberingValue(t *testing.T) {
	type answers struct {
		season, episode, absolute int
		seasoned, numbered, abs   bool
		shape                     Shape
	}
	tests := []struct {
		name string
		exp  Numbering
		ans  answers
	}{
		// a season and an episode
		{
			"Show.S01E05.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{1}, Episodes: []Ep{ep(1, 5, 5)}},
			answers{season: 1, seasoned: true, episode: 5, numbered: true, shape: ShapeEpisode},
		},
		{
			"Show.1x05.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{1}, Episodes: []Ep{ep(1, 5, 5)}},
			answers{season: 1, seasoned: true, episode: 5, numbered: true, shape: ShapeEpisode},
		},
		// a season in front of the episode
		{
			"[Grp] Show S2 - 04 [1080p].mkv",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{2}, Episodes: []Ep{ep(2, 4, 4)}},
			answers{season: 2, seasoned: true, episode: 4, numbered: true, shape: ShapeEpisode},
		},
		{
			"[Grp] Show 2nd Season - 08 [1080p].mkv",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{2}, Episodes: []Ep{ep(2, 8, 8)}},
			answers{season: 2, seasoned: true, episode: 8, numbered: true, shape: ShapeEpisode},
		},
		// season 0 and episode 0 are a season and an episode
		{
			"Show.S00E05.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{0}, Episodes: []Ep{ep(0, 5, 5)}},
			answers{seasoned: true, episode: 5, numbered: true, shape: ShapeEpisode},
		},
		{
			"Show.S01E00.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{1}, Episodes: []Ep{ep(1, 0, 0)}},
			answers{season: 1, seasoned: true, numbered: true, shape: ShapeEpisode},
		},
		{
			"[Grp] Show - 00 [1080p].mkv",
			Numbering{Scheme: SchemeAbsolute, Episodes: []Ep{ep(NoSeason, 0, 0)}},
			answers{numbered: true, abs: true, shape: ShapeEpisode},
		},
		// packs
		{
			"Show.S01.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{1}},
			answers{season: 1, seasoned: true, shape: ShapeSeason},
		},
		{
			"Show.S00.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{0}},
			answers{seasoned: true, shape: ShapeSeason},
		},
		{
			"Show.S01-S03.1080p.BluRay.x264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{1, 2, 3}},
			answers{season: 1, seasoned: true, shape: ShapeSeasons},
		},
		// the seasons are in the order written, and Season gives the first
		{
			"[Grp] Show [Season 2 + Season 1 OVA] [BD 1080p]",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{2, 1}},
			answers{season: 2, seasoned: true, shape: ShapeSeasons},
		},
		// the first season written is not the season of the episode
		{
			"Show.S01-S03E05.1080p.BluRay.x264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{1, 3}, Episodes: []Ep{ep(3, 5, 5)}},
			answers{season: 1, seasoned: true, episode: 5, numbered: true, shape: ShapeEpisode},
		},
		// several episodes
		{
			"Show.S01E05E06.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{1}, Episodes: []Ep{ep(1, 5, 5), ep(1, 6, 6)}},
			answers{season: 1, seasoned: true, episode: 5, numbered: true, shape: ShapeEpisodes},
		},
		{
			"Show.S01E01-E03.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{1}, Episodes: []Ep{ep(1, 1, 3)}},
			answers{season: 1, seasoned: true, episode: 1, numbered: true, shape: ShapeEpisodes},
		},
		// the first episode of a batch is its episode, as that of any range
		{
			"[Grp] Show - 01-12 [1080p]",
			Numbering{Scheme: SchemeAbsolute, Episodes: []Ep{ep(NoSeason, 1, 12)}},
			answers{episode: 1, numbered: true, shape: ShapeEpisodes},
		},
		{
			"[Grp] Show S2 - 01-12 [1080p]",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{2}, Episodes: []Ep{ep(2, 1, 12)}},
			answers{season: 2, seasoned: true, episode: 1, numbered: true, shape: ShapeEpisodes},
		},
		// episodes with no season anywhere are absolute
		{
			"[Grp] Show - 1169 [1080p].mkv",
			Numbering{Scheme: SchemeAbsolute, Episodes: []Ep{ep(NoSeason, 1169, 1169)}},
			answers{episode: 1169, numbered: true, absolute: 1169, abs: true, shape: ShapeEpisode},
		},
		{
			"Show.E1100.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeAbsolute, Episodes: []Ep{ep(NoSeason, 1100, 1100)}},
			answers{episode: 1100, numbered: true, absolute: 1100, abs: true, shape: ShapeEpisode},
		},
		// half an episode
		{
			"[Grp] Show - 21.5 [1080p].mkv",
			Numbering{Scheme: SchemeAbsolute, Episodes: []Ep{{Season: NoSeason, First: 21, Last: 21, Frac: 5}}},
			answers{episode: 21, numbered: true, shape: ShapeEpisode},
		},
		{
			"Show.S01E05-E06.5.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{1}, Episodes: []Ep{{Season: 1, First: 5, Last: 6, Frac: 5}}},
			answers{season: 1, seasoned: true, episode: 5, numbered: true, shape: ShapeEpisodes},
		},
		// a second numbering
		{
			"Show.S23E19.Episode.1174.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{23}, Episodes: []Ep{ep(23, 19, 19)}, Alt: []Ep{ep(NoSeason, 1174, 1174)}},
			answers{season: 23, seasoned: true, episode: 19, numbered: true, absolute: 1174, abs: true, shape: ShapeEpisode},
		},
		{
			"Noragami - S02E06 - What Must Be Done [Episode 6]",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{2}, Episodes: []Ep{ep(2, 6, 6)}, Alt: []Ep{ep(NoSeason, 6, 6)}},
			answers{season: 2, seasoned: true, episode: 6, numbered: true, shape: ShapeEpisode},
		},
		{
			"Show.S00E05.Episode.12.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{0}, Episodes: []Ep{ep(0, 5, 5)}, Alt: []Ep{ep(NoSeason, 12, 12)}},
			answers{seasoned: true, episode: 5, numbered: true, shape: ShapeEpisode},
		},
		// the same episode twice
		{
			"Show.S11E11-GRP/Show.S11E11-GRP.mkv",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{11}, Episodes: []Ep{ep(11, 11, 11)}},
			answers{season: 11, seasoned: true, episode: 11, numbered: true, shape: ShapeEpisode},
		},
		// a date where an episode would stand
		{
			"Show.2024.03.11.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeDaily},
			answers{shape: ShapeDaily},
		},
		{
			"Show.2024.05.06.Title.Season.1.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeDaily, Seasons: []int{1}},
			answers{season: 1, seasoned: true, shape: ShapeDaily},
		},
		// a date beside an episode
		{
			"Show.S01E05.2024.03.11.1080p.WEB.h264-GRP",
			Numbering{Scheme: SchemeSeasoned, Seasons: []int{1}, Episodes: []Ep{ep(1, 5, 5)}},
			answers{season: 1, seasoned: true, episode: 5, numbered: true, shape: ShapeEpisode},
		},
		// no numbering
		{"Movie.2020.1080p.BluRay.x264-GRP", Numbering{}, answers{}},
		{"Artist_Name-Album_Title-2CD-FLAC-2020-GRP", Numbering{}, answers{}},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		n := r.Numbering
		if !reflect.DeepEqual(n, test.exp) {
			t.Errorf("%q: expected %+v, got: %+v", test.name, test.exp, n)
		}
		var got answers
		got.season, got.seasoned = n.Season()
		got.episode, got.numbered = n.Episode()
		got.absolute, got.abs = n.Absolute()
		if got.shape = n.Shape(); got != test.ans {
			t.Errorf("%q: expected %+v, got: %+v", test.name, test.ans, got)
		}
		checkNumbering(t, test.name, r)
	}
	// a dated name is a daily episode by its type: the type a caller gives
	// says whether the date stands for an episode
	const dated = "Show.2024.03.11.1080p.WEB.h264-GRP"
	for typ, exp := range map[Type]Scheme{Unknown: SchemeDaily, Episode: SchemeDaily, Movie: SchemeNone, Series: SchemeNone} {
		if n := ParseStringHint(dated, Hint{Type: typ}).Numbering; n.Scheme != exp {
			t.Errorf("%q as %q: expected scheme %q, got: %q", dated, typ, exp, n.Scheme)
		}
	}
}

// TestNumberingNames checks the names of the schemes, the shapes and the
// kinds, which a caller prints and logs.
func TestNumberingNames(t *testing.T) {
	for _, test := range []struct {
		v   fmt.Stringer
		exp string
	}{
		{SchemeNone, ""}, {SchemeSeasoned, "seasoned"}, {SchemeAbsolute, "absolute"}, {SchemeDaily, "daily"}, {Scheme(99), ""},
		{ShapeNone, ""}, {ShapeEpisode, "episode"}, {ShapeEpisodes, "episodes"}, {ShapeSeason, "season"}, {ShapeSeasons, "seasons"}, {ShapeDaily, "daily"}, {Shape(99), ""},
		{EpRegular, ""}, {EpSpecial, "special"}, {EpOVA, "ova"}, {EpOP, "op"}, {EpED, "ed"}, {EpPV, "pv"}, {EpKind(99), ""},
	} {
		if s := test.v.String(); s != test.exp {
			t.Errorf("%T(%d): expected %q, got: %q", test.v, test.v, test.exp, s)
		}
	}
}

// TestNumberingAll checks the order All yields in, that it stops when asked
// to, and that it yields the two ends of a range it has no room to fill.
func TestNumberingAll(t *testing.T) {
	n := ParseString("[Grp] Show - S04E29-E31 (WEB 1080p) | Other Name - E88 - S00E36").Numbering
	var got [][]int
	for season, episode := range n.All() {
		got = append(got, []int{season, episode})
	}
	if exp := [][]int{{4, 29}, {4, 30}, {4, 31}, {4, 88}, {0, 36}}; !slices.EqualFunc(got, exp, slices.Equal[[]int]) {
		t.Errorf("expected %v in the order of the name, got: %v", exp, got)
	}
	got = nil
	for season, episode := range n.All() {
		if got = append(got, []int{season, episode}); len(got) == 2 {
			break
		}
	}
	if len(got) != 2 {
		t.Errorf("expected All to stop after two, got: %v", got)
	}
	// the room is shared by the ranges of a numbering, in their order
	wide := Numbering{Episodes: []Ep{ep(1, 1, 9000), ep(1, 9001, 12000), ep(2, 1, 3), ep(NoSeason, 7, 7)}}
	count := map[int]int{}
	for season := range wide.All() {
		count[season]++
	}
	if exp := map[int]int{1: 9000 + 2, 2: 3, NoSeason: 1}; !reflect.DeepEqual(count, exp) {
		t.Errorf("expected the episodes per season %v, got: %v", exp, count)
	}
	// an extra yields nothing
	got = nil
	for season, episode := range (Numbering{Episodes: []Ep{{Season: 1, First: 1, Last: 9, Kind: EpOVA}, ep(1, 5, 6), {Season: 1, First: 2, Last: 2, Kind: EpSpecial}}}).All() {
		got = append(got, []int{season, episode})
	}
	if exp := [][]int{{1, 5}, {1, 6}}; !slices.EqualFunc(got, exp, slices.Equal[[]int]) {
		t.Errorf("expected %v, got: %v", exp, got)
	}
}

// TestNumberingAllAnyValue checks that All ends on a value no parse made, as
// one decoded from a stored release may be: it yields no more than the two
// ends of each Ep and maxRangeFill episodes beside them, whatever the numbers.
// An Ep whose Last is not above its First yields its First, and a number
// below 0 is yielded as it is.
func TestNumberingAllAnyValue(t *testing.T) {
	const lo, hi = math.MinInt, math.MaxInt
	for _, test := range []struct {
		name string
		eps  []Ep
		// the pairs when they are few, and else how many there are
		exp   [][]int
		count int
	}{
		{"last below first", []Ep{ep(1, 5, 3)}, [][]int{{1, 5}}, 0},
		{"below zero", []Ep{ep(-7, -3, -1)}, [][]int{{-7, -3}, {-7, -2}, {-7, -1}}, 0},
		{"from below zero to the highest", []Ep{ep(1, -2, hi)}, [][]int{{1, -2}, {1, hi}}, 0},
		{"from the lowest to the highest", []Ep{ep(1, lo, hi)}, [][]int{{1, lo}, {1, hi}}, 0},
		{"the two highest", []Ep{ep(1, hi-1, hi)}, [][]int{{1, hi - 1}, {1, hi}}, 0},
		{"the highest three", []Ep{ep(1, hi-2, hi)}, [][]int{{1, hi - 2}, {1, hi - 1}, {1, hi}}, 0},
		{"the lowest three", []Ep{ep(1, lo, lo+2)}, [][]int{{1, lo}, {1, lo + 1}, {1, lo + 2}}, 0},
		{"the highest alone", []Ep{ep(NoSeason, hi, hi)}, [][]int{{NoSeason, hi}}, 0},
		{"the highest down to the lowest", []Ep{ep(1, hi, lo)}, [][]int{{1, hi}}, 0},
		// a range that fits is filled, and the one after it has what is left
		{"as wide as the room", []Ep{ep(1, 0, maxRangeFill+1)}, nil, maxRangeFill + 2},
		{"one wider than the room", []Ep{ep(1, 0, maxRangeFill+2)}, [][]int{{1, 0}, {1, maxRangeFill + 2}}, 0},
		{"the room used up", []Ep{ep(1, -5000, 5001), ep(2, lo, hi), ep(3, 1, 3), ep(3, 7, 8)}, nil, maxRangeFill + 2 + 2 + 2 + 2},
		// the first range of 9,000 is filled, and the others are their ends
		{"many wide ranges", slices.Repeat([]Ep{ep(1, -2, hi), ep(2, lo, 0), ep(3, 1, 9000)}, 100), nil, 100*6 + 8998},
	} {
		n := Numbering{Episodes: test.eps}
		var got [][]int
		var count int
		for season, episode := range n.All() {
			if count++; count > 4*maxRangeFill {
				break
			}
			if test.exp != nil {
				got = append(got, []int{season, episode})
			}
		}
		switch {
		case test.exp != nil && !slices.EqualFunc(got, test.exp, slices.Equal[[]int]):
			t.Errorf("%s: expected %v, got: %v", test.name, test.exp, got)
		case test.exp == nil && count != test.count:
			t.Errorf("%s: expected %d pairs, got: %d", test.name, test.count, count)
		}
		// the other methods read such a value too
		_, _ = n.Season()
		_, _ = n.Episode()
		_, _ = n.Absolute()
		_ = n.Shape()
	}
}

// checkNumbering checks what holds for the Numbering of every parsed release,
// whatever the name: the fields agree with each other and with the series
// tags, and each method gives what the fields say.
func checkNumbering(tb testing.TB, name string, r Release) {
	tb.Helper()
	n := r.Numbering
	fail := func(format string, args ...any) {
		tb.Helper()
		tb.Errorf("%.200q: %s\n  %+v", name, fmt.Sprintf(format, args...), n)
	}
	// the fields
	for _, list := range [][]Ep{n.Episodes, n.Alt} {
		for i, e := range list {
			switch {
			case e.First < 0 || e.First > e.Last:
				fail("expected an episode from First up to Last, got: %+v", e)
			case e.Season != NoSeason && e.Season < 0:
				fail("expected a season or NoSeason, got: %+v", e)
			case e.Kind != EpRegular || e.Frac > 99 || e.Frac != 0 && e.Frac%10 == 0:
				fail("expected a regular episode with a fraction of two digits at most and no zero at its end, got: %+v", e)
			case e.Season != NoSeason && !slices.Contains(n.Seasons, e.Season):
				fail("expected the season of %+v in Seasons", e)
			case slices.Contains(list[:i], e):
				fail("expected %+v once", e)
			}
		}
		if list != nil && (len(list) == 0 || cap(list) != len(list)) {
			fail("expected nil for no episodes and no room to spare, got: %d of %d", len(list), cap(list))
		}
	}
	for i, season := range n.Seasons {
		if season < 0 || slices.Contains(n.Seasons[:i], season) {
			fail("expected each season once, got: %v", n.Seasons)
		}
	}
	if n.Seasons != nil && (len(n.Seasons) == 0 || cap(n.Seasons) != len(n.Seasons)) {
		fail("expected nil for no seasons and no room to spare, got: %d of %d", len(n.Seasons), cap(n.Seasons))
	}
	if slices.ContainsFunc(n.Alt, func(e Ep) bool { return e.Season != NoSeason }) || n.Alt != nil && (n.Episodes == nil || n.Seasons == nil) {
		fail("expected a second numbering with no season, beside a season and an episode")
	}
	if n.Volume != 0 || n.Contents != nil || n.Batch {
		fail("expected no volume, contents or batch")
	}
	daily := n.Episodes == nil && r.Type == Episode && r.Year != 0 && r.Month != 0 && r.Day != 0
	scheme := SchemeNone
	switch {
	case daily:
		scheme = SchemeDaily
	case n.Seasons != nil:
		scheme = SchemeSeasoned
	case n.Episodes != nil:
		scheme = SchemeAbsolute
	}
	if n.Scheme != scheme {
		fail("expected scheme %q, got: %q", scheme, n.Scheme)
	}
	// the tags
	var seasons, episodes bool
	for _, tag := range r.tags {
		if !tag.Is(TagTypeSeries) {
			continue
		}
		if tag.val(1) != "" {
			if seasons = true; !slices.Contains(n.Seasons, atoi(tag.val(1))) {
				fail("expected the season of the tag %q in Seasons", tag.val(0))
			}
		}
		episodes = episodes || tag.wroteEpisode()
	}
	if seasons != (n.Seasons != nil) || episodes != (n.Episodes != nil) {
		fail("expected seasons %t and episodes %t by the tags", seasons, episodes)
	}
	if again, _ := r.numbering(); !reflect.DeepEqual(again, Numbering{Scheme: again.Scheme, Seasons: n.Seasons, Episodes: n.Episodes, Alt: n.Alt}) {
		fail("expected the tags to give the same numbering again, got: %+v", again)
	}
	// the methods
	season, seasoned := n.Season()
	if seasoned != (n.Seasons != nil) || seasoned && season != n.Seasons[0] || !seasoned && season != 0 {
		fail("expected Season to give the first of Seasons, got: %d %t", season, seasoned)
	}
	episode, numbered := n.Episode()
	if numbered != (n.Episodes != nil) || numbered && episode != n.Episodes[0].First || !numbered && episode != 0 {
		fail("expected Episode to give the First of the first episode, got: %d %t", episode, numbered)
	}
	shape := ShapeNone
	switch {
	case len(n.Episodes) == 1 && n.Episodes[0].First == n.Episodes[0].Last:
		shape = ShapeEpisode
	case n.Episodes != nil:
		shape = ShapeEpisodes
	case daily:
		shape = ShapeDaily
	case len(n.Seasons) == 1:
		shape = ShapeSeason
	case n.Seasons != nil:
		shape = ShapeSeasons
	}
	if n.Shape() != shape {
		fail("expected shape %q, got: %q", shape, n.Shape())
	}
	// All yields the episodes of each Ep in turn: its First, then all the
	// episodes inside a range or none of them, and the Last of a range
	var all [][2]int
	for season, episode := range n.All() {
		all = append(all, [2]int{season, episode})
	}
	var k, filled int
	for _, e := range n.Episodes {
		if k == len(all) || all[k] != [2]int{e.Season, e.First} {
			fail("expected All to yield the First of %+v as pair %d, got: %v", e, k, all[min(k, len(all)):min(k+1, len(all))])
			break
		}
		if k++; e.First == e.Last {
			continue
		}
		if inside := e.Last - e.First - 1; inside > 0 && k < len(all) && all[k] == [2]int{e.Season, e.First + 1} {
			for episode := e.First + 1; episode < e.Last && k < len(all) && all[k] == [2]int{e.Season, episode}; episode++ {
				k, filled = k+1, filled+1
			}
		}
		if k == len(all) || all[k] != [2]int{e.Season, e.Last} {
			fail("expected All to yield the Last of %+v as pair %d, got: %v", e, k, all[min(k, len(all)):min(k+1, len(all))])
			break
		}
		k++
	}
	if k != len(all) || filled > maxRangeFill {
		fail("expected All to yield the episodes and no more, and to fill at most %d, got: %d pairs, %d read and %d filled", maxRangeFill, len(all), k, filled)
	}
	// an absolute episode is one whole episode: the one episode of a name
	// with no season anywhere, which Episode gives too, and else a second
	// numbering that is not the season's episode again
	absolute, ok := n.Absolute()
	one := func(e Ep) bool { return e.First == absolute && e.Last == absolute && e.Frac == 0 }
	switch {
	case !ok && absolute != 0:
		fail("expected no number with no absolute episode, got: %d", absolute)
	case !ok:
	case !seasoned && (len(n.Episodes) != 1 || !one(n.Episodes[0]) || absolute != episode):
		fail("expected the absolute episode %d to be the one episode", absolute)
	case seasoned && (season == 0 || absolute == episode || !slices.ContainsFunc(n.Alt, one)):
		fail("expected the absolute episode %d to be a second numbering, and not of the specials or the episode again", absolute)
	}
}

// TestNumberingInvariants checks checkNumbering on every name there is: those
// of tests.yaml, of the labelled fixtures and of the corpora that are present.
func TestNumberingInvariants(t *testing.T) {
	names := slices.Concat(benchCorpus(t), scratchNames())
	for _, file := range []string{"anitomy.json", "anitogo.json", "nyaa-400.json", "nyaa-blind-140.json", "probes-title.json", "probes-title-end.json"} {
		for _, c := range loadLabelled(t, file) {
			names = append(names, c.FileName)
		}
	}
	if !testing.Short() {
		for _, file := range []string{"nyaa.txt", "sports.txt"} {
			v, _ := readCorpus(file)
			names = append(names, v...)
		}
	}
	shapes := map[Shape]int{}
	for _, name := range names {
		r := ParseString(name)
		checkNumbering(t, name, r)
		shapes[r.Numbering.Shape()]++
	}
	t.Logf("%d names: %v", len(names), shapes)
}

// TestNumberingLinear checks that the time to parse a name grows with its
// length and not with the square of it, for names that are all numbering. A
// name is untrusted input, and the numbering is read on every parse. Each
// shape is parsed at two lengths, one eight times the other, and the longer
// may take 24 times as long: a walk over the name for every series tag takes
// 64.
func TestNumberingLinear(t *testing.T) {
	clock := func(s string) time.Duration {
		best := time.Duration(1<<63 - 1)
		for range 5 {
			start := time.Now()
			ParseString(s)
			best = min(best, time.Since(start))
		}
		return best
	}
	for _, unit := range []string{
		// a range that is a word of the title, and the episodes
		"Title 17-26 - 01 ~ 08 ",
		// seasons with their episodes, the same and different ones
		"S01E01.",
		"S01E01.S02E02.",
		// ranges between two tags and inside one
		"E01-E02.",
		"01 ~ 12 ",
		// a list of seasons
		"Season 1+2+",
		// episodes with no season, and second numberings
		"[Grp] Show - 05 ",
		"- 05 ",
		"- 05v2 ",
		"Episode 5 ",
		"S23E19.Episode.1174.",
		"S01E01.S02E02.Episode.3.",
		// a season in front of the episode, and seasons with a range beside
		"S2 - 04 ",
		"2nd Season - 08 ",
		"Season 1-4 (1-76) ",
	} {
		short := clock(strings.Repeat(unit, 1000))
		long := clock(strings.Repeat(unit, 8000))
		if long > 24*short {
			t.Errorf("8000 of %q took %s and 1000 took %s, more than 24 times as long", unit, long, short)
		}
	}
}

// TestNumberingParseAllocs keeps what the numbering adds to a parse to what a
// release holds on to: nothing for a name with no numbering, and one
// allocation for its seasons, its episodes and its second numbering each.
func TestNumberingParseAllocs(t *testing.T) {
	for _, test := range []struct {
		name string
		max  float64
	}{
		{"Movie.Title.2019.1080p.WEB-DL.DDP5.1.H.264-GRP", 0},
		{"Show.Title.2024.03.11.1080p.WEB-DL.H.264-GRP", 0},
		{"Show.Title.S01.1080p.WEB-DL.H.264-GRP", 1},
		{"[SubsPlease] Show Title - 05 (1080p) [ABCD1234].mkv", 1},
		{"[SubsPlease] Show Title (01-12) (1080p) [Batch]", 1},
		{"Show.Title.S01E02.The.Episode.1080p.WEB-DL.DDP5.1.H.264-GRP", 2},
		{"Show.Title.S01E02-E04.1080p.WEB-DL.H.264-GRP", 2},
		{"Show.Title.S23E19.Episode.1174.1080p.WEB-DL.H.264-GRP", 3},
	} {
		r := ParseString(test.name)
		if n := testing.AllocsPerRun(100, func() { _, _ = r.numbering() }); n > test.max {
			t.Errorf("%q: expected at most %v allocations for the numbering, got: %v", test.name, test.max, n)
		}
	}
}

// TestEventNumbering checks what Event reads of the numbering of a name that
// starts with a competition: an episode with no season makes it a show about
// the sport, and the range of a batch does not.
func TestEventNumbering(t *testing.T) {
	for _, test := range []struct {
		name string
		exp  bool
	}{
		{"UFC.300.Prelims.1080p.WEB.h264-GRP", true},
		{"WWE.Unreal.E02.1080p.WEB.h264-GRP", false},
		{"UFC.Countdown.E05.720p.HDTV.x264-GRP", false},
		{"UFC.2024.E01-E03.Qualifying.1080p.WEB.h264-GRP", false},
		{"UFC.2024.(01-05).Qualifying.1080p.WEB.h264-GRP", true},
	} {
		if ev, ok := ParseString(test.name).Event(); ok != test.exp {
			t.Errorf("%q: expected an event %t, got: %+v %t", test.name, test.exp, ev, ok)
		}
	}
	// a release made by hand has the episode it was given
	r := Release{Type: Episode, Title: "UFC 300", Subtitle: "Prelims"}
	if _, ok := r.Event(); !ok {
		t.Errorf("expected an event with no numbering")
	}
	r.Numbering = Numbering{Scheme: SchemeAbsolute, Episodes: []Ep{ep(NoSeason, 5, 5)}}
	if ev, ok := r.Event(); ok {
		t.Errorf("expected no event with an episode and no season, got: %+v", ev)
	}
	// the range of a batch is no episode, and neither is an extra
	for _, n := range []Numbering{
		{Scheme: SchemeAbsolute, Episodes: []Ep{ep(NoSeason, 1, 3)}},
		{Episodes: []Ep{{Season: NoSeason, First: 2, Last: 2, Kind: EpOVA}}},
		{Episodes: []Ep{ep(NoSeason, 0, 0)}},
	} {
		if r.Numbering = n; !func() bool { _, ok := r.Event(); return ok }() {
			t.Errorf("expected an event with %+v", n)
		}
	}
	r.Numbering = Numbering{Scheme: SchemeSeasoned, Seasons: []int{1}, Episodes: []Ep{ep(1, 5, 5)}}
	if ev, ok := r.Event(); ok {
		t.Errorf("expected no event with a season and an episode, got: %+v", ev)
	}
}
