package rls

import (
	"bufio"
	"cmp"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/autobrr/rls/taginfo"
)

// The corpus under testdata/corpus is independent ground truth: it is labelled
// by other projects, so unlike tests.yaml -- which TestExport_tests regenerates
// from this parser's own output -- it can actually say whether a change made
// the parser better or worse.
//
// The anitomy and anitogo fixtures are committed with their licence files, as
// the bytes of the upstream commits that testdata/corpus/README.md names. The
// other data files of other projects are gitignored (*.txt, *.json), so the
// tests that read them skip when they are not present and the package still
// builds and tests without them.
//
// The samples and the probe sets are our own: names read by hand, with the
// value each field should have written down from what the name says, and not
// from what the parser printed. They are committed, and their tests always
// run.

const corpusDir = "testdata/corpus"

// labelledCase is one row of a labelled fixture. The field names are shared
// across anitomy, habari and anitogo, and the samples and the probe sets use
// the same names. Type and NoTag are fields of the samples and the probe sets
// alone.
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
	Type            json.RawMessage `json:"type"`
	NoTag           json.RawMessage `json:"no_tag"`
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

// values returns every value of a field, which is a string or a list of them,
// and whether the row has the field at all. In a sample a field that is there
// and empty says the name has none ('"release_group": ""'), where a field that
// is left out was not judged.
func values(raw json.RawMessage) ([]string, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return nil, true
		}
		return []string{s}, true
	}
	var v []string
	if json.Unmarshal(raw, &v) == nil {
		return v, true
	}
	return nil, false
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

// fixtureRes gives a fixture's resolution in the form the parser reports it:
// the height of a size ('1280x720' is '720p'), with the p a bare height
// leaves out.
func fixtureRes(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "×", "x"))
	if _, h, ok := strings.Cut(s, "x"); ok {
		s = h
	}
	if s != "" && strings.Trim(s, "0123456789") == "" {
		s += "p"
	}
	return s
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
	"anitomy.json": {"title": 65, "group": 92, "episode": 65, "episodeTitle": 69, "season": 90, "checksum": 99, "ext": 99, "resolution": 90, "version": 92},
	"habari.json":  {"title": 70, "group": 89, "episode": 65, "episodeTitle": 55, "season": 89, "checksum": 99, "ext": 99, "resolution": 93, "version": 71},
	"anitogo.json": {"title": 65, "group": 93, "episode": 64, "episodeTitle": 65, "season": 93, "checksum": 99, "ext": 99, "resolution": 90, "version": 92},
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
				// one label is one episode, and a list is the two ends of
				// the episodes a batch gives
				if v, _ := values(c.EpisodeNumber); len(v) != 0 && v[0] != "" {
					get("episode").add(slices.Equal(episodes(r), sampleNums(v)))
				}
				season := first(c.AnimeSeason)
				if season == "" {
					season = first(c.SeasonNumber)
				}
				if season != "" {
					n, _ := r.Numbering.Season()
					get("season").add(n == fixtureNum(season))
				}
				if s := first(c.FileChecksum); s != "" {
					get("checksum").add(strings.EqualFold(r.Sum, s))
				}
				if s := first(c.VideoResolution); s != "" {
					get("resolution").add(r.Resolution == fixtureRes(s))
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
			for _, k := range slices.Sorted(maps.Keys(m)) {
				v := *m[k]
				t.Logf("%-13s %4d/%-4d %5.1f%%", k, v.ok, v.total, v.pct())
				if floor, ok := floors[name][k]; ok && v.pct() < floor {
					t.Errorf("%s: %.1f%% is below the %.1f%% floor", k, v.pct(), floor)
				}
			}
		})
	}
}

// sampleFloors are the names each hand-read sample has right, per field, and
// in every labelled field at once ("names"). The samples are small and every
// row was read by hand, so the floor is the count itself and not a rounded
// percentage: a change that loses one name has to lower its floor and say why,
// and one that gains a name raises it.
var sampleFloors = map[string]map[string]int{
	"nyaa-400.json":       {"names": 331, "title": 347, "group": 392, "type": 310, "season": 398, "episode": 375},
	"nyaa-blind-140.json": {"names": 27, "title": 63, "group": 124, "type": 45, "season": 134, "episode": 101},
}

// episodes gives the episodes of r as a sample writes them: nothing for a
// release with none, one number for one episode, the two ends of a range.
// Half an episode keeps its fraction ('18.5', '1-11.5').
func episodes(r Release) []string {
	first, last, ranged := episodeRange(r.Numbering)
	if !ranged {
		episode, ok := r.Numbering.Episode()
		if !ok {
			return nil
		}
		first, last = episode, episode
	}
	// the fraction is that of the first Ep that writes the number, read from
	// the end of the name for the end of a range, so that 'S01E12 +
	// S01E12.5' is episode 12 and '1-11.5' ends at 11.5. It belongs to the
	// Last of an Ep, which is the episode itself when the Ep is no range
	frac := func(n int, end bool) string {
		eps := slices.Clone(r.Numbering.Episodes)
		if end {
			slices.Reverse(eps)
		}
		for _, e := range eps {
			ends := []int{e.First, e.Last}
			if end {
				slices.Reverse(ends)
			}
			for _, episode := range ends {
				if episode != n {
					continue
				}
				if e.Frac != 0 && n == e.Last {
					return "." + strconv.Itoa(int(e.Frac))
				}
				return ""
			}
		}
		return ""
	}
	if !ranged {
		return []string{strconv.Itoa(first) + frac(first, false)}
	}
	return []string{strconv.Itoa(first) + frac(first, false), strconv.Itoa(last) + frac(last, true)}
}

// sampleNums gives the numbers of a sample field without their zero padding,
// so that '04' and '4' are the same episode and '11.5' keeps its half.
func sampleNums(v []string) []string {
	out := make([]string, 0, len(v))
	for _, s := range v {
		whole, frac, _ := strings.Cut(s, ".")
		s = strconv.Itoa(fixtureNum(whole))
		if frac = strings.TrimRight(frac, "0"); frac != "" {
			s += "." + frac
		}
		out = append(out, s)
	}
	return out
}

// sameTitle reports whether two titles are the same words. The spaces are
// left out of the comparison, since a name that joins words with a colon or
// a slash reads either way ('Re:Zero' and 'Re Zero', 'Fate/Zero').
func sameTitle(a, b string) bool {
	return strings.ReplaceAll(MustNormalize(a), " ", "") == strings.ReplaceAll(MustNormalize(b), " ", "")
}

// anyOf reports whether got is one of the values a sample accepts for a
// field, without regard to case. A field with no value accepts an empty got
// alone, and a list may hold the empty string beside other values, for a
// name that may or may not give one ('[tags] WMD (1997)').
func anyOf(v []string, got string, same func(a, b string) bool) bool {
	if len(v) == 0 {
		return got == ""
	}
	return slices.ContainsFunc(v, func(s string) bool { return same(got, s) })
}

// scoreSample compares r with a row of a hand-read sample, and returns whether
// each field the row labels is right. A title is right when it is any of the
// titles the row accepts, since a name that gives two does not say which is
// the main one ('Tonari no Totoro / My Neighbor Totoro').
func scoreSample(c labelledCase, r Release) map[string]bool {
	m := make(map[string]bool, 5)
	if v, ok := values(c.AnimeTitle); ok {
		m["title"] = anyOf(v, r.Title, sameTitle)
	}
	if v, ok := values(c.ReleaseGroup); ok {
		m["group"] = anyOf(v, r.Group, strings.EqualFold)
	}
	if v, ok := values(c.Type); ok {
		m["type"] = anyOf(v, r.Type.String(), strings.EqualFold)
	}
	if v, ok := values(c.AnimeSeason); ok {
		// in rising order, as a sample lists them
		var got []string
		for _, n := range slices.Sorted(slices.Values(r.Numbering.Seasons)) {
			got = append(got, strconv.Itoa(n))
		}
		m["season"] = slices.Equal(got, sampleNums(v))
	}
	if v, ok := values(c.EpisodeNumber); ok {
		m["episode"] = slices.Equal(episodes(r), sampleNums(v))
	}
	return m
}

// TestSampleScoring checks the comparisons scoreSample makes, on the cases
// where a careless one would score a right reading wrong or a wrong one right.
func TestSampleScoring(t *testing.T) {
	for _, test := range []struct {
		name string
		exp  []string
	}{
		{"[Grp] Show - 04 [1080p]", []string{"4"}},
		{"[Grp] Show - 18.5 [1080p]", []string{"18.5"}},
		{"[Grp] Show - 00 [1080p]", []string{"0"}},
		{"[Grp] Show - 01 ~ 12 [1080p]", []string{"1", "12"}},
		{"[Grp] Show S01E12 + S01E12.5 [1080p]", []string{"12"}},
		{"Show.S01.1080p.WEB.H264-GRP", nil},
	} {
		if v := episodes(ParseString(test.name)); !slices.Equal(v, test.exp) {
			t.Errorf("%q: expected episodes %q, got: %q", test.name, test.exp, v)
		}
	}
	if v, exp := sampleNums([]string{"04", "18.50", "11.5", "000"}), []string{"4", "18.5", "11.5", "0"}; !slices.Equal(v, exp) {
		t.Errorf("expected numbers %q, got: %q", exp, v)
	}
	for _, test := range []struct {
		a, b string
		exp  bool
	}{
		{"Re:Zero", "Re Zero", true},
		{"Fate/Zero", "Fate Zero", true},
		{"Kirei ni Shite Moraemasu ka.", "kirei ni shite moraemasu ka", true},
		{"Bleach", "Bleach 225", false},
	} {
		if sameTitle(test.a, test.b) != test.exp {
			t.Errorf("sameTitle(%q, %q): expected %t", test.a, test.b, test.exp)
		}
	}
	for _, test := range []struct {
		v   []string
		got string
		exp bool
	}{
		{nil, "", true},
		{nil, "Dub", false},
		{[]string{"WMD", ""}, "", true},
		{[]string{"WMD", ""}, "wmd", true},
		{[]string{"WMD", ""}, "P38", false},
	} {
		if anyOf(test.v, test.got, strings.EqualFold) != test.exp {
			t.Errorf("anyOf(%q, %q): expected %t", test.v, test.got, test.exp)
		}
	}
	// a field that is there and empty has no value, one that is left out is
	// not judged, and a list keeps an empty string it holds
	for _, test := range []struct {
		raw string
		exp []string
		ok  bool
	}{
		{``, nil, false},
		{`""`, nil, true},
		{`"04"`, []string{"04"}, true},
		{`["WMD", ""]`, []string{"WMD", ""}, true},
	} {
		if v, ok := values(json.RawMessage(test.raw)); !slices.Equal(v, test.exp) || ok != test.ok {
			t.Errorf("values(%s): expected %q %t, got: %q %t", test.raw, test.exp, test.ok, v, ok)
		}
	}
}

// TestAccuracySample scores the parser against the hand-read samples of nyaa
// names. With -v it logs every name that has a field wrong, so that two runs
// can be compared name by name.
func TestAccuracySample(t *testing.T) {
	for _, name := range []string{"nyaa-400.json", "nyaa-blind-140.json"} {
		t.Run(name, func(t *testing.T) {
			buf, err := os.ReadFile(corpusDir + "/labelled/" + name)
			if err != nil {
				t.Fatalf("expected %s to be present: %v", name, err)
			}
			var cases []labelledCase
			if err := json.Unmarshal(buf, &cases); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			m := map[string]*tally{"names": {}}
			for _, c := range cases {
				score := scoreSample(c, ParseString(c.FileName))
				var wrong []string
				for _, k := range slices.Sorted(maps.Keys(score)) {
					if m[k] == nil {
						m[k] = &tally{}
					}
					m[k].add(score[k])
					if !score[k] {
						wrong = append(wrong, k)
					}
				}
				m["names"].add(len(wrong) == 0)
				if len(wrong) != 0 {
					t.Logf("wrong %s: %s", strings.Join(wrong, ","), c.FileName)
				}
			}
			for _, k := range slices.Sorted(maps.Keys(m)) {
				v, floor := *m[k], sampleFloors[name][k]
				t.Logf("%-8s %4d/%-4d %5.1f%%", k, v.ok, v.total, v.pct())
				switch {
				case v.ok < floor:
					t.Errorf("%s: %d right is below the floor of %d", k, v.ok, floor)
				case v.ok > floor:
					t.Errorf("%s: %d right is above the floor of %d: raise the floor in sampleFloors", k, v.ok, floor)
				}
			}
		})
	}
}

// probeFailures are the probes the parser gets wrong today. A probe that is
// not listed has to pass, and one that is listed has to fail: a change that
// fixes one takes it off the list, in the same diff.
var probeFailures = map[string][]string{
	"probes-title.json": {
		"Series.7.The.Contenders.2001.DVDRip.x264-GRP",
	},
	"probes-title-end.json": {
		"Pardon.My.French.2021.1080p.WEB.h264-GRP",
		"Excuse.My.French.2014.1080p.BluRay.x264-GRP",
		"The.Good.German.2006.1080p.BluRay.x264-GRP",
		"The.Quiet.Italian.2010.720p.BluRay.x264-GRP",
		"Little.Britain.USA.S01E01.720p.HDTV.x264-GRP",
	},
}

// vocabulary returns every value a row of the tag vocabulary gives.
func vocabulary() map[string]bool {
	m := make(map[string]bool)
	for _, infos := range taginfo.All() {
		for _, info := range infos {
			m[info.Tag()] = true
		}
	}
	return m
}

// tagValues returns the values r holds in the fields a vocabulary word can
// land in.
func tagValues(r Release) []string {
	v := slices.Concat(r.Language, r.Other, r.Cut, r.Edition, r.Codec, r.HDR, r.Audio)
	return append(v, r.Collection, r.Region, r.Source, r.Genre, r.Platform, r.Arch, r.Container)
}

// TestProbes checks the probe sets: scene names built by hand around a word of
// the vocabulary that is a word of the title in that name ('The.French.
// Dispatch.2021', 'Going.Dutch.S01E01'). A probe passes when the title and the
// type are those of the row and no field holds a value of its no_tag, which is
// what the word would be as a tag. The corpora are anime and sports names and
// cannot catch a rule that breaks a scene name, which is what these are for.
func TestProbes(t *testing.T) {
	for _, name := range []string{"probes-title.json", "probes-title-end.json"} {
		t.Run(name, func(t *testing.T) {
			buf, err := os.ReadFile(corpusDir + "/labelled/" + name)
			if err != nil {
				t.Fatalf("expected %s to be present: %v", name, err)
			}
			var cases []labelledCase
			if err := json.Unmarshal(buf, &cases); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			known, vocab := probeFailures[name], vocabulary()
			var passed int
			for _, c := range cases {
				r := ParseString(c.FileName)
				noTag, _ := values(c.NoTag)
				// a value the vocabulary does not have is in no field, and
				// would make the probe check nothing
				for _, s := range noTag {
					if !vocab[s] {
						t.Errorf("%q: no_tag %q is not a value of the vocabulary", c.FileName, s)
					}
				}
				got := tagValues(r)
				pass := r.Title == first(c.Title) && r.Type.String() == first(c.Type) &&
					!slices.ContainsFunc(noTag, func(s string) bool { return slices.Contains(got, s) })
				switch fails := slices.Contains(known, c.FileName); {
				case pass && fails:
					t.Errorf("%q passes now: take it out of probeFailures", c.FileName)
				case !pass && !fails:
					t.Errorf("%q\n  expected title %q, type %s, no tag of %q\n  got      title %q, type %s, tags %q",
						c.FileName, first(c.Title), first(c.Type), noTag, r.Title, r.Type, slices.DeleteFunc(got, func(s string) bool { return s == "" }))
				}
				if pass {
					passed++
				}
			}
			for _, s := range known {
				if !slices.ContainsFunc(cases, func(c labelledCase) bool { return c.FileName == s }) {
					t.Errorf("%q is in probeFailures and not in %s", s, name)
				}
			}
			t.Logf("%d of %d probes pass", passed, len(cases))
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
			keys := slices.SortedFunc(maps.Keys(typ), func(a, b string) int { return cmp.Compare(typ[b], typ[a]) })
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
// carry an episode number. Numbering.Episode alone cannot tell them apart,
// which is why Absolute exists.
func TestAbsoluteEpisode(t *testing.T) {
	for _, test := range []struct {
		s                         string
		series, episode, absolute int
		ok                        bool
		desc                      string
	}{
		{"[SubsPlease] One Piece - 1169 (1080p) [ABCD1234].mkv", 0, 1169, 1169, true, "bare number, no season: absolute"},
		{"[HorribleSubs] Detective Conan - 862 [1080p].mkv", 0, 862, 862, true, "same, three digits"},
		{"[Group] Show Title - 05 - Episode Name (1080p).mkv", 0, 5, 5, true, "bare number with an episode title"},
		{"One.Piece.E1100.1080p.WEB.h264-GRP", 0, 1100, 1100, true, "written with an E, no season: absolute"},
		{"[Group] One Piece EP1168 1080p WEB-DL AAC2.0 H.264", 0, 1168, 1168, true, "written with EP"},
		{"[Group] Show Title Episode 909 [1080p]", 0, 909, 909, true, "written out"},
		{"[Group] Show Title 2nd Season - 08 [1080p]", 2, 8, 0, false, "ordinal season qualifies it"},
		{"[Group] Show Title Second Season - 08 [1080p]", 2, 8, 0, false, "so does one written as a word"},
		{"[Group] Show Title E05-E07 [1080p]", 0, 5, 0, false, "the ends of a range between two tags are not one episode"},
		{"[Group] Show Title (E001-E130) (480p)", 0, 1, 0, false, "the same in a bracket"},
		{"[Group] Show Title E05~E07 [1080p]", 0, 5, 0, false, "and with a tilde"},
		{"[Group] Show Title E07-E05 [1080p]", 0, 7, 0, false, "a lower number after the hyphen is no range, and two episodes have no absolute number"},
		{"[Group] Show Title - 21.5 (720p) [4191AC97].mkv", 0, 21, 0, false, "half an episode has no absolute number"},
		{"[Group] Show Title - 01 ~ 12 [1080p][BATCH]", 0, 1, 0, false, "a batch is a range, not an episode, and starts at its first"},
		{"[SubsPlease] Hell Mode S2 - 04 (480p) [20BE14E8].mkv", 2, 4, 0, false, "season marker qualifies it: not absolute"},
		{"Show.Title.S01E05.1080p.WEB-DL-GRP", 1, 5, 0, false, "SxxExx is qualified"},
		{"Show.Title.S00E05.1080p.WEB-DL-GRP", 0, 5, 0, false, "S00Exx is a special, not an absolute number"},
		{"[Group] Show Title - 00 [1080p].mkv", 0, 0, 0, true, "a written episode 0 is absolute"},
		{"Show.Title.2024.03.11.1080p.WEB-DL-GRP", 0, 0, 0, false, "date based, no episode at all"},
	} {
		t.Run(test.desc, func(t *testing.T) {
			n := ParseString(test.s).Numbering
			series, _ := n.Season()
			episode, _ := n.Episode()
			absolute, ok := n.Absolute()
			if series != test.series || episode != test.episode || absolute != test.absolute || ok != test.ok {
				t.Errorf("%q\n  expected series=%d episode=%d absolute=%d ok=%t\n  got      series=%d episode=%d absolute=%d ok=%t",
					test.s, test.series, test.episode, test.absolute, test.ok, series, episode, absolute, ok)
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
	var n, abs, second int
	for _, line := range corpusNames(t, "nyaa.txt") {
		n++
		num := ParseString(line).Numbering
		absolute, ok := num.Absolute()
		if !ok {
			continue
		}
		// with no season the absolute episode is the one episode of the
		// name, which Episode gives. Beside a season it is a second
		// numbering, which is another number than the season's episode and
		// not inside a range the name writes ('S04E29-31v2')
		season, seasoned := num.Season()
		episode, _ := num.Episode()
		one := func(e Ep) bool { return e.First == absolute && e.Last == absolute && e.Frac == 0 }
		ranged := slices.ContainsFunc(num.Episodes, func(e Ep) bool {
			return e.First != e.Last && e.First <= absolute && absolute <= e.Last
		})
		switch {
		case !seasoned && absolute != episode:
			t.Fatalf("%q: Absolute %d does not match Episode %d", line, absolute, episode)
		case !seasoned && num.Shape() != ShapeEpisode:
			t.Fatalf("%q: Absolute %d of more than one episode, %v", line, absolute, num.Episodes)
		case !seasoned:
			abs++
		case season == 0:
			t.Fatalf("%q: Absolute %d set alongside the specials", line, absolute)
		case absolute == episode:
			t.Fatalf("%q: Absolute %d is the episode of season %d", line, absolute, season)
		case ranged:
			t.Fatalf("%q: Absolute %d is inside a range of episodes of season %d", line, absolute, season)
		case !slices.ContainsFunc(num.Alt, one):
			t.Fatalf("%q: Absolute %d is no second numbering of %v", line, absolute, num.Alt)
		default:
			second++
		}
	}
	t.Logf("%d of %d names carry an absolute episode number, %d of them beside a season and an episode", abs+second, n, second)
}
