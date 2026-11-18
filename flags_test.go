package rls

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/autobrr/rls/taginfo"
)

// TestFlagsVocabulary checks that each flag is the tag of an other row of
// taginfo.csv, so that a renamed row cannot leave a flag no parse gives.
func TestFlagsVocabulary(t *testing.T) {
	tags := make(map[string]bool)
	for _, info := range taginfo.All()["other"] {
		tags[info.Tag()] = true
	}
	for _, s := range slices.Concat(flagNames[:], []string{"REAL.PROPER", "REPACK", "REREPACK", "REPACK$1"}) {
		if !tags[s] {
			t.Errorf("expected an other row with the tag %q", s)
		}
	}
	if all := Flags(1<<len(flagNames) - 1); all != FlagProper|FlagReal|FlagRemux|FlagHybrid|FlagInternal|FlagReRip {
		t.Errorf("expected one spelling per constant, got the set %v", all)
	}
}

// TestReadOther checks the reading of each value of Other.
func TestReadOther(t *testing.T) {
	tests := []struct {
		other  []string
		flags  Flags
		repack int
	}{
		{nil, 0, 0},
		{[]string{"PROPER"}, FlagProper, 0},
		{[]string{"REAL.PROPER"}, FlagReal | FlagProper, 0},
		{[]string{"REAL"}, FlagReal, 0},
		{[]string{"REMUX"}, FlagRemux, 0},
		{[]string{"HYBRiD"}, FlagHybrid, 0},
		{[]string{"INTERNAL"}, FlagInternal, 0},
		{[]string{"RERiP"}, FlagReRip, 0},
		{[]string{"REPACK"}, 0, 1},
		{[]string{"REREPACK"}, 0, 2},
		{[]string{"REPACK2"}, 0, 2},
		{[]string{"REPACK3"}, 0, 3},
		{[]string{"REPACK10"}, 0, 10},
		{[]string{"REPACK99"}, 0, 99},
		{[]string{"REPACK", "REPACK3", "REREPACK"}, 0, 3},
		{[]string{"PROPER", "REPACK", "INTERNAL"}, FlagProper | FlagInternal, 1},
		{[]string{"REMUX", "HYBRiD", "REAL", "PROPER"}, FlagReal | FlagProper | FlagRemux | FlagHybrid, 0},
		// in any case, as a caller's row may spell them
		{[]string{"proper", "Hybrid", "repack"}, FlagProper | FlagHybrid, 1},
		{[]string{"Real.Proper", "remux", "iNTERNAL", "rerip"}, FlagReal | FlagProper | FlagRemux | FlagInternal | FlagReRip, 0},
		{[]string{"ReRePack"}, 0, 2},
		{[]string{"repack10"}, 0, 10},
		// values that are none of them
		{[]string{"REPACK1", "REPACK05", "REPACK100", "REPACK1x", "REPACK+5", "REPAC"}, 0, 0},
		{[]string{"REMASTERED", "READNFO", "PROPERLY", "REAL.PROPER.X", ""}, 0, 0},
	}
	for _, test := range tests {
		if flags, repack := readOther(test.other); flags != test.flags || repack != test.repack {
			t.Errorf("%q: expected %v and %d, got: %v and %d", test.other, test.flags, test.repack, flags, repack)
		}
	}
}

func TestFlags(t *testing.T) {
	tests := []struct {
		f    Flags
		strs []string
		s    string
	}{
		{0, nil, ""},
		{FlagProper, []string{"PROPER"}, "PROPER"},
		{FlagRemux | FlagProper, []string{"PROPER", "REMUX"}, "PROPER,REMUX"},
		{FlagReRip | FlagInternal | FlagHybrid | FlagReal, []string{"REAL", "HYBRiD", "INTERNAL", "RERiP"}, "REAL,HYBRiD,INTERNAL,RERiP"},
	}
	for _, test := range tests {
		if strs := test.f.Strings(); !slices.Equal(strs, test.strs) {
			t.Errorf("%06b: expected strings %q, got: %q", test.f, test.strs, strs)
		}
		if s := test.f.String(); s != test.s {
			t.Errorf("%06b: expected string %q, got: %q", test.f, test.s, s)
		}
		// the spellings are those of Other
		if f, _ := readOther(test.strs); f != test.f {
			t.Errorf("%q: expected to read back as %06b, got: %06b", test.strs, test.f, f)
		}
	}
	for _, test := range []struct {
		f, g Flags
		exp  bool
	}{
		{FlagProper | FlagReal, FlagProper, true},
		{FlagProper | FlagReal, FlagProper | FlagReal, true},
		{FlagProper, FlagProper | FlagReal, false},
		{FlagProper, FlagRemux, false},
		{FlagProper, 0, false},
		{0, 0, false},
	} {
		if ok := test.f.Has(test.g); ok != test.exp {
			t.Errorf("%v has %v: expected %t, got: %t", test.f, test.g, test.exp, ok)
		}
	}
}

// TestFlagsOutside checks that a bit outside the six flags is nothing to the
// set.
func TestFlagsOutside(t *testing.T) {
	const outside = Flags(0x8000)
	if strs := outside.Strings(); strs != nil {
		t.Errorf("expected nil strings, got: %#v", strs)
	}
	if strs := (outside | FlagRemux).Strings(); !slices.Equal(strs, []string{"REMUX"}) {
		t.Errorf("expected [REMUX], got: %q", strs)
	}
	if buf, err := json.Marshal(outside); err != nil || string(buf) != `[]` {
		t.Errorf("expected [], got: %s (%v)", buf, err)
	}
	if outside.Has(outside) || FlagProper.Has(outside) || !FlagProper.Has(FlagProper|outside) {
		t.Errorf("expected Has to read the flags of its argument alone")
	}
}

func TestFlagsJSON(t *testing.T) {
	tests := []struct {
		f   Flags
		exp string
	}{
		{0, `[]`},
		{FlagRemux | FlagProper, `["PROPER","REMUX"]`},
		{FlagHybrid, `["HYBRiD"]`},
	}
	for _, test := range tests {
		buf, err := json.Marshal(test.f)
		if err != nil {
			t.Fatalf("%v: expected no error, got: %v", test.f, err)
		}
		if string(buf) != test.exp {
			t.Errorf("%v: expected %s, got: %s", test.f, test.exp, buf)
		}
		f := FlagReRip
		if err := json.Unmarshal(buf, &f); err != nil || f != test.f {
			t.Errorf("%s: expected %v, got: %v (%v)", buf, test.f, f, err)
		}
	}
	var f Flags
	if err := json.Unmarshal([]byte(`["hybrid","PROPER","proper"]`), &f); err != nil || f != FlagHybrid|FlagProper {
		t.Errorf("expected any case and order to read, got: %v (%v)", f, err)
	}
	f = FlagReal | FlagRemux
	if err := json.Unmarshal([]byte(`null`), &f); err != nil || f != FlagReal|FlagRemux {
		t.Errorf("expected null to leave the set as it is, got: %v (%v)", f, err)
	}
	var v struct{ F Flags }
	v.F = FlagHybrid
	if err := json.Unmarshal([]byte(`{"F":null}`), &v); err != nil || v.F != FlagHybrid {
		t.Errorf("expected a null field to leave the set as it is, got: %v (%v)", v.F, err)
	}
	if err := json.Unmarshal([]byte(`["PROPER","REPACK"]`), &f); err == nil {
		t.Errorf("expected an error for a spelling that is no flag, got: %v", f)
	}
}

// TestFlagsRepack checks the flags and the repack of names, on each path of
// the build: a plain parse, one with a hint, and one of a name opened by
// flags in brackets, which are built apart from the rest.
func TestFlagsRepack(t *testing.T) {
	tests := []struct {
		s      string
		flags  Flags
		repack int
	}{
		{"Show.S01E02.1080p.WEB.h264-GRP", 0, 0},
		{"Show.S01E02.PROPER.1080p.WEB.h264-GRP", FlagProper, 0},
		{"Show.S01E02.REAL.PROPER.1080p.WEB.h264-GRP", FlagReal | FlagProper, 0},
		{"Show.S01E02.Real-Proper.1080p.WEB.h264-GRP", FlagReal | FlagProper, 0},
		{"Show.S01E02.REAL.1080p.WEB.h264-GRP", FlagReal, 0},
		{"Show.S01E02.proper.1080p.WEB.h264-GRP", FlagProper, 0},
		{"Show.S01E02.REPACK.1080p.WEB.h264-GRP", 0, 1},
		{"Show.S01E02.Repacked.1080p.WEB.h264-GRP", 0, 1},
		{"Show.S01E02.Re-Pack.1080p.WEB.h264-GRP", 0, 1},
		{"Show.S01E02.REREPACK.1080p.WEB.h264-GRP", 0, 2},
		{"Show.S01E02.REPACK2.1080p.WEB.h264-GRP", 0, 2},
		{"Show.S01E02.REPACK3.1080p.WEB.h264-GRP", 0, 3},
		{"Show.S01E02.repack9.1080p.WEB.h264-GRP", 0, 9},
		{"Show.S01E02.REPACK10.1080p.WEB.h264-GRP", 0, 10},
		{"Show.S01E02.1080p.WEB.REPACK25.h264-GRP", 0, 25},
		{"Show.S01E02.REPACK.REPACK3.1080p.WEB.h264-GRP", 0, 3},
		{"Show.S01E02.RERIP.1080p.WEB.h264-GRP", FlagReRip, 0},
		{"Show.S01E02.iNTERNAL.1080p.WEB.h264-GRP", FlagInternal, 0},
		{"Show.S01E02.PROPER.REPACK.1080p.WEB.h264-GRP", FlagProper, 1},
		{"Movie.2020.Hybrid.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-GRP", FlagRemux | FlagHybrid, 0},
		{"Movie.2020.2160p.UHD.BluRay.HEVC.TrueHD.7.1.Remux-GRP", FlagRemux, 0},
		{"Movie.2020.1080p.BDRemux.AVC.DTS-HD.MA.5.1-GRP", FlagRemux, 0},
		// a word of the title or the group is no flag
		{"Proper.Show.S01E02.1080p.WEB.h264-GRP", 0, 0},
		{"Show.S01E02.1080p.WEB.h264-PROPER", 0, 0},
		{"Movie.2020.1080p.BluRay.x264-HYBRiD", 0, 0},
		// a trailing bracket of the flag is the flag, not the site
		{"Movie (2020) (1080p BluRay x265 - GRP) [REPACK2]", 0, 2},
		{"Movie (2020) (1080p BluRay x265 - GRP) [Repack3]", 0, 3},
		{"Movie.2020.1080p.BluRay.x264-GRP[RE-PACK]", 0, 1},
		{"Movie.2020.1080p.BluRay.x264-GRP[REAL.PROPER]", FlagReal | FlagProper, 0},
		// flags in brackets that open the name are built apart from the rest
		{"[REPACK2] Movie.2020.1080p.BluRay.x264-GRP", 0, 2},
		{"[PROPER][Group] Show - 01 [1080p]", FlagProper, 0},
		{"[REPACK] [Group] Show - 01 [1080p] [REMUX]", FlagRemux, 1},
		{"[REPACK][PROPER] Movie.2020.1080p.BluRay.REPACK3.x264-GRP", FlagProper, 3},
	}
	for _, test := range tests {
		for _, r := range []Release{
			ParseString(test.s),
			ParseStringHint(test.s, Hint{}),
			ParseStringHint(test.s, Hint{Type: Movie}),
			Parse([]byte(test.s)),
		} {
			if r.Flags != test.flags || r.Repack != test.repack {
				t.Errorf("%q: expected %v and repack %d, got: %v and %d (other %q)", test.s, test.flags, test.repack, r.Flags, r.Repack, r.Other)
			}
		}
	}
}

// TestFlagsCallerRows checks a caller's other rows that spell a flag in
// another case: the flags are read from them, and a remux of no source is
// still taken from a BluRay.
func TestFlagsCallerRows(t *testing.T) {
	p := NewParser(WithTaginfo(map[string][]*taginfo.Taginfo{
		"other": {
			taginfo.Must("remux", "Remux", "rmx", "", "", ""),
			taginfo.Must("Proper", "Proper", "prp", "", "", ""),
			taginfo.Must("repack", "Repack", "rpk", "", "", ""),
		},
	}))
	tests := []struct {
		s      string
		other  []string
		flags  Flags
		repack int
		source string
	}{
		{"Movie.2020.1080p.rmx-GRP", []string{"remux"}, FlagRemux, 0, "BluRay"},
		{"Movie.2020.1080p.BluRay.prp-GRP", []string{"Proper"}, FlagProper, 0, "BluRay"},
		{"Movie.2020.1080p.BluRay.rpk-GRP", []string{"repack"}, 0, 1, "BluRay"},
	}
	for _, test := range tests {
		r := p.ParseRelease([]byte(test.s))
		if !slices.Equal(r.Other, test.other) || r.Flags != test.flags || r.Repack != test.repack || string(r.Source) != test.source {
			t.Errorf("%q: expected other %q, %v, repack %d, source %q, got: %q, %v, %d, %q", test.s, test.other, test.flags, test.repack, test.source, r.Other, r.Flags, r.Repack, r.Source)
		}
	}
}

// TestFlagsSetByParse checks that Flags and Repack are set by the parse and do
// not follow Other afterwards: a caller that changes Other, or builds a
// Release by hand, sets them itself.
func TestFlagsSetByParse(t *testing.T) {
	r := ParseString("Show.S01E02.PROPER.REPACK.1080p.WEB.h264-GRP")
	if r.Flags != FlagProper || r.Repack != 1 {
		t.Fatalf("expected the parse to give %v and repack 1, got: %v and %d", FlagProper, r.Flags, r.Repack)
	}
	r.Other = nil
	if r.Flags != FlagProper || r.Repack != 1 {
		t.Errorf("expected clearing Other to leave the flags, got: %v and %d", r.Flags, r.Repack)
	}
	r = ParseString("Show.S01E02.1080p.WEB.h264-GRP")
	r.Other = []string{"REMUX", "REPACK3"}
	if r.Flags != 0 || r.Repack != 0 {
		t.Errorf("expected setting Other to leave the flags, got: %v and %d", r.Flags, r.Repack)
	}
	hand := Release{Other: []string{"PROPER", "REPACK"}}
	if hand.Flags != 0 || hand.Repack != 0 {
		t.Errorf("expected a Release built by hand to have no flags it is not given, got: %v and %d", hand.Flags, hand.Repack)
	}
}

// otherFlagRe is the spelling of the rows of Other that a repack generation
// is read from.
var otherFlagRe = regexp.MustCompile(`(?i)^REPACK([2-9]|[1-9][0-9])$`)

// otherReading is what Flags and Repack must be for other, worked out apart
// from readOther.
func otherReading(other []string) (Flags, int) {
	var f Flags
	repack := 0
	for _, s := range other {
		u := strings.ToUpper(s)
		if u == "REAL.PROPER" {
			f |= FlagReal | FlagProper
		}
		if i := slices.IndexFunc(flagNames[:], func(name string) bool { return strings.ToUpper(name) == u }); i != -1 {
			f |= 1 << i
		}
		switch m := otherFlagRe.FindStringSubmatch(s); {
		case u == "REPACK":
			repack = max(repack, 1)
		case u == "REREPACK":
			repack = max(repack, 2)
		case m != nil:
			n, _ := strconv.Atoi(m[1])
			repack = max(repack, n)
		}
	}
	return f, repack
}

// TestFlagsInvariant checks that Flags and Repack are what Other gives, over
// the names of tests.yaml and the corpus when it is there, with and without a
// hint.
func TestFlagsInvariant(t *testing.T) {
	var names []string
	for _, test := range rlsTests(t) {
		names = append(names, test.s)
	}
	if !testing.Short() {
		for _, file := range []string{"nyaa.txt", "sports.txt"} {
			v, _ := readCorpus(file)
			names = append(names, v...)
		}
	}
	n := 0
	for _, name := range names {
		for _, r := range []Release{ParseString(name), ParseStringHint(name, Hint{Type: Series})} {
			flags, repack := otherReading(r.Other)
			if r.Flags != flags || r.Repack != repack {
				t.Errorf("%q: expected %v and repack %d from other %q, got: %v and %d", name, flags, repack, r.Other, r.Flags, r.Repack)
			}
			if r.Flags != 0 || r.Repack != 0 {
				n++
			}
		}
	}
	if n == 0 {
		t.Errorf("expected some of %d names to have flags", len(names))
	}
	t.Logf("%d names, %d parses with flags", len(names), n)
}
