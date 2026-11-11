package rls

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The corpus under testdata/corpus is independent ground truth: it is labelled
// by other projects, so unlike tests.yaml -- which TestExport_tests regenerates
// from this parser's own output -- it can actually say whether a change made
// the parser better or worse.
//
// The anitomy and anitogo fixtures are committed with their licence files, as
// the bytes of the upstream commits that testdata/corpus/README.md names. The
// other data files are gitignored (*.txt, *.json), so the tests that read them
// skip when they are not present and the package still builds and tests
// without them.

const corpusDir = "testdata/corpus"

// labelledCase is one row of a third-party parser's test fixtures. The field
// names are shared across anitomy, habari and anitogo.
type labelledCase struct {
	FileName        string          `json:"file_name"`
	AnimeTitle      json.RawMessage `json:"anime_title"`
	Title           json.RawMessage `json:"title"`
	ReleaseGroup    json.RawMessage `json:"release_group"`
	EpisodeNumber   json.RawMessage `json:"episode_number"`
	AnimeSeason     json.RawMessage `json:"anime_season"`
	SeasonNumber    json.RawMessage `json:"season_number"`
	FileChecksum    json.RawMessage `json:"file_checksum"`
	VideoResolution json.RawMessage `json:"video_resolution"`
	FileExtension   json.RawMessage `json:"file_extension"`
	ReleaseVersion  json.RawMessage `json:"release_version"`
	EpisodeTitle    json.RawMessage `json:"episode_title"`
}

// first returns the first value of a field that may be a string or a list of
// them, which differs between the three fixture sets.
func first(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var v []string
	if json.Unmarshal(raw, &v) == nil && len(v) != 0 {
		return v[0]
	}
	return ""
}

func loadLabelled(tb testing.TB, name string) []labelledCase {
	buf, err := os.ReadFile(corpusDir + "/labelled/" + name)
	if err != nil {
		tb.Skipf("corpus not present: %v", err)
	}
	var v []labelledCase
	if err := json.Unmarshal(buf, &v); err != nil {
		tb.Fatalf("%s: %v", name, err)
	}
	return v
}

// num parses a fixture's episode or season value, which is zero padded and may
// be "00".
func fixtureNum(s string) int {
	if t := strings.TrimLeft(s, "0"); t != "" {
		n, err := strconv.Atoi(t)
		if err != nil {
			return -1
		}
		return n
	}
	if strings.Trim(s, "0") == "" {
		return 0
	}
	return -1
}

type tally struct{ ok, total int }

func (t *tally) add(ok bool) {
	t.total++
	if ok {
		t.ok++
	}
}

func (t tally) pct() float64 {
	if t.total == 0 {
		return 0
	}
	return 100 * float64(t.ok) / float64(t.total)
}

// floors are the accuracy levels already reached, so a change that drops one is
// a regression rather than a surprise. They are the measured values rounded
// down to a whole percent, and 99 where the measured value is 100: a change
// that trades a case away has to lower its floor and say why.
var floors = map[string]map[string]float64{
	"anitomy.json": {"title": 65, "group": 92, "episode": 64, "episodeTitle": 69, "season": 90, "checksum": 99, "ext": 99, "resolution": 90, "version": 92},
	"habari.json":  {"title": 70, "group": 88, "episode": 63, "episodeTitle": 55, "season": 89, "checksum": 99, "ext": 99, "resolution": 93, "version": 71},
	"anitogo.json": {"title": 65, "group": 93, "episode": 63, "episodeTitle": 65, "season": 93, "checksum": 99, "ext": 99, "resolution": 90, "version": 92},
}

// upstream are the SHA-256 sums of the committed fixtures, which are the files
// of the upstream commits named in testdata/corpus/README.md.
var upstream = map[string]string{
	"anitomy.json": "a47d5336e356b0ffd50ae36128fdb861985c848dc72bca33407a69cba9fbf679",
	"anitogo.json": "2bd2f6f1c09b1e66d5fe31049896c3dff628892cd7ce5b4aafca6c48e08a576c",
}

// TestLabelledUpstream checks that the committed fixtures are present and are
// the upstream bytes, so that the floors measure what they were set against.
func TestLabelledUpstream(t *testing.T) {
	for name, exp := range upstream {
		buf, err := os.ReadFile(corpusDir + "/labelled/" + name)
		if err != nil {
			t.Errorf("expected %s to be present: %v", name, err)
			continue
		}
		if sum := fmt.Sprintf("%x", sha256.Sum256(buf)); sum != exp {
			t.Errorf("%s: expected sha256 %s, got: %s", name, exp, sum)
		}
	}
}

func TestAccuracyLabelled(t *testing.T) {
	for _, name := range []string{"anitomy.json", "habari.json", "anitogo.json"} {
		t.Run(name, func(t *testing.T) {
			cases := loadLabelled(t, name)
			m := map[string]*tally{}
			get := func(k string) *tally {
				if m[k] == nil {
					m[k] = &tally{}
				}
				return m[k]
			}
			for _, c := range cases {
				if c.FileName == "" {
					continue
				}
				r := ParseString(c.FileName)
				want := first(c.AnimeTitle)
				if want == "" {
					want = first(c.Title)
				}
				if want != "" {
					get("title").add(MustNormalize(r.Title) == MustNormalize(want))
				}
				if g := first(c.ReleaseGroup); g != "" {
					get("group").add(strings.EqualFold(r.Group, g))
				}
				if e := first(c.EpisodeNumber); e != "" {
					get("episode").add(r.Episode == fixtureNum(e))
				}
				season := first(c.AnimeSeason)
				if season == "" {
					season = first(c.SeasonNumber)
				}
				if season != "" {
					get("season").add(r.Series == fixtureNum(season))
				}
				if s := first(c.FileChecksum); s != "" {
					get("checksum").add(strings.EqualFold(r.Sum, s))
				}
				if s := first(c.VideoResolution); s != "" {
					get("resolution").add(r.Resolution != "")
				}
				if s := first(c.FileExtension); s != "" {
					get("ext").add(strings.EqualFold(r.Ext, s))
				}
				if s := first(c.ReleaseVersion); s != "" {
					get("version").add(strings.EqualFold(strings.TrimPrefix(r.Version, "v"), strings.TrimPrefix(strings.ToLower(s), "v")))
				}
				if s := first(c.EpisodeTitle); s != "" {
					get("episodeTitle").add(MustNormalize(r.Subtitle) == MustNormalize(s))
				}
			}
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v := *m[k]
				t.Logf("%-13s %4d/%-4d %5.1f%%", k, v.ok, v.total, v.pct())
				if floor, ok := floors[name][k]; ok && v.pct() < floor {
					t.Errorf("%s: %.1f%% is below the %.1f%% floor", k, v.pct(), floor)
				}
			}
		})
	}
}

// TestCorpusStructural reports what the parser does across the unlabelled
// corpora. There is no ground truth here, so it asserts only the properties
// that hold by construction, and logs the rest for comparison across changes.
func TestCorpusStructural(t *testing.T) {
	for _, name := range []string{"nyaa.txt", "sports.txt"} {
		t.Run(name, func(t *testing.T) {
			if testing.Short() {
				t.Skip("skipping corpus walk in short mode")
			}
			f, err := os.Open(corpusDir + "/" + name)
			if err != nil {
				t.Skipf("corpus not present: %v", err)
			}
			defer f.Close()
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 1<<20), 1<<20)
			var n, noTitle, noGroup, hasSum, hasHDR int
			typ := map[string]int{}
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				n++
				r := ParseString(line)
				// the tag stream must reproduce the input exactly: this is the
				// invariant every change has to preserve
				if s := fmt.Sprintf("%o", r); s != line {
					t.Fatalf("round trip failed:\n in: %q\nout: %q", line, s)
				}
				typ[r.Type.String()]++
				if strings.TrimSpace(r.Title) == "" {
					noTitle++
				}
				if r.Group == "" && r.Site == "" {
					noGroup++
				}
				if r.Sum != "" {
					hasSum++
				}
				if len(r.HDR) != 0 {
					hasHDR++
				}
			}
			if err := sc.Err(); err != nil {
				t.Fatal(err)
			}
			t.Logf("%d names: no title %d (%.1f%%), no group/site %d (%.1f%%), checksum %d, hdr %d",
				n, noTitle, 100*float64(noTitle)/float64(n), noGroup, 100*float64(noGroup)/float64(n), hasSum, hasHDR)
			keys := make([]string, 0, len(typ))
			for k := range typ {
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool { return typ[keys[i]] > typ[keys[j]] })
			var b strings.Builder
			for _, k := range keys {
				if k == "" {
					k = "unknown"
				}
				fmt.Fprintf(&b, " %s=%d", k, typ[k])
			}
			t.Logf("types:%s", b.String())
			// a title is always recoverable from a non-empty name
			if noTitle*100/n > 1 {
				t.Errorf("%d names (%.1f%%) produced no title", noTitle, 100*float64(noTitle)/float64(n))
			}
		})
	}
}

// TestAbsoluteEpisode pins the distinction between the three ways a release can
// carry an episode number. Release.Episode alone cannot tell them apart, which
// is why Absolute exists.
func TestAbsoluteEpisode(t *testing.T) {
	for _, test := range []struct {
		s                         string
		series, episode, absolute int
		desc                      string
	}{
		{"[SubsPlease] One Piece - 1169 (1080p) [ABCD1234].mkv", 0, 1169, 1169, "bare number, no season: absolute"},
		{"[HorribleSubs] Detective Conan - 862 [1080p].mkv", 0, 862, 862, "same, three digits"},
		{"[Group] Show Title - 05 - Episode Name (1080p).mkv", 0, 5, 5, "bare number with an episode title"},
		{"One.Piece.E1100.1080p.WEB.h264-GRP", 0, 1100, 1100, "written with an E, no season: absolute"},
		{"[Group] One Piece EP1168 1080p WEB-DL AAC2.0 H.264", 0, 1168, 1168, "written with EP"},
		{"[Group] Show Title Episode 909 [1080p]", 0, 909, 909, "written out"},
		{"[Group] Show Title 2nd Season - 08 [1080p]", 2, 8, 0, "ordinal season qualifies it"},
		{"[Group] Show Title Second Season - 08 [1080p]", 2, 8, 0, "so does one written as a word"},
		{"[Group] Show Title E05-E07 [1080p]", 0, 5, 0, "the ends of a range between two tags are not one episode"},
		{"[Group] Show Title (E001-E130) (480p)", 0, 1, 0, "the same in a bracket"},
		{"[Group] Show Title E05~E07 [1080p]", 0, 5, 0, "and with a tilde"},
		{"[Group] Show Title E07-E05 [1080p]", 0, 7, 7, "a lower number after the hyphen is no range"},
		{"[Group] Show Title - 21.5 (720p) [4191AC97].mkv", 0, 21, 0, "half an episode has no absolute number"},
		{"[Group] Show Title - 01 ~ 12 [1080p][BATCH]", 0, 0, 0, "a batch is a range, not an episode"},
		{"[SubsPlease] Hell Mode S2 - 04 (480p) [20BE14E8].mkv", 2, 4, 0, "season marker qualifies it: not absolute"},
		{"Show.Title.S01E05.1080p.WEB-DL-GRP", 1, 5, 0, "SxxExx is qualified"},
		{"Show.Title.S00E05.1080p.WEB-DL-GRP", 0, 5, 0, "S00Exx is a special, not an absolute number"},
		{"Show.Title.2024.03.11.1080p.WEB-DL-GRP", 0, 0, 0, "date based, no episode at all"},
	} {
		t.Run(test.desc, func(t *testing.T) {
			r := ParseString(test.s)
			absolute, ok := r.Absolute()
			if r.Series != test.series || r.Episode != test.episode || absolute != test.absolute || ok != (test.absolute != 0) {
				t.Errorf("%q\n  expected series=%d episode=%d absolute=%d\n  got      series=%d episode=%d absolute=%d ok=%t",
					test.s, test.series, test.episode, test.absolute, r.Series, r.Episode, absolute, ok)
			}
		})
	}
}

// TestAbsoluteInvariants checks the properties Absolute has to hold across the
// whole corpus, since it is derived rather than parsed directly.
func TestAbsoluteInvariants(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping corpus walk in short mode")
	}
	f, err := os.Open(corpusDir + "/nyaa.txt")
	if err != nil {
		t.Skipf("corpus not present: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var n, abs int
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		n++
		r := ParseString(line)
		switch absolute, ok := r.Absolute(); {
		case !ok:
		case absolute != r.Episode:
			t.Fatalf("%q: Absolute %d does not match Episode %d", line, absolute, r.Episode)
		case r.Series != 0:
			t.Fatalf("%q: Absolute %d set alongside season %d", line, absolute, r.Series)
		default:
			abs++
		}
	}
	t.Logf("%d of %d names carry an absolute episode number", abs, n)
}
