package rls

import (
	"bufio"
	"bytes"
	"fmt"
	"maps"
	"math/rand"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/autobrr/rls/taginfo"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

func TestParseRelease(t *testing.T) {
	p := NewTagParser(taginfo.All(groupInfos()), DefaultLexers()...)
	m := make(map[string]bool)
	for n, tt := range rlsTests(t) {
		i, test := n, tt
		name := fmt.Sprintf("%s/%d", test.exp.Type, i)
		if strings.HasPrefix(name, "/") {
			name = "unknown" + name
		}
		t.Run(name, func(t *testing.T) {
			t.Logf("%q", test.s)
			if _, ok := m[test.s]; ok {
				t.Fatalf("test %d %q is a duplicate!", i, test.s)
			}
			m[test.s] = true
			r := p.ParseRelease([]byte(test.s))
			if test.s != "" && r.Tags() == nil {
				t.Fatalf("test %d %q expected tags, got nil", i, test.s)
			}
			if s, exp := fmt.Sprintf("%o", r), test.s; s != exp {
				t.Errorf("test %d %q expected:\n  %q\ngot:\n  %q", i, test.s, exp, s)
			}
			var count int
			for _, tag := range r.Tags() {
				if tag.Is(TagTypeDate) {
					count++
				}
			}
			if count > 1 && r.Type != Magazine {
				t.Fatalf("test %d %q has TagTypeDate count > 1: %d", i, test.s, count)
			}
			v := buildRls(r)
			if !cmp.Equal(v, test.exp) {
				t.Errorf("test %d %q expected to be same, got:\n%s", i, test.s, cmp.Diff(test.exp, v))
			}
			// t.Logf("test %d\n  %q\n  %q", i, test.s, r.Title)
		})
	}
}

// TestSeriesEpisodesRangeFill checks that the episodes inside a range are
// filled in, and that ranges too wide to fill are given by their ends.
func TestSeriesEpisodesRangeFill(t *testing.T) {
	for _, test := range []struct {
		s   string
		exp int
	}{
		{"Show.S01E02-04.1080p.WEB-GRP", 3},
		{"[Group] One Piece (E001-E1100) (480p)", 1100},
		{"Show.E1-E99999.1080p.WEB-GRP", 2},
		{"Show.S01.E1-E99999.1080p.WEB-GRP", 2},
	} {
		if n := len(ParseString(test.s).SeriesEpisodes()); n != test.exp {
			t.Errorf("%q expected %d episodes, got: %d", test.s, test.exp, n)
		}
	}
	// every range adds its two ends, and together they fill no more than
	// maxRangeFill episodes
	for _, s := range []string{"E1-999.", "E1-E9999.", "S01E1-999."} {
		const count = 500
		if n := len(ParseString(strings.Repeat(s, count)).SeriesEpisodes()); n > maxRangeFill+2*count {
			t.Errorf("%q repeated expected at most %d episodes, got: %d", s, maxRangeFill+2*count, n)
		}
	}
}

// TestDateColon checks that a colon before two digits does not make a month.
func TestDateColon(t *testing.T) {
	for _, s := range []string{
		"Show - :.05 [1080p]",
		"Show.S01E05: 12.Rounds.1080p.WEB.h264-GRP",
		"[EMBER] 86 - Eighty Six (2021) (Season 1 (EP: 12-23) + Specials) [BDRip] [1080p Dual Audio HEVC 10 bits DDP]",
	} {
		if r := ParseString(s); r.Month != 0 {
			t.Errorf("%q expected no month, got: %d", s, r.Month)
		}
	}
}

// TestOpeningResolution checks that a resolution alone in the bracket that
// opens a name is the resolution and not the group, and that a name which is
// a row of brackets keeps its title.
func TestOpeningResolution(t *testing.T) {
	tests := []struct {
		name, title, resolution, group string
	}{
		{"[1080p] Show - 01", "Show", "1080p", ""},
		{"[1280x720] Show - 01", "Show", "720p", ""},
		// a tag as a part of the bracket
		{"[Anime-HEVC] Show - 01", "Show", "", "Anime-HEVC"},
		// a bracket after it
		{"[1080p] [Group] Show - 01", "Show", "", "1080p"},
		{"[1080p][Group] Show - 01 [ABCD1234]", "Show", "", "1080p"},
		{"[1080p] [Complete Manual] - 04", "Complete Manual", "", "1080p"},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		if r.Title != test.title || r.Resolution != test.resolution || r.Group != test.group {
			t.Errorf("%q: expected title %q, resolution %q, group %q, got: %q, %q, %q", test.name, test.title, test.resolution, test.group, r.Title, r.Resolution, r.Group)
		}
	}
}

// TestSpecialEpisode checks that an 'SP' in front of an episode with no season
// marks a special, in a name with no video tag as in one with them, and that
// it stays the format of a record elsewhere.
func TestSpecialEpisode(t *testing.T) {
	for _, test := range []struct {
		name, title, subtitle, source string
		typ                           Type
	}{
		{"[Grp] Show SP Episode 00 - Prologue", "Show SP", "Prologue", "", Episode},
		{"[Grp] Show SP/OVA Episode 00 - Prologue", "Show SP", "Prologue", "", Episode},
		{"Show SP Episode 00 Prologue", "Show SP", "Prologue", "", Episode},
		{"[Grp] Show SP Episode 00 Prologue (Web 1080p AAC).mkv", "Show SP", "Prologue", "WEB", Episode},
		// in a bracket, between hyphens, with no episode after it, or in a
		// name with an audio tag or a year
		{"Artist - Album (SP) Episode 1 FLAC", "Album", "", "SP", Music},
		{"Artist - Album SP - 01", "Album", "", "SP", Music},
		{"Artist-Album-SP-FLAC-2020-GRP", "Album", "", "SP", Music},
		{"Artist-Album_SP_Episode_1-WEB-FLAC-2020-GRP", "Album", "", "SP", Music},
		{"Artist - Album SP Episode 1 FLAC", "Album", "", "SP", Music},
		{"Artist - Single SP E01 MP3 320", "Single", "", "SP", Music},
		{"Artist - Mixtape SP Ep 5 (2019)", "Mixtape", "", "SP", Music},
		// or after a hyphen, which sets an artist apart from an album
		{"Artist - Album SP Episode 1", "Album", "", "SP", Music},
		{"Artist-Album_SP_Episode_1-WEB-GRP", "Album", "", "SP", Music},
		{"Artist - Album SP Episode 1 (Remastered)", "Album", "", "SP", Music},
	} {
		r := ParseString(test.name)
		if r.Type != test.typ || r.Title != test.title || r.Subtitle != test.subtitle || r.Source != test.source {
			t.Errorf("%q: expected %s, title %q, subtitle %q, source %q, got: %s, %q, %q, %q", test.name, test.typ, test.title, test.subtitle, test.source, r.Type, r.Title, r.Subtitle, r.Source)
		}
	}
}

// TestListValuesOnce checks that a value the name writes twice is in its
// list field once, at the place it was first written.
func TestListValuesOnce(t *testing.T) {
	r := ParseString("[Group] Show - 01 [Dual-Audio] [1080p HEVC x265] [AAC] [Dual-Audio] [HEVC] [Eng] [English]")
	if exp := []string{"DUAL.AUDIO", "AAC"}; !slices.Equal(r.Audio, exp) {
		t.Errorf("expected audio %q, got: %q", exp, r.Audio)
	}
	if exp := []string{"HEVC", "x265"}; !slices.Equal(r.Codec, exp) {
		t.Errorf("expected codec %q, got: %q", exp, r.Codec)
	}
	if exp := []string{"ENGLiSH"}; !slices.Equal(r.Language, exp) {
		t.Errorf("expected language %q, got: %q", exp, r.Language)
	}
	r = ParseString("Movie.2020.PROPER.REPACK.Limited.Edition.1080p.BluRay.HDR.HDR.Extended.Extended.REPACK.LIMITED.EDITION.x264-GRP")
	if exp := []string{"Limited.Edition"}; !slices.Equal(r.Edition, exp) {
		t.Errorf("expected edition %q, got: %q", exp, r.Edition)
	}
	if exp := []string{"PROPER", "REPACK"}; !slices.Equal(r.Other, exp) {
		t.Errorf("expected other %q, got: %q", exp, r.Other)
	}
	if exp := []string{"HDR"}; !slices.Equal(r.HDR, exp) {
		t.Errorf("expected hdr %q, got: %q", exp, r.HDR)
	}
	if exp := []string{"Extended.Cut"}; !slices.Equal(r.Cut, exp) {
		t.Errorf("expected cut %q, got: %q", exp, r.Cut)
	}
}

// TestSubtitleNotUnused checks that the words of the subtitle are not also in
// the unused text, and that the words a later subtitle replaced are.
func TestSubtitleNotUnused(t *testing.T) {
	tests := []struct {
		name, subtitle string
		unused         []string
	}{
		{"Show.2024.05.06.[The.Title].720p.HDTV.x264-GRP", "The Title", nil},
		{"Show.2024.05.06.(The.Title).720p.HDTV.x264-GRP", "The Title", nil},
		{"Show.2024.05.06.The.Title.720p.HDTV.x264-GRP", "The Title", nil},
		{"Show S01E08 (2022 Airing) 1080i HDTV AAC 2.0-ZR", "Airing", nil},
		// the episode's own title replaces the one read after the year
		{"Show.2024.The.Title.720p.S01E05.Name.WEB.x264-GRP", "Name", []string{"The", "Title"}},
		{"Show.2024.[The.Title].720p.S01E05.Name.WEB-GRP", "Name", []string{"The", "Title"}},
	}
	for _, test := range tests {
		r := ParseString(test.name)
		var unused []string
		for _, tag := range r.Unused() {
			unused = append(unused, tag.Text())
		}
		if r.Subtitle != test.subtitle || !slices.Equal(unused, test.unused) {
			t.Errorf("%q: expected subtitle %q and unused %q, got: %q and %q", test.name, test.subtitle, test.unused, r.Subtitle, unused)
		}
	}
}

func TestCollapser(t *testing.T) {
	tests := []struct {
		s string
		c string
		n string
	}{
		{"''\t\tAmélie\r\r1998\n\nMKV\f\f''", "Amelie 1998 MKV", "amelie 1998 mkv"},
		{"\t Star Trek  -  Lower  Decks \t", "Star Trek - Lower Decks", "star trek lower decks"},
		{"Star Trek-Lower DECKS", "Star Trek-Lower DECKS", "star trek-lower decks"},
		{"   t-pain  rappa   ", "t-pain rappa", "t-pain rappa"},
		{"\t\f\f[PMEDIA] \u2b50\ufe0f\u0009\t", "[PMEDIA] \u2b50", "pmedia \u2b50"}, // ⭐️ vs ⭐
		{"$elfie.Shootout", "$elfie.Shootout", "selfie shootout"},
		{"LAWLE$$", "LAWLE$$", "lawless"},
		{"$100 TAXI RIDE", "$100 TAXI RIDE", "100 taxi ride"},
		{"Mr. & Mrs. Smith", "Mr. & Mrs. Smith", "mr & mrs smith"},
		{"Bob's Burgers", "Bobs Burgers", "bobs burgers"},
		// a byte that is not valid utf-8 becomes U+FFFD, as strings.ToValidUTF8
		// would make it, and the text after it is kept
		{"Am\xe9lie", "Am\ufffdlie", "am\ufffdlie"},
		{"Am\xe9rica Latina", "Am\ufffdrica Latina", "am\ufffdrica latina"},
		{"a\xff\xfeb", "a\ufffdb", "a\ufffdb"},
		{" \xffTitle  Two ", "\ufffdTitle Two", "\ufffdtitle two"},
		{"Caf\xc3", "Caf\ufffd", "caf\ufffd"},
		{"Title \ufffd Two", "Title \ufffd Two", "title \ufffd two"},
		{"a\ufffd\xffb", "a\ufffd\ufffdb", "a\ufffd\ufffdb"},
	}
	for i, test := range tests {
		c, _, err := transform.String(NewCleaner(), test.s)
		if err != nil {
			t.Fatalf("test %d expected no error, got: %v", i, err)
		}
		if c != test.c {
			t.Errorf("test %d clean expected %q, got: %q", i, test.c, c)
		}
		n, _, err := transform.String(NewNormalizer(), test.s)
		if err != nil {
			t.Fatalf("test %d expected no error, got: %v", i, err)
		}
		if n != test.n {
			t.Errorf("test %d normalize expected %q, got: %q", i, test.n, n)
		}
	}
}

// TestCollapserLong checks input longer than the buffers a transform chain
// passes between its links, where a rune can be split across two calls.
func TestCollapserLong(t *testing.T) {
	const n = 5000
	for _, test := range []struct {
		s, c, n string
	}{
		{strings.Repeat("é", n), strings.Repeat("e", n), strings.Repeat("e", n)},
		{strings.Repeat("É\xff", n), strings.Repeat("E\ufffd", n), strings.Repeat("e\ufffd", n)},
	} {
		if c := MustClean(test.s); c != test.c {
			t.Errorf("clean of %d bytes expected %d bytes, got: %d", len(test.s), len(test.c), len(c))
		}
		if n := MustNormalize(test.s); n != test.n {
			t.Errorf("normalize of %d bytes expected %d bytes, got: %d", len(test.s), len(test.n), len(n))
		}
	}
}

// TestCollapserChunks checks that the result does not depend on how a transform
// chain cuts the input into calls. A chain hands a Collapser the text in pieces
// once it is longer than its buffers, so a space at the end of one piece and the
// word at the start of the next have to come out as they would from one call.
func TestCollapserChunks(t *testing.T) {
	// once runs the collapser over the whole of s in a single call
	once := func(c Collapser, s string) string {
		src := norm.NFD.String(s)
		dst := make([]byte, 4*len(src)+utf8.UTFMax)
		c.Reset()
		n, _, err := c.Transform(dst, []byte(src), true)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		return norm.NFC.String(string(dst[:n]))
	}
	for _, pattern := range []string{
		"ab ",
		"The  Quick Brown - Fox, $100 and LAWLE$$ ",
		"Amélie\t\tPok\xe9mon  Mr. & Mrs. O'Neil \u2b50\ufe0f ",
		"a  \xff\xfe  b - c ",
	} {
		long := strings.Repeat(pattern, 9000/len(pattern)+1)
		for n := 1; n <= 9000; n++ {
			// cover every length around the buffer sizes, sample the rest
			if n > 600 && n%97 != 0 && (n < 4000 || n > 4200) && (n < 8100 || n > 8300) {
				continue
			}
			s := long[:n]
			if c, exp := MustClean(s), once(newCleanCollapser(), s); c != exp {
				t.Fatalf("clean of %d bytes of %q differs from a single call:\n  %q\n  %q", n, pattern, c, exp)
			}
			if c, exp := MustNormalize(s), once(newNormalizeCollapser(), s); c != exp {
				t.Fatalf("normalize of %d bytes of %q differs from a single call:\n  %q\n  %q", n, pattern, c, exp)
			}
		}
	}
	// a collapser used for one string after another, with and without Reset
	c, dst := newCleanCollapser(), make([]byte, 64)
	for range 2 {
		n, _, err := c.Transform(dst, []byte("  a  b  "), true)
		if s := string(dst[:n]); err != nil || s != "a b" {
			t.Errorf("expected %q, got: %q (%v)", "a b", s, err)
		}
	}
	for _, s := range []string{strings.Repeat("ab ", 200), "  cd  "} {
		if c, exp := MustClean(s), strings.TrimSpace(strings.ReplaceAll(s, "  ", " ")); c != exp {
			t.Errorf("expected %q, got: %q", exp, c)
		}
	}
}

func TestCompareTitle(t *testing.T) {
	tests := []struct {
		a, b string
		exp  int
	}{
		{"STAR WARS - 1", "star wars 2", -1},
		{"star wars 2", "STAR WARS - 1", 1},
		{"STAR WARS - ep 1", "star wars ep 2", 0},
		{"star wars ep 2", "STAR WARS - ep 1", 0},
		{"rocky iv", "rocky", 1},
		{"rocky", "rocky iv", -1},
		{"ROCKY", "rocky", 0},
		{"harry potter and the goblet of fire", "harry potter & the goblet of fire", 0},
		{"harry potter & the goblet of fire", "harry potter and the goblet of fire", 0},
		{"KING & I", "KING AND I", 0},
		{"KING AND I", "KING & I", 0},
		{"VI", "100", 1},
		{"100", "VI", -1},
		{"1000 years", "Il deducio", -1},
		{"Il deducio", "1000 years", 1},
		{"1000 years", "I'll follow you down", -1},
		{"I'll follow you down", "1000 years", 1},
		{"star wars 8", "star wars x", -1},
		{"star wars x", "star wars 8", 1},
		{"100 years", "X years", -1},
		{"X years", "100 years", 1},
		{"i'll follow you", "Ill.Follow.You", 0},
		{"Ill.Follow.You", "i'll follow you", 0},
	}
	for i, test := range tests {
		v := compareTitle(test.a, test.b)()
		if v != test.exp {
			t.Errorf("test %d expected %q :: %q = %d, got: %d", i, test.a, test.b, test.exp, v)
		}
	}
}

func TestCompare(t *testing.T) {
	seed := time.Now().UnixNano()
	if s := os.Getenv("SEED"); s != "" {
		var err error
		if seed, err = strconv.ParseInt(s, 10, 64); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	}
	t.Logf("seed: %d", seed)
	exp := []string{
		"",
		"1",
		"13",
		"i",
		"i.am.legend",
		"ii",
		"'twas the night",
		"twas the night",
		"v",
		"v.for.vendetta",
		"xiii",
		"a\tthing.1998.dvdrip",
		"a thing.1999.dvdrip",
		"Amélie.mkv",
		"amelie.1998.mkv",
		"Amélie.1999.mkv",
		"ghostbusters.mkv",
		"ghostbusters.ii.mkv",
		"ghostbusters.afterlife.mkv",
		"Harry.Potter.and.the.Sorcerer's.Stone.2001.Theatrical.Cut.mkv",
		"Harry.Potter.and.the.Chamber.of.Secrets.2002.Theatrical.Cut.mkv",
		"Harry.Potter.and.the.Prisoner.of.Azkaban.2004.mkv",
		"Harry.Potter.&.the.Goblet.of.Fire.2005.mkv",
		"Harry.Potter.and.the.Order.of.the.Phoenix.2007.mkv",
		"Harry.Potter.and.the.Half-Blood.Prince.2009.mkv",
		"Harry.Potter.and.the.Deathly.Hallows.Part.1.2010.mkv",
		"Harry.Potter.and.the.Deathly.Hallows.Part.2.2011.mkv",
		"i.am.legend.mkv",
		"I'll.Follow.You.Down.2013.BluRay.Remux.1080p.AVC.DTS-HD.MA.5.1-HiFi.mkv",
		"LAWLE$$.mkv",
		"lawless.mkv",
		"rocky.mkv",
		"\trocky ii.mkv",
		"rocky iii.mkv",
		"rocky iv.mkv",
		"rocky v.mkv",
		"ROCKY 6.MKV",
		"\trocky\t6.mkv",
		"rocky 6.mkv",
		"rOCKY VII.mkv",
		"rocky 8.mkv",
		"rocky\tix.mkv\t\t",
		" ROCKY x.mkv ",
		"rocky 11.mkv",
		"the.matrix (part 2).1997.mkv",
		"The.Matrix.1999.mkv",
		"The.Matrix.Reloaded.2003.mkv",
		"The.Matrix.Revolutions.2004.mkv",
		"The.Matrix.Resurrections.2021.mkv",
		"The.Thomas.Crown.Affair.1968.720p.BluRay.AAC.2.0.x264-TDD.mkv",
		"The Thomas Crown Affair 1968 1080p BluRay AVC DTS-HD MA 2.0-CtrlHD",
		"The.Thomas.Crown.Affair.1968.4K.Remaster.720p.BluRay.AAC.2.0.x264-TDD.mkv",
		"The Thomas Crown Affair 1999 BluRay 1080p DTS-HD MA 5.1 AVC REMUX-FraMeSToR",
		"ultra vol. 1.mkv",
		"ultra vol 2.mkv",
		"ultra vol 3.1997.mkv",
		"ultra vol iii.1997.mkv",
		"ultra vol iv.mkv",
		"ultra vol. 8.mkv",
		"ultra vol ix.mkv",
		"ultra. vol. 13.mkv",
		"ultra vol xiii.mkv",
		"v.for.vendetta.mkv",
		"X.2022.BluRay.720p.DD.2.0.x264",
		"Star.Trek.Lower.Decks.S01E00.Decks.Creator.And.Cast.Talk.Season.1.1080p.CBS.WEB-DL.AAC2.0.x264-TEPES.mkv",
		"Star.Trek.Lower.Decks.S02.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb",
		"Star.Trek.Lower.Decks.S03E01.Grounded.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb.mkv",
		"Star.Trek.Lower.Decks.S03E02.The.Least.Dangerous.Game.1080p.AMZN.WEB-DL.DDP.5.1.H.264-GNOME.mkv",
		"Star.Trek.Lower.Decks.S03E03.1080p.WEB.H264-GLHF",
		"Star Trek - Lower Decks    S03E03 Mining the Mind's Mines (1080p AMZN Webrip x265 10bit EAC3 5.1 - Goki)[TAoE].mkv",
		"Star.Trek.Lower.Decks.S03E04.Room.For.Growth.1080p.AMZN.WEB-DL.DDP.5.1.H.264-GNOME.mkv",
		"Star.Trek.Lower.Decks.S03E05.1080p.WEB.h264-KOGi.mkv",
		"Star Trek - Lower Decks (2020) S03E03 Mining the Mind's Mines (1080p AMZN Webrip x265 10bit EAC3 5.1 - Goki)[TAoE].mkv",
		"Zebra.S01E02",
		"Zébra.2009.S00.x264-group.mkv",
		"Zebra.2009.S01.FLAC-group",
		"Zebra.2009.S01E02",
		"Zébra.2009.S02",
		"T-Pain - The Lost Remixes (2020) Mp3 320kbps [PMEDIA] ⭐️",
		"the cc - A 1999.mp3",
		"the cc - a - the remix 1999.mp3",
		"minesweeper.winnt",
		"super.SMASH.brothers.nsw",
		"super.smash.brothers.nsw",
		"C.S. Lewis - Die Chroniken.von.Narnia - Der.Koenig.von.Narnia.Bd.II.2013.German.Retail.EPUB.eBook-BitBook",
		"C.S..Lewis.-.Die.Chroniken.von.Narnia ~ Der.Koenig.von.Narnia.Bd.1.2013.eBook-BitBook",
		"C.S..Lewis~Die.Chroniken.von.Narnia~Der.Koenig.von.Narnia.Bd.3.2013.German.Retail.EPUB.eBook-BitBook",
	}
	m := make(map[string]int)
	releases := make([]Release, len(exp))
	for i, s := range exp {
		if releases[i] = ParseString(s); s != "" && releases[i].Title == "" {
			t.Fatalf("test %d expected non-empty title for %q", i, s)
		}
		if _, ok := m[s]; ok {
			t.Fatalf("test %d %q is already defined in map", i, s)
		}
		m[s] = i
		// t.Logf("%d: %q: %s", i, s, releases[i].Type)
	}
	// randomize
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(releases), func(i, j int) {
		releases[i], releases[j] = releases[j], releases[i]
	})
	// sort
	sort.Slice(releases, func(i, j int) bool {
		return Compare(releases[i], releases[j]) < 0
	})
	// check
	v := make([]string, len(releases))
	for i := 0; i < len(releases); i++ {
		v[i] = fmt.Sprintf("%o", releases[i])
	}
	if !cmp.Equal(v, exp) {
		t.Errorf("expected to be same, got:\n%s", cmp.Diff(exp, v))
	}
}

func TestFind(t *testing.T) {
	f := genre()
	tags := []Tag{
		NewTag(TagTypeText, nil, []byte("a"), []byte("a")),
		NewTag(TagTypeText, nil, []byte("b"), []byte("b")),
		NewTag(TagTypeGenre, f, []byte("anime"), []byte("anime")),
		NewTag(TagTypeText, nil, []byte("a"), []byte("a")),
		NewTag(TagTypeGenre, f, []byte("horror"), []byte("horror")),
		NewTag(TagTypeText, nil, []byte("c"), []byte("c")),
	}
	for i, test := range []struct {
		f     string
		count int
		n     int
		m     string
		s     string
		types []TagType
	}{
		{"", -1, 6, "%o", "a b anime a horror c", nil},
		{"", -1, 6, "%s", "a b Anime a Horror c", nil},
		{"a", -1, 2, "%o", "a a", nil},
		{"a", -1, 2, "%s", "a a", nil},
		{"a", 1, 1, "%o", "a", nil},
		{"a", 1, 1, "%s", "a", nil},
		{"A", 1, 0, "%o", "", nil},
		{"A", 1, 0, "%s", "", nil},

		{"", -1, 4, "%s", "a b a c", []TagType{TagTypeText}}, // 8
		{"b", -1, 1, "%s", "b", []TagType{TagTypeText}},

		{"", -1, 2, "%o", "anime horror", []TagType{TagTypeGenre}}, // 10
		{"", -1, 2, "%s", "Anime Horror", []TagType{TagTypeGenre}},

		{"anime", -1, 1, "%o", "anime", nil}, // 12
		{"ANIME", -1, 0, "%o", "", nil},
		{"Anime", -1, 0, "%o", "", nil},
		{"anime", -1, 1, "%o", "anime", []TagType{TagTypeGenre}},
		{"ANIME", -1, 0, "%o", "", []TagType{TagTypeGenre}},
		{"Anime", -1, 0, "%o", "", []TagType{TagTypeGenre}},

		{"horror", -1, 1, "%s", "Horror", nil}, // 18
		{"HORROR", -1, 1, "%s", "Horror", nil},
		{"Horror", -1, 1, "%s", "Horror", nil},
		{"horror", -1, 1, "%s", "Horror", []TagType{TagTypeGenre}},
		{"HORROR", -1, 1, "%s", "Horror", []TagType{TagTypeGenre}},
		{"Horror", -1, 1, "%s", "Horror", []TagType{TagTypeGenre}},

		{"(?i)^anime$", -1, 1, "%r", "Anime", nil}, // 24
		{"(?i)^(anime|horror)$", -1, 2, "%r", "Anime Horror", nil},
	} {
		v, _ := Find(tags, test.f, test.count, rune(test.m[1]), test.types...)
		if n := len(v); n != test.n {
			t.Errorf("test %d expected %d, got: %d", i, test.n, n)
		}
		if s := joinTags(v, test.m, " "); s != test.s {
			t.Errorf("test %d expected %q, got: %q", i, test.s, s)
		}
	}
}

func TestTagInfo_find(t *testing.T) {
	f := genre()
	for i, test := range []string{"anime", "Anime", "ANIME", "ANiME"} {
		tag := NewTag(TagTypeGenre, f, []byte(test), []byte(test))
		info := tag.Info()
		if info == nil {
			t.Fatalf("test %d expected not nil", i)
		}
		if s, exp := info.Tag(), "Anime"; s != exp {
			t.Errorf("test %d expected %s, got: %s", i, exp, s)
		}
	}
}

// TestTagInfo_newType checks that a tag info with a type that does not exist
// is refused whether or not it has a title.
func TestTagInfo_newType(t *testing.T) {
	for _, title := range []string{"", "Title"} {
		if info, err := taginfo.New("X", title, "", "", "gaem", ""); err == nil {
			t.Errorf("title %q: expected an error, got: %v", title, info)
		}
	}
	if _, err := taginfo.New("X", "", "", "", "game", ""); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// TestTagInfo_newRegexp checks that a regexp that does not compile is refused
// when the tag info is made, though it is compiled when first used, and that
// a tag info matches from several goroutines at once.
func TestTagInfo_newRegexp(t *testing.T) {
	for _, re := range []string{"(", "a{2,1}", `\p{Nope}`, "[z-a]"} {
		if info, err := taginfo.New("X", "", re, "", "", ""); err == nil {
			t.Errorf("%q: expected an error, got: %v", re, info)
		}
	}
	info, err := taginfo.New("MULTi$1", "Multi ($1)", `multi[\-\._ ]?(\d+)`, "", "", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if tag, _ := info.Expand("multi5"); !info.Match("MULTI5") || info.Match("multi") || tag != "MULTi5" {
				t.Errorf("expected multi5 to match and expand to MULTi5, got: %q", tag)
			}
		})
	}
	wg.Wait()
}

func TestParseRoman(t *testing.T) {
	for i, test := range []struct {
		s   string
		exp int
		t   bool
	}{
		{"", 0, true},
		{"i", 1, true},
		{"ok", -1, false},
		{"more", -1, false},
		{"lcmxiv", -1, false}, // invalid 864
		{"dccclxiv", 864, true},
		{"cmxcix", 999, true},
		{"mm", 2000, true},
		{"mmiv", 2004, true},
		{"mmxvii", 2017, true},
		{"mmxviii", 2018, true},
	} {
		v, ok := parseRoman(test.s)
		if ok != test.t {
			t.Fatalf("test %d expected %t, got: %t", i, test.t, ok)
		}
		if v != test.exp {
			t.Errorf("test %d expected to be %d, got: %d", i, test.exp, v)
		}
		// t.Logf("%q: %d", test.s, v)
	}
}

// TestTagInfo_placeholders checks that every tag info with a $1 ... $N in its
// tag or title fills it from its own capture groups.
func TestTagInfo_placeholders(t *testing.T) {
	types := map[string]TagType{
		"edition":    TagTypeEdition,
		"language":   TagTypeLanguage,
		"other":      TagTypeOther,
		"resolution": TagTypeResolution,
		"size":       TagTypeSize,
		"source":     TagTypeSource,
	}
	tests := map[string]struct {
		s, tag, title string
	}{
		"edition/$1.Anniversary.Edition": {"25th Anniversary Edition", "25th.Anniversary.Edition", "Anniversary (25th)"},
		"language/$1$2UB$3":              {"GERSUB", "GERSUB", "Subs (GER)"},
		"language/MULTi$1":               {"multi5", "MULTi5", "Multi (5)"},
		"other/$1X":                      {"19XX", "19XX", "19XX"},
		"other/Plus.$1.Trainer":          {"plus 10 trainer", "Plus.10.Trainer", "Trainer (10)"},
		"other/REPACK$1":                 {"repack3", "REPACK3", "Repack (3)"},
		"resolution/$1p":                 {"1600p", "1600p", "Other (1600p)"},
		"size/$1$2":                      {"4.7GB", "4.7GB", "4.7 GB"},
		"source/$1INCH.ViNYL":            {"12 inch vinyl", "12INCH.ViNYL", "Vinyl (12 inch)"},
	}
	var n int
	for typ, infos := range taginfo.All() {
		for _, info := range infos {
			if !strings.Contains(info.Tag()+info.Title(), "$") {
				continue
			}
			name := typ + "/" + info.Tag()
			test, ok := tests[name]
			if !ok {
				t.Errorf("%s has a placeholder and no test", name)
				continue
			}
			n++
			if tag, title := info.Expand(test.s); tag != test.tag || title != test.title {
				t.Errorf("%s expected %q to expand to %q, %q, got: %q, %q", name, test.s, test.tag, test.title, tag, title)
			}
			tag := NewTag(types[typ], findFunc(infos...), []byte(test.s), []byte(test.s))
			if s := tag.Normalize(); s != test.tag {
				t.Errorf("%s expected %q to normalize to %q, got: %q", name, test.s, test.tag, s)
			}
			if s := tag.InfoTitle(); s != test.title {
				t.Errorf("%s expected %q to have title %q, got: %q", name, test.s, test.title, s)
			}
		}
	}
	if n != len(tests) {
		t.Errorf("expected %d tag infos with a placeholder, got: %d", len(tests), n)
	}
}

func TestTagInfo_regexp(t *testing.T) {
	infos, err := taginfo.LoadAll()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	var types []string
	for k := range infos {
		types = append(types, k)
	}
	sort.Strings(types)
	for _, typ := range types {
		for i, info := range infos[typ] {
			tag := info.Tag()
			if strings.Contains(tag, "$") {
				t.Logf("skipping type %q line %d tag %s (%q)", typ, i+2, tag, info.Title())
				continue
			}
			restr := info.Regexp()
			if restr == "" {
				restr = tag
			}
			re, err := regexp.Compile(`^(?i)` + restr + `$`)
			if err != nil {
				t.Errorf("type %q line %d tag %s (%q) expected no error compiling %q, got: %v", typ, i+2, tag, info.Title(), info.Regexp(), err)
				continue
			}
			if !re.MatchString(info.Tag()) {
				t.Errorf("type %q line %d tag %s (%q) does not match %q", typ, i+2, tag, info.Title(), info.Regexp())
			}
		}
	}
}

func TestExport_tests(t *testing.T) {
	if s := os.Getenv("TESTS"); s != "export" {
		return
	}
	var keys []string
	m := make(map[string]rls)
	for i, test := range rlsTests(t) {
		if _, ok := m[test.s]; ok {
			t.Fatalf("test %d %q is a duplicate!", i, test.s)
		}
		keys, m[test.s] = append(keys, test.s), test.exp
	}
	sort.SliceStable(keys, func(i, j int) bool {
		var cmp int
		a, b := m[keys[i]], m[keys[j]]
		for _, f := range []func() int{
			compareInt(CompareMap[ParseType(a.Type)], CompareMap[ParseType(b.Type)]),
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
			compareString(keys[i], keys[j]),
		} {
			if cmp = f(); cmp != 0 {
				break
			}
		}
		return cmp < 0
	})
	buf := new(bytes.Buffer)
	fmt.Fprintf(buf, "# add tests to bottom of this file then run `TESTS=export go test` to order.\n")
	for i, key := range keys {
		fmt.Fprintf(buf, "%q:\n", key)
		v := reflect.ValueOf(m[key])
		for j := 0; j < v.Type().NumField(); j++ {
			name := strings.ToLower(v.Type().Field(j).Name)
			switch name {
			case "seriesepisodes":
				name = "seriesEpisodes"
			}
			switch v.Field(j).Kind() {
			case reflect.Int:
				if i := v.Field(j).Int(); i != 0 {
					fmt.Fprintf(buf, "  %s: %d\n", name, i)
				}
			case reflect.String:
				if s := v.Field(j).String(); s != "" {
					fmt.Fprintf(buf, "  %s: %q\n", name, s)
				}
			default:
				t.Fatalf("unknown type %T", v.Field(i).Interface())
			}
		}
	}
	if err := os.WriteFile("tests.yaml", buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExport_taginfo(t *testing.T) {
	if s := os.Getenv("TESTS"); s != "export" {
		return
	}
	buf, err := exportTaginfo("taginfo/taginfo.csv")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if err := os.WriteFile("taginfo/taginfo.csv", buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTaginfo_order checks taginfo.csv is in export order, as a type's tags
// are matched in file order and the first match wins (ie, WEB-DL must come
// before WEB).
func TestTaginfo_order(t *testing.T) {
	exp, err := exportTaginfo("taginfo/taginfo.csv")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	buf, err := os.ReadFile("taginfo/taginfo.csv")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	a, b := strings.Split(string(buf), "\n"), strings.Split(string(exp), "\n")
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			t.Fatalf("taginfo/taginfo.csv line %d is not in export order, expected %q, got %q (run `TESTS=export go test` to fix)", i+1, b[i], a[i])
		}
	}
	if len(a) != len(b) {
		t.Fatalf("taginfo/taginfo.csv has %d lines, export has %d (run `TESTS=export go test` to fix)", len(a), len(b))
	}
}

// exportTaginfo loads the tag info from file, returning it as csv in export
// order.
func exportTaginfo(file string) ([]byte, error) {
	infos, err := taginfo.LoadFile(file)
	if err != nil {
		return nil, err
	}
	// the types in a fixed order: the comparison below is not a strict order
	// for every pair of rows, so the result depends on where a type's rows
	// start, which the order of a map would change from run to run
	var all [][]string
	for _, k := range slices.Sorted(maps.Keys(infos)) {
		v := infos[k]
		for _, info := range v {
			title := info.Title()
			if info.Tag() == title {
				title = ""
			}
			var excl string
			if info.Excl() {
				excl = "1"
			}
			all = append(all, []string{
				k,
				info.Tag(),
				title,
				info.Regexp(),
				info.Other(),
				Type(info.Type()).String(),
				excl,
			})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		var cmp int
		a, b := all[i], all[j]
		for _, f := range []func() int{
			compareString(a[0], b[0]),
			compareHasDollar(a[1], b[1]),
			compareResolution(a, b),
			compareSource(a, b),
			compareRegion(a, b),
			comparePlatform(a, b),
			compareCodec(a, b),
			compareHDR(a, b),
			compareChannels(a, b),
			compareSuffix(a, b),
			comparePrefix(a[1], b[1]),
			compareInts(a[1], b[1]),
			compareString(strings.ToLower(a[1]), strings.ToLower(b[1])),
			compareString(a[1], b[1]),
		} {
			if cmp = f(); cmp != 0 {
				break
			}
		}
		return cmp < 0
	})
	buf := new(bytes.Buffer)
	_, _ = buf.WriteString("Type,Tag,Title,Regexp,Other,ReleaseType,TypeExclusive\n")
	for _, v := range all {
		_, _ = buf.WriteString(strings.Join(v, ",") + "\n")
	}
	return buf.Bytes(), nil
}

func compareRegion(a, b []string) func() int {
	return func() int {
		if a[0] != "region" || b[0] != "region" {
			return 0
		}
		switch {
		case len(a[1]) < len(b[1]):
			return -1
		case len(b[1]) < len(a[1]):
			return 1
		}
		return strings.Compare(a[1], b[1])
	}
}

func comparePlatform(a, b []string) func() int {
	return func() int {
		as, bs := strings.ToLower(a[1]), strings.ToLower(b[1])
		if a[0] != "platform" || b[0] != "platform" || !strings.HasPrefix(as, "win") || !strings.HasPrefix(bs, "win") {
			return 0
		}
		switch {
		case len(a[1]) < len(b[1]):
			return 1
		case len(b[1]) < len(a[1]):
			return -1
		}
		return 0
	}
}

func compareResolution(a, b []string) func() int {
	return func() int {
		if a[0] != "resolution" || b[0] != "resolution" {
			return 0
		}
		switch {
		case a[1] == "PN.Selector" && b[1] != "PN.Selector":
			return -1
		case b[1] == "PN.Selector" && a[1] != "PN.Selector":
			return 1
		}
		return 0
	}
}

func compareCodec(a, b []string) func() int {
	return func() int {
		if a[0] != "codec" || b[0] != "codec" {
			return 0
		}
		switch {
		case len(a[1]) < len(b[1]):
			return 1
		case len(b[1]) < len(a[1]):
			return -1
		}
		return 0
	}
}

func compareHDR(a, b []string) func() int {
	return func() int {
		if a[0] != "hdr" || b[0] != "hdr" {
			return 0
		}
		switch {
		case len(a[1]) < len(b[1]):
			return 1
		case len(b[1]) < len(a[1]):
			return -1
		}
		return 0
	}
}

func compareChannels(a, b []string) func() int {
	return func() int {
		if a[0] != "channels" || b[0] != "channels" {
			return 0
		}
		switch cmp := strings.Compare(a[1], b[1]); {
		case cmp < 0:
			return 1
		case cmp > 0:
			return -1
		}
		return 0
	}
}

func compareSource(a, b []string) func() int {
	return func() int {
		if a[0] != "source" || b[0] != "source" {
			return 0
		}
		if !strings.HasPrefix(a[1], "UHD.") || !strings.HasPrefix(b[1], "UHD.") {
			return 0
		}
		switch {
		case a[1] == "UHD.BluRay":
			return 1
		case b[1] == "UHD.BluRay":
			return -1
		}
		return 0
	}
}

func comparePrefix(a, b string) func() int {
	return func() int {
		if a == b {
			return 0
		}
		switch {
		case strings.HasPrefix(strings.ToLower(a), strings.ToLower(b)):
			return -1
		case strings.HasPrefix(strings.ToLower(b), strings.ToLower(a)):
			return 1
		}
		return 0
	}
}

func compareSuffix(a, b []string) func() int {
	return func() int {
		if a[0] != "ext" || b[0] != "ext" || a[1] == b[1] {
			return 0
		}
		switch {
		case strings.HasSuffix(b[1], a[1]):
			return -1
		case strings.HasSuffix(a[1], b[1]):
			return 1
		}
		return 0
	}
}

func compareHasDollar(a, b string) func() int {
	return func() int {
		ac, bc := strings.Contains(a, "$"), strings.Contains(b, "$")
		switch {
		case ac && bc:
			return strings.Compare(a, b)
		case ac && !bc:
			return 1
		case bc && !ac:
			return -1
		}
		return 0
	}
}

func compareInts(a, b string) func() int {
	return func() int {
		am, bm := number.FindStringSubmatch(a), number.FindStringSubmatch(b)
		as, bs := number.ReplaceAllString(a, ""), number.ReplaceAllString(b, "")
		if len(am) != 0 && len(bm) != 0 && as != "" && bs != "" && as == bs {
			var cmp int
			switch cmp = compareIntString(a, b)(); {
			case cmp < 0:
				return 1
			case cmp > 0:
				return -1
			}
			return cmp
		}
		return strings.Compare(strings.ToLower(as), strings.ToLower(bs))
	}
}

var number = regexp.MustCompile(`\d+(?:\.\d+)?`)

type rls struct {
	Type string

	Artist   string
	Title    string
	Subtitle string
	Alt      string

	Platform string
	Arch     string

	Source     string
	Resolution string
	Framerate  string
	Collection string

	Year    int
	Month   int
	Day     int
	Yearend int

	Series         int
	Episode        int
	Absolute       int
	SeriesEpisodes string
	Version        string
	Disc           string

	Codec    string
	HDR      string
	Bitdepth string
	Audio    string
	Channels string
	Other    string
	Cut      string
	Edition  string
	Language string

	Size      string
	Region    string
	Container string
	Genre     string
	ID        string
	Group     string
	Meta      string
	Site      string
	Sum       string
	Pass      string
	Req       int
	Ext       string

	Unused string
}

func buildRls(r Release) rls {
	absolute, _ := r.Absolute()
	req := 0
	if r.Req {
		req = 1
	}
	var seriesEpisodes []string
	if eps := r.SeriesEpisodes(); len(eps) > 1 {
		for _, ep := range eps {
			seriesEpisodes = append(seriesEpisodes, fmt.Sprintf("S%02dE%02d", ep[0], ep[1]))
		}
	}
	return rls{
		Type: r.Type.String(),

		Artist:   r.Artist,
		Title:    r.Title,
		Subtitle: r.Subtitle,
		Alt:      r.Alt,

		Platform: r.Platform,
		Arch:     r.Arch,

		Source:     r.Source,
		Resolution: r.Resolution,
		Framerate:  r.FrameRate,
		Collection: r.Collection,

		Year:    r.Year,
		Month:   r.Month,
		Day:     r.Day,
		Yearend: r.YearEnd,

		Series:         r.Series,
		Episode:        r.Episode,
		Absolute:       absolute,
		SeriesEpisodes: strings.Join(seriesEpisodes, " "),
		Version:        r.Version,
		Disc:           r.Disc,

		Codec:    strings.Join(r.Codec, " "),
		HDR:      strings.Join(r.HDR, " "),
		Bitdepth: r.BitDepth,
		Audio:    strings.Join(r.Audio, " "),
		Channels: r.Channels,
		Other:    strings.Join(r.Other, " "),
		Cut:      strings.Join(r.Cut, " "),
		Edition:  strings.Join(r.Edition, " "),
		Language: strings.Join(r.Language, " "),

		Size:      r.Size,
		Region:    r.Region,
		Container: r.Container,
		Genre:     r.Genre,
		ID:        r.ID,
		Group:     r.Group,
		Meta:      strings.Join(r.Meta, " "),
		Site:      r.Site,
		Sum:       r.Sum,
		Pass:      r.Pass,
		Ext:       r.Ext,
		Req:       req,
		Unused:    joinTags(r.Unused(), "%s", " "),
	}
}

type rlsTest struct {
	s   string
	exp rls
}

func rlsTests(tb testing.TB) []rlsTest {
	buf, err := os.ReadFile("tests.yaml")
	if err != nil {
		tb.Fatal(err)
	}
	s := bufio.NewScanner(bytes.NewReader(buf))
	var tests []rlsTest
	test := rlsTest{}
	var count int
	for i, n := 0, 0; s.Scan(); count, i = count+1, i+1 {
		switch line := s.Bytes(); {
		case bytes.HasPrefix(line, []byte(`"`)):
			if i > 1 {
				tests, test = append(tests, test), rlsTest{}
			}
			if n = bytes.LastIndexByte(line, '"'); n == -1 {
				tb.Fatalf("unable to locate \" on line %d item %d: %q", count, i, string(line))
			}
			var err error
			test.s, err = strconv.Unquote(string(line[:n+1]))
			if err != nil {
				tb.Fatalf("unable to unquote line %d item %d: %v", count, i, err)
			}
		case bytes.HasPrefix(line, []byte(`  `)):
			if n = bytes.IndexByte(line, ':'); n == -1 {
				tb.Fatalf("unable to locate : on line %d item %d", count, i)
			}
			name := strings.ToUpper(string(line[2:3])) + string(line[3:n])
			switch name {
			case "Id", "Hdr":
				name = strings.ToUpper(name)
			}
			f := reflect.ValueOf(&test.exp).Elem().FieldByName(name)
			switch f.Kind() {
			case reflect.Int:
				i, err := strconv.ParseInt(string(line[n+2:]), 10, 64)
				if err != nil {
					tb.Fatalf("unable to convert int for %s on line %d: %v", name, count, err)
				}
				f.SetInt(i)
			case reflect.String:
				s, err := strconv.Unquote(string(line[n+2:]))
				if err != nil {
					tb.Fatalf("unable to unquote string for %s on line %d: %v", name, count, err)
				}
				f.SetString(s)
			}
		case bytes.HasPrefix(line, []byte(`#`)):
		default:
			tb.Fatalf("unknown line %d item %d: %q", count, i, string(line))
		}
	}
	if err := s.Err(); err != nil {
		tb.Fatal(err)
	}
	return append(tests, test)
}

// genre returns a find func for the embedded genres.
func genre() taginfo.FindFunc {
	return taginfo.Find(taginfo.All()["genre"]...)
}

// joinTags joins tags using a formatting string and a separator.
func joinTags(tags []Tag, str, sep string) string {
	var v []string
	for _, tag := range tags {
		v = append(v, fmt.Sprintf(str, tag))
	}
	return strings.Join(v, sep)
}

func groupInfos() map[string][]*taginfo.Taginfo {
	var groups []*taginfo.Taginfo
	for _, group := range []struct {
		tag, typ string
	}{
		{"CODEX", "game"},
		{"DARKSiDERS", "game"},
		{"D-Z0N3", "movie"},
		{"MrSeeN-SiMPLE", ""},
	} {
		groups = append(groups, taginfo.Must(group.tag, "", "", "", group.typ, ""))
	}
	return map[string][]*taginfo.Taginfo{
		"group": groups,
	}
}

// TestLeadingGroupMark checks that a hyphenated word after the last bracket
// gives way to the group in the leading bracket only when it begins with the
// initials of that group, and is a tag and the group in any other name.
func TestLeadingGroupMark(t *testing.T) {
	tests := []struct {
		s     string
		group string
		other []string
	}{
		{"[Hakata Ramen] High Score Girl (S2) 02 [1080p HEVC Sub-1x] HR-DR", "Hakata Ramen", nil},
		{"[TGx] Movie (2020) [1080p] REPACK-GRP", "GRP", []string{"REPACK"}},
		{"[TGx] Show S01E02 [1080p WEB-DL] PROPER-NTb", "NTb", []string{"PROPER"}},
		{"[TGx] Movie (2020) [2160p UHD BluRay] REMUX-FraMeSToR", "FraMeSToR", []string{"REMUX"}},
		{"[Group] Movie (2010) [1080p] HR-GRP", "GRP", []string{"HR"}},
	}
	for _, test := range tests {
		r := ParseString(test.s)
		if r.Group != test.group {
			t.Errorf("%q: expected group %q, got: %q", test.s, test.group, r.Group)
		}
		if !slices.Equal(r.Other, test.other) {
			t.Errorf("%q: expected other %q, got: %q", test.s, test.other, r.Other)
		}
	}
}
