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
// the same names. Type, NoTag and Batch are fields of the samples and the
// probe sets alone.
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
	AnimeType       json.RawMessage `json:"anime_type"`
	VolumeNumber    json.RawMessage `json:"volume_number"`
	Batch           bool            `json:"batch"`
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
	"anitomy.json": {"title": 82, "group": 94, "episode": 87, "episodeTitle": 76, "season": 90, "checksum": 99, "ext": 99, "resolution": 90, "version": 92},
	"habari.json":  {"title": 88, "group": 90, "episode": 90, "episodeTitle": 63, "season": 91, "checksum": 99, "ext": 99, "resolution": 93, "version": 71},
	"anitogo.json": {"title": 81, "group": 95, "episode": 86, "episodeTitle": 75, "season": 93, "checksum": 99, "ext": 99, "resolution": 90, "version": 92},
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
				// the episodes a batch gives. A row that names the kind of
				// an extra carries the number of the extra there ('OVA -
				// 01' is episode 01 of type OVA), as in the samples
				if v, _ := values(c.EpisodeNumber); len(v) != 0 && v[0] != "" {
					get("episode").add(sameEpisodes(c, r, sampleNums(v)))
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
	"nyaa-400.json":       {"names": 388, "title": 394, "group": 393, "type": 339, "season": 399, "episode": 399, "kind": 14, "volume": 4},
	"nyaa-blind-140.json": {"names": 121, "title": 135, "group": 126, "type": 88, "season": 137, "episode": 136, "kind": 35, "volume": 6},
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
		// a row that names the kind of an extra carries the number of the
		// extra in its episode
		m["episode"] = sameEpisodes(c, r, sampleNums(v))
	}
	if v, ok := values(c.AnimeType); ok {
		m["kind"] = hasKinds(r.Numbering, v)
	}
	if v, ok := values(c.VolumeNumber); ok {
		// of a range of volumes the first is the volume
		m["volume"] = len(v) != 0 && r.Numbering.Volume == fixtureNum(v[0])
	}
	return m
}

// sameEpisodes reports whether r gives the episodes want of the row c. They
// are the regular episodes of the name. Only a row that names a kind in
// anime_type is scored on the extras, and only when the name gives no regular
// episode: there the number of the row is that of the extra ('OVA - 01' is
// episode 01 of type OVA), and whether the kind is the right one is scored
// apart, see hasKinds. A row that names no kind is right with regular
// episodes alone, so episodes read as OVA 1 to 12 are not the episodes 1 to
// 12.
func sameEpisodes(c labelledCase, r Release, want []string) bool {
	regular := r
	regular.Numbering.Episodes = slices.DeleteFunc(slices.Clone(r.Numbering.Episodes), func(e Ep) bool { return e.Kind != EpRegular })
	got := episodes(regular)
	if kinds, _ := values(c.AnimeType); got == nil && len(kinds) != 0 && len(r.Numbering.Episodes) != 0 {
		return sameExtras(r.Numbering, want)
	}
	return slices.Equal(got, want)
}

// rowKind returns the kind that a name of anime_type or of
// Numbering.Contents counts as, in small letters: a creditless opening or
// ending is an opening or an ending, and an OAD or an ONA is an OVA, numbered
// or not. An episode has one kind for the three.
func rowKind(s string) string {
	switch s = strings.ToLower(s); s {
	case "ncop":
		return "op"
	case "nced":
		return "ed"
	case "oad", "ona":
		return "ova"
	}
	return s
}

// sameExtras reports whether the extras of n are those a row gives in its
// episode: none, one number, or the two ends of a range, which is every
// number from the one to the other. The name has to give exactly those
// numbers, of every kind it gives ('NCOP2+NCED2' is opening 2 and ending 2),
// and no regular episode. So the extras 1 and 3 are not the range 1 to 3,
// and the range is not the extras 1 and 3.
func sameExtras(n Numbering, want []string) bool {
	exp := map[string]bool{}
	switch len(want) {
	case 0:
	case 1:
		exp[want[0]] = true
	case 2:
		first, last := fixtureNum(want[0]), fixtureNum(want[1])
		if first >= last || last-first > maxRangeFill {
			return false
		}
		for k := first; k < last; k++ {
			exp[strconv.Itoa(k)] = true
		}
		// the end keeps its half ('1-11.5')
		exp[want[1]] = true
	default:
		return false
	}
	got := map[EpKind]map[string]bool{}
	for _, e := range n.Episodes {
		if e.Kind == EpRegular {
			return false
		}
		if got[e.Kind] == nil {
			got[e.Kind] = map[string]bool{}
		}
		for k := e.First; k <= e.Last && k-e.First <= maxRangeFill; k++ {
			s := strconv.Itoa(k)
			if k == e.Last && e.Frac != 0 {
				s += "." + strconv.Itoa(int(e.Frac))
			}
			got[e.Kind][s] = true
		}
	}
	for _, v := range got {
		if !maps.Equal(v, exp) {
			return false
		}
	}
	return len(got) != 0 || len(exp) == 0
}

// hasKinds reports whether n gives the kinds that a sample row names in
// anime_type, and no other: as the Kind of an episode when an extra is
// numbered ('OVA 02'), and in Contents when it is not ('Title - OVA'). A
// reading that gives a kind beside those of the row is as wrong as one that
// misses a kind ('OVA + Movie' against a row that says OVA). The kinds are
// compared as rowKind gives them.
func hasKinds(n Numbering, want []string) bool {
	got, exp := map[string]bool{}, map[string]bool{}
	for _, e := range n.Episodes {
		if e.Kind != EpRegular {
			got[rowKind(e.Kind.String())] = true
		}
	}
	for _, s := range n.Contents {
		got[rowKind(s)] = true
	}
	for _, s := range want {
		exp[rowKind(s)] = true
	}
	return len(exp) != 0 && maps.Equal(got, exp)
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
	// the number and the kind of an extra, and the volume: a row that names
	// a kind is scored on the extra, and one that names none on the regular
	// episodes alone
	for _, test := range []struct {
		name, row string
		exp       map[string]bool
	}{
		{"[Grp] Show - OVA 02 [1080p]", `{"episode_number": "02", "anime_type": "OVA"}`, map[string]bool{"episode": true, "kind": true}},
		{"[Grp] Show - OVA 02 [1080p]", `{"episode_number": "02", "anime_type": "Special"}`, map[string]bool{"episode": true, "kind": false}},
		{"[Grp] Show - OVA [1080p]", `{"episode_number": "", "anime_type": "OVA"}`, map[string]bool{"episode": true, "kind": true}},
		{"[Grp] Show NCOP2+NCED2 [1080p]", `{"episode_number": "2", "anime_type": ["NCOP", "NCED"]}`, map[string]bool{"episode": true, "kind": true}},
		{"[Grp] Show - Specials - 01 - 03 [1080p]", `{"episode_number": ["01", "03"], "anime_type": "Special"}`, map[string]bool{"episode": true, "kind": true}},
		{"[Grp] Show - SP01-SP03 [1080p]", `{"episode_number": ["01", "03"], "anime_type": "Special"}`, map[string]bool{"episode": true, "kind": true}},
		// two extras are not the range from the one to the other, and a
		// range is not its two ends
		{"[Grp] Show SP1&3 [1080p]", `{"episode_number": ["1", "3"], "anime_type": "Special"}`, map[string]bool{"episode": false, "kind": true}},
		{"[Grp] Show SP1&2 [1080p]", `{"episode_number": ["1", "2"], "anime_type": "Special"}`, map[string]bool{"episode": true, "kind": true}},
		{"[Grp] Show - SP01-SP03 [1080p]", `{"episode_number": "01", "anime_type": "Special"}`, map[string]bool{"episode": false, "kind": true}},
		{"[Grp] Show - SP01 [1080p]", `{"episode_number": ["01", "03"], "anime_type": "Special"}`, map[string]bool{"episode": false, "kind": true}},
		{"[Grp] Show NCOP2+NCED3 [1080p]", `{"episode_number": "2", "anime_type": ["NCOP", "NCED"]}`, map[string]bool{"episode": false, "kind": true}},
		// a kind beside the ones the row names is a wrong reading, and so
		// are extras of a kind the row does not name, and extras on a row
		// that names no kind
		{"[Grp] Show - OVA + NCED [1080p]", `{"episode_number": "", "anime_type": "OVA"}`, map[string]bool{"episode": true, "kind": false}},
		{"[Grp] Show - OVA + NCED [1080p]", `{"episode_number": "", "anime_type": ["OVA", "NCED"]}`, map[string]bool{"episode": true, "kind": true}},
		{"[Grp] Show - OVA 01-12 [1080p]", `{"episode_number": ["01", "12"], "anime_type": "Special"}`, map[string]bool{"episode": true, "kind": false}},
		{"[Grp] Show - OVA 01-12 [1080p]", `{"episode_number": ["01", "12"], "anime_type": ""}`, map[string]bool{"episode": false, "kind": false}},
		{"[Grp] Show - OVA 01-12 [1080p]", `{"episode_number": ["01", "12"]}`, map[string]bool{"episode": false}},
		{"[Grp] Show - OVA 01-12 [1080p]", `{"episode_number": ["01", "12"], "anime_type": "OVA"}`, map[string]bool{"episode": true, "kind": true}},
		// an OAD and an ONA are an OVA, with a number and without one
		{"[Grp] Show - OAD 02 [1080p]", `{"episode_number": "02", "anime_type": "OVA"}`, map[string]bool{"episode": true, "kind": true}},
		{"[Grp] Show - OAD 02 [1080p]", `{"episode_number": "02", "anime_type": "OAD"}`, map[string]bool{"episode": true, "kind": true}},
		{"[Grp] Show - ONA [1080p]", `{"episode_number": "", "anime_type": "OVA"}`, map[string]bool{"episode": true, "kind": true}},
		{"[Grp] Show - ONA [1080p]", `{"episode_number": "", "anime_type": "ONA"}`, map[string]bool{"episode": true, "kind": true}},
		{"[Grp] Show II + SP01 + SP02 [720p]", `{"episode_number": ""}`, map[string]bool{"episode": true}},
		{"[Grp] Show S1 01-13 + SP01-03 [720p]", `{"episode_number": ["1", "13"]}`, map[string]bool{"episode": true}},
		{"[Grp] Show - 05 [1080p]", `{"episode_number": "05", "anime_type": "OVA"}`, map[string]bool{"episode": true, "kind": false}},
		{"[Grp] Show - Vol.06 [BD 1080p]", `{"volume_number": "06"}`, map[string]bool{"volume": true}},
		{"[Grp] Show (Vol.1-5) [BD 1080p]", `{"volume_number": ["1", "5"]}`, map[string]bool{"volume": true}},
		{"[Grp] Show - Vol.06 [BD 1080p]", `{"volume_number": "07"}`, map[string]bool{"volume": false}},
	} {
		var c labelledCase
		if err := json.Unmarshal([]byte(test.row), &c); err != nil {
			t.Fatal(err)
		}
		if m := scoreSample(c, ParseString(test.name)); !maps.Equal(m, test.exp) {
			t.Errorf("%q scored against %s: expected %v, got: %v", test.name, test.row, test.exp, m)
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

// probeFailures are the probes the parser gets wrong today, each with the
// reading it gives, as probeReading writes it. A probe that is not listed has
// to pass, and one that is listed has to fail with that reading: a change
// that fixes one takes it off the list, and one that reads it in another
// wrong way writes the new reading, in the same diff.
var probeFailures = map[string]map[string]string{
	"probes-title.json": {
		"Series.7.The.Contenders.2001.DVDRip.x264-GRP": `series "The Contenders", group "GRP", seasons [7]`,
	},
	"probes-title-end.json": {
		"Pardon.My.French.2021.1080p.WEB.h264-GRP":     `movie "Pardon My", tag FRENCH, group "GRP"`,
		"Excuse.My.French.2014.1080p.BluRay.x264-GRP":  `movie "Excuse My", tag FRENCH, group "GRP"`,
		"The.Good.German.2006.1080p.BluRay.x264-GRP":   `movie "The Good", tag GERMAN, group "GRP"`,
		"The.Quiet.Italian.2010.720p.BluRay.x264-GRP":  `movie "The Quiet", tag iTALiAN, group "GRP"`,
		"Little.Britain.USA.S01E01.720p.HDTV.x264-GRP": `episode "Little Britain", tag USA, group "GRP", seasons [1], episode 1`,
	},
	"probes-brackets.json": {
		"Doctor Who (2005) S13 Special - Eve of the Daleks (1080p BluRay x265) [QxR]":                             `series "Doctor Who", group "QxR", seasons [13]`,
		"Cowboy Bebop (1998) Season 1 S01 + Movie + Extras (1080p BluRay x265 HEVC 10bit AAC 5.1 Garshasp) [QxR]": `series "Cowboy Bebop", group "Garshasp", seasons [1], contents ["Movie"]`,
		"Doctor Who - The Star Beast (Special) [1080p] [Group]":                                                   `movie "Doctor Who - The Star Beast", group "Group", contents ["Special"]`,
		"Top Gear Special - Patagonia (2014) [1080p]":                                                             `movie "Top Gear Special - Patagonia"`,
		"Planet Earth II Extra (2016) [1080p]":                                                                    `movie "Planet Earth II Extra"`,
		"Persona 5 OST Vol.1 [2017] [FLAC]":                                                                       `music "Persona 5 OST Vol 1"`,
		"Daft Punk - Discovery (2001) [FLAC] [Batch]":                                                             `music "Discovery", artist "Daft Punk", group "Batch"`,
		"Microsoft Office 2021 Pro Plus (x64) [Batch]":                                                            `app "Microsoft Office 2021 Pro Plus", group "Batch"`,
		"Sonic Mania OST [FLAC] [Batch]":                                                                          `music "Sonic Mania", group "Batch"`,
		"The Beatles - Box Set (2009) [FLAC] [Batch]":                                                             `music "Box Set", artist "The Beatles", group "Batch"`,
		"James Bond 007 - Complete Collection (1962-2021)":                                                        `music "Complete Collection", artist "James Bond 007"`,
		"[TGx] Dante 01 1080p WEB-DL x264":                                                                        `episode "Dante", group "TGx", episode 1`,
	},
}

// probeReading writes what the parser reads in the probe c, in the fields a
// probe is judged on and the group: the type, the title, the tags that are
// values of no_tag, and the numbering.
func probeReading(c labelledCase, r Release) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %q", r.Type, r.Title)
	noTag, _ := values(c.NoTag)
	for _, s := range tagValues(r) {
		if s != "" && slices.Contains(noTag, s) {
			fmt.Fprintf(&sb, ", tag %s", s)
		}
	}
	if r.Artist != "" {
		fmt.Fprintf(&sb, ", artist %q", r.Artist)
	}
	if r.Group != "" {
		fmt.Fprintf(&sb, ", group %q", r.Group)
	}
	n := r.Numbering
	if len(n.Seasons) != 0 {
		fmt.Fprintf(&sb, ", seasons %d", n.Seasons)
	}
	for _, e := range n.Episodes {
		kind := "episode"
		if e.Kind != EpRegular {
			kind = e.Kind.String()
		}
		if fmt.Fprintf(&sb, ", %s %d", kind, e.First); e.Last != e.First {
			fmt.Fprintf(&sb, "-%d", e.Last)
		}
	}
	if len(n.Contents) != 0 {
		fmt.Fprintf(&sb, ", contents %q", n.Contents)
	}
	if n.Volume != 0 {
		fmt.Fprintf(&sb, ", volume %d", n.Volume)
	}
	if n.Batch {
		sb.WriteString(", batch")
	}
	if len(r.Alt) != 0 {
		fmt.Fprintf(&sb, ", other titles %q", r.Alt)
	}
	return sb.String()
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
	v := slices.Concat(r.Language, r.Other, r.Cut, r.Edition, r.Codec, r.DynamicRange.Strings(), r.Audio)
	return append(v, r.Collection, r.Region, r.Source, r.Genre, r.Platform, r.Arch, r.Container)
}

// sameUnits reports whether r holds what a probe says its name holds, and
// nothing more: the contents and the kinds of its extras in anime_type, the
// number of an extra in episode_number, the volume, and whether it is a
// batch. A probe with none of these fields is a name the structure stage
// reads nothing in, and no probe has another title.
func sameUnits(c labelledCase, r Release) bool {
	n := r.Numbering
	var got, want []string
	extras := false
	for _, e := range n.Episodes {
		if e.Kind != EpRegular {
			got, extras = append(got, e.Kind.String()), true
		}
	}
	for _, s := range n.Contents {
		got = append(got, strings.ToLower(s))
	}
	v, _ := values(c.AnimeType)
	for _, s := range v {
		want = append(want, strings.ToLower(s))
	}
	slices.Sort(got)
	slices.Sort(want)
	volume := 0
	if v, _ := values(c.VolumeNumber); len(v) != 0 {
		volume = fixtureNum(v[0])
	}
	number, _ := values(c.EpisodeNumber)
	return slices.Equal(slices.Compact(got), want) && n.Volume == volume && n.Batch == c.Batch && len(r.Alt) == 0 &&
		(!extras || sameExtras(n, sampleNums(number)))
}

// TestProbes checks the probe sets: names built by hand, or real ones, around
// a word that the name does not mean the way a rule would read it. In the two
// sets of scene names it is a word of the vocabulary that is a word of the
// title ('The.French.Dispatch.2021', 'Going.Dutch.S01E01'). In the set of
// bracketed names it is what the structure stage reads in a fansub name, in
// the name of a film, a show, a record, a book or a game that no fansub group
// wrote ('Kill Bill: Vol. 1 (2003) [BluRay] [1080p] [YTS.AM]', 'The Specials
// (2019) [720p]', '[FLAC] Artist - The Complete Series Box (2015)').
//
// A probe passes when the title and the type are those of the row, no field
// holds a value of its no_tag, which is what the word would be as a tag, and
// the release holds the units of the row and no other, see sameUnits. A row
// gives no type when the name does not say what it is ('Show Name (2020) OVA
// [1080p]' is one film or a set of episodes), and is then not judged on it.
// The corpora are anime and sports names and cannot catch a rule that breaks
// another kind of name, which is what these are for.
func TestProbes(t *testing.T) {
	for _, name := range []string{"probes-title.json", "probes-title-end.json", "probes-brackets.json"} {
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
				pass := r.Title == first(c.Title) && (len(c.Type) == 0 || r.Type.String() == first(c.Type)) && sameUnits(c, r) &&
					!slices.ContainsFunc(noTag, func(s string) bool { return slices.Contains(got, s) })
				switch reading, fails := known[c.FileName]; {
				case pass && fails:
					t.Errorf("%q passes now: take it out of probeFailures", c.FileName)
				case !pass && !fails:
					t.Errorf("%q\n  expected title %q, type %s, no tag of %q, units %s %s %s, batch %t\n  got      title %q, type %s, tags %q, numbering %+v, other titles %q",
						c.FileName, first(c.Title), first(c.Type), noTag, c.AnimeType, c.EpisodeNumber, c.VolumeNumber, c.Batch,
						r.Title, r.Type, slices.DeleteFunc(got, func(s string) bool { return s == "" }), r.Numbering, r.Alt)
				case !pass && reading != probeReading(c, r):
					t.Errorf("%q is read in another wrong way now: write the reading in probeFailures\n  expected: %s\n  got:      %s", c.FileName, reading, probeReading(c, r))
				}
				if pass {
					passed++
				}
			}
			for s := range known {
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
				if r.DynamicRange != 0 {
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
