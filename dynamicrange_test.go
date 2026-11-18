package rls

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/autobrr/rls/taginfo"
)

// TestDynamicRangeVocabulary checks that the formats are the tags of the hdr
// rows of taginfo.csv, one row each.
func TestDynamicRangeVocabulary(t *testing.T) {
	var tags []string
	for _, info := range taginfo.All()["hdr"] {
		tags = append(tags, info.Tag())
	}
	slices.Sort(tags)
	names := slices.Sorted(slices.Values(dynamicRangeNames[:]))
	if !slices.Equal(tags, names) {
		t.Errorf("expected the hdr rows to be %q, got: %q", names, tags)
	}
	if all := DynamicRange(1<<len(dynamicRangeNames) - 1); all != DV|HDR|HDRPlus|HDR10|HDR10Plus|HLG|SDR {
		t.Errorf("expected one spelling per constant, got the set %v", all)
	}
}

func TestDynamicRange(t *testing.T) {
	tests := []struct {
		d    DynamicRange
		strs []string
		s    string
	}{
		{0, nil, ""},
		{DV, []string{"DV"}, "DV"},
		{HDR10 | DV, []string{"DV", "HDR10"}, "DV,HDR10"},
		{HDR10Plus | HDR10 | HDR | DV, []string{"DV", "HDR", "HDR10", "HDR10+"}, "DV,HDR,HDR10,HDR10+"},
		{SDR | HLG | HDRPlus, []string{"HDR+", "HLG", "SDR"}, "HDR+,HLG,SDR"},
	}
	for _, test := range tests {
		if strs := test.d.Strings(); !slices.Equal(strs, test.strs) {
			t.Errorf("%08b: expected strings %q, got: %q", test.d, test.strs, strs)
		}
		if s := test.d.String(); s != test.s {
			t.Errorf("%08b: expected string %q, got: %q", test.d, test.s, s)
		}
		var d DynamicRange
		for _, s := range test.strs {
			d |= ParseDynamicRange(s)
		}
		if d != test.d {
			t.Errorf("%q: expected to parse back to %08b, got: %08b", test.strs, test.d, d)
		}
	}
}

func TestParseDynamicRange(t *testing.T) {
	tests := []struct {
		s   string
		exp DynamicRange
	}{
		{"DV", DV},
		{"dv", DV},
		{"HDR", HDR},
		{"hdr+", HDRPlus},
		{"HDR10", HDR10},
		{"Hdr10+", HDR10Plus},
		{"HLG", HLG},
		{"sdr", SDR},
		{"", 0},
		{"DoVi", 0},
		{"HDR10Plus", 0},
		{"DV,HDR10", 0},
		{"Dolby Vision", 0},
	}
	for _, test := range tests {
		if d := ParseDynamicRange(test.s); d != test.exp {
			t.Errorf("%q: expected %08b, got: %08b", test.s, test.exp, d)
		}
	}
}

// TestDynamicRangeOutside checks that a bit outside the seven formats is
// nothing to the set.
func TestDynamicRangeOutside(t *testing.T) {
	const outside = DynamicRange(0x80)
	if strs := outside.Strings(); strs != nil {
		t.Errorf("expected nil strings, got: %#v", strs)
	}
	if s := outside.String(); s != "" {
		t.Errorf("expected an empty string, got: %q", s)
	}
	if strs := (outside | DV).Strings(); !slices.Equal(strs, []string{"DV"}) {
		t.Errorf("expected [DV], got: %q", strs)
	}
	if buf, err := json.Marshal(outside | HDR10); err != nil || string(buf) != `["HDR10"]` {
		t.Errorf("expected [\"HDR10\"], got: %s (%v)", buf, err)
	}
	if buf, err := json.Marshal(outside); err != nil || string(buf) != `[]` {
		t.Errorf("expected [], got: %s (%v)", buf, err)
	}
	if outside.Has(outside) || DV.Has(outside) || (DV | outside).Has(outside) {
		t.Errorf("expected no set to have a bit outside the formats")
	}
	if !DV.Has(DV|outside) || (outside | HDR).Has(DV|outside) {
		t.Errorf("expected Has to read the formats of its argument alone")
	}
}

func TestDynamicRangeHas(t *testing.T) {
	tests := []struct {
		d, f DynamicRange
		exp  bool
	}{
		{DV | HDR10, DV, true},
		{DV | HDR10, DV | HDR10, true},
		{DV | HDR10, DV | HDR, false},
		{DV | HDR10, HDR10Plus, false},
		{DV | HDR10, 0, false},
		{0, 0, false},
	}
	for _, test := range tests {
		if ok := test.d.Has(test.f); ok != test.exp {
			t.Errorf("%v has %v: expected %t, got: %t", test.d, test.f, test.exp, ok)
		}
	}
}

func TestDynamicRangeJSON(t *testing.T) {
	tests := []struct {
		d   DynamicRange
		exp string
	}{
		{0, `[]`},
		{HDR10Plus | DV, `["DV","HDR10+"]`},
		{SDR, `["SDR"]`},
	}
	for _, test := range tests {
		buf, err := json.Marshal(test.d)
		if err != nil {
			t.Fatalf("%v: expected no error, got: %v", test.d, err)
		}
		if string(buf) != test.exp {
			t.Errorf("%v: expected %s, got: %s", test.d, test.exp, buf)
		}
		var d DynamicRange
		if err := json.Unmarshal(buf, &d); err != nil || d != test.d {
			t.Errorf("%s: expected %v, got: %v (%v)", buf, test.d, d, err)
		}
	}
	var d DynamicRange
	if err := json.Unmarshal([]byte(`["hdr10+","dv","DV"]`), &d); err != nil || d != DV|HDR10Plus {
		t.Errorf("expected any case and order to read, got: %v (%v)", d, err)
	}
	d = DV | HLG
	if err := json.Unmarshal([]byte(`null`), &d); err != nil || d != DV|HLG {
		t.Errorf("expected null to leave the set as it is, got: %v (%v)", d, err)
	}
	var v struct{ D DynamicRange }
	v.D = SDR
	if err := json.Unmarshal([]byte(`{"D":null}`), &v); err != nil || v.D != SDR {
		t.Errorf("expected a null field to leave the set as it is, got: %v (%v)", v.D, err)
	}
	if err := json.Unmarshal([]byte(`["DV","Dolby Vision"]`), &d); err == nil {
		t.Errorf("expected an error for a spelling that is no format, got: %v", d)
	}
	var r struct{ DynamicRange DynamicRange }
	r.DynamicRange = DV | HDR
	if buf, _ := json.Marshal(r); string(buf) != `{"DynamicRange":["DV","HDR"]}` {
		t.Errorf("expected the field to marshal as a list, got: %s", buf)
	}
}

// TestDynamicRangeOrder checks that the set gives its formats in one order
// whatever order the name writes them in.
func TestDynamicRangeOrder(t *testing.T) {
	for _, s := range []string{
		"Movie.2020.2160p.WEB-DL.DV.HDR10.HEVC-GRP",
		"Movie.2020.2160p.WEB-DL.HDR10.DV.HEVC-GRP",
		"Movie.2020.2160p.WEB-DL.DoVi.HDR10.HEVC-GRP",
	} {
		r := ParseString(s)
		if r.DynamicRange != DV|HDR10 {
			t.Errorf("%q: expected %v, got: %v", s, DV|HDR10, r.DynamicRange)
		}
		if exp := []string{"DV", "HDR10"}; !slices.Equal(r.DynamicRange.Strings(), exp) {
			t.Errorf("%q: expected %q, got: %q", s, exp, r.DynamicRange.Strings())
		}
	}
}

// TestDynamicRangeCallerRows checks a caller's hdr rows: one whose tag is a
// format in another case counts as that format, and one whose tag is no format
// is in no format while its tag stays a hdr tag.
func TestDynamicRangeCallerRows(t *testing.T) {
	p := NewParser(WithTaginfo(map[string][]*taginfo.Taginfo{
		"hdr": {
			taginfo.Must("dv", "Dolby Vision", "dvx", "", "movie", ""),
			taginfo.Must("XDR", "Extended Dynamic Range", "", "", "movie", ""),
		},
	}))
	tests := []struct {
		s   string
		exp DynamicRange
		hdr string
	}{
		{"Movie.2020.2160p.WEB-DL.DVX.HDR10.HEVC-GRP", DV | HDR10, "DVX"},
		{"Movie.2020.2160p.WEB-DL.XDR.HDR10.HEVC-GRP", HDR10, "XDR"},
	}
	for _, test := range tests {
		r := p.ParseRelease([]byte(test.s))
		if r.DynamicRange != test.exp {
			t.Errorf("%q: expected %v, got: %v", test.s, test.exp, r.DynamicRange)
		}
		if !slices.ContainsFunc(r.Tags(), func(tag Tag) bool { return tag.Is(TagTypeHDR) && tag.Text() == test.hdr }) {
			t.Errorf("%q: expected %q to stay a hdr tag, got: %e", test.s, test.hdr, r)
		}
	}
}

// TestDynamicRangeUnset checks that a hdr tag of another release type given
// back to the text clears its format only when no hdr tag left gives it. Two
// tags give one format from two rows only when a row spells it in another
// case, as the row of a tag is the first that matches its spelling: here
// 'Dolby' is a game-only 'dv' that the case-sensitive 'DV' row does not match.
func TestDynamicRangeUnset(t *testing.T) {
	infos := taginfo.All()
	infos["hdr"] = []*taginfo.Taginfo{
		taginfo.Must("DV", "Dolby Vision", "(?-i:DV)|dovi", "", "movie", ""),
		taginfo.Must("dv", "Dolby Vision", "dolby|(?-i:dv)", "", "game", "1"),
		taginfo.Must("HDR10", "High Dynamic Range (10-bit)", "", "", "movie", ""),
		taginfo.Must("XDR", "Extended Dynamic Range", "", "", "game", "1"),
	}
	p := NewTagParser(infos, DefaultLexers()...).(*TagParser)
	tests := []struct {
		s    string
		exp  DynamicRange
		text []string
	}{
		{"Movie.2020.2160p.WEB-DL.Dolby.HDR10.HEVC-GRP", HDR10, []string{"Dolby"}},
		{"Movie.2020.2160p.WEB-DL.Dolby.DV.HDR10.HEVC-GRP", DV | HDR10, []string{"Dolby"}},
		{"Movie.2020.2160p.WEB-DL.DoVi.Dolby.HDR10.HEVC-GRP", DV | HDR10, []string{"Dolby"}},
		{"Movie.2020.2160p.WEB-DL.Dolby.HDR10.Dolby.HEVC-GRP", HDR10, []string{"Dolby", "Dolby"}},
		{"Movie.2020.2160p.WEB-DL.XDR.HDR10.HEVC-GRP", HDR10, []string{"XDR"}},
	}
	for _, test := range tests {
		r := p.ParseHint([]byte(test.s), Hint{Type: Movie})
		if r.DynamicRange != test.exp {
			t.Errorf("%q: expected %v, got: %v", test.s, test.exp, r.DynamicRange)
		}
		var text []string
		for _, tag := range r.Tags() {
			if tag.Is(TagTypeText) && slices.Contains(test.text, tag.Text()) {
				text = append(text, tag.Text())
			}
		}
		if !slices.Equal(text, test.text) {
			t.Errorf("%q: expected %q given back as text, got: %q", test.s, test.text, text)
		}
	}
}
