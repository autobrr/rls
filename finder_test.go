package rls

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/autobrr/rls/taginfo"
)

// scanFinder returns a finder that confirms every row with its regexp and
// has no rows kept for the second lookup, for a test to compare with one
// that compares bytes.
func scanFinder(typ TagType, infos ...*taginfo.Taginfo) *finder {
	noSpellings = true
	defer func() { noSpellings = false }()
	return newFinder(typ, infos...)
}

// finderStrings returns strings to look up in the rows: every spelling of
// every row as it is written, in lower and in upper case, with a byte
// changed, cut short, and with the runes outside ASCII that a letter folds
// to.
func finderStrings(infos []*taginfo.Taginfo) []string {
	var v []string
	for _, info := range infos {
		exp, err := expandRE(info.RE(), true)
		if err != nil {
			exp = nil
		}
		for _, s := range append(exp, info.Tag()) {
			v = append(v, s, strings.ToLower(s), strings.ToUpper(s), s+"x", "x"+s, s+" ", strings.ReplaceAll(s, ".", "-"))
			if s != "" {
				v = append(v, s[1:], s[:len(s)-1], s[:len(s)/2]+"_"+s[len(s)/2:], s[:len(s)-1]+"é")
			}
			for _, fold := range [][2]string{{"s", "\u017f"}, {"S", "\u017f"}, {"k", "\u212a"}, {"K", "\u212a"}} {
				if strings.Contains(s, fold[0]) {
					v = append(v, strings.Replace(s, fold[0], fold[1], 1))
				}
			}
		}
	}
	return append(v, "", " ", "é", "\xff", strings.Repeat("a", 60), strings.Repeat("WEB-DL", 10))
}

// finderTypes are the tag types by the name their rows are loaded under.
func finderTypes() map[string]TagType {
	m := make(map[string]TagType)
	for typ := TagTypeWhitespace; typ <= TagTypeFrameRate; typ++ {
		m[strings.ToLower(typ.String())] = typ
	}
	return m
}

// checkFinder checks that the finder for infos finds the rows a finder
// without the spellings finds, for a string alone and for a tag of the type.
func checkFinder(t *testing.T, name string, typ TagType, infos []*taginfo.Taginfo, strs []string) {
	t.Helper()
	fast, slow := newFinder(typ, infos...), scanFinder(typ, infos...)
	tag := func(info *taginfo.Taginfo) string {
		if info == nil {
			return "<nil>"
		}
		return info.Tag()
	}
	// twice, so that the rows kept after the first lookup are read as well
	others := []TagType{TagTypeText, TagTypeChannels, TagTypeOther, TagTypeSize}
	for pass := range 2 {
		for k, s := range strs {
			if info, exp := fast.find(s), slow.find(s); info != exp {
				t.Errorf("%s: find(%q): expected %s, got: %s", name, s, tag(exp), tag(info))
			}
			// as the lexers make a tag, as the builder retypes one, and as a
			// caller makes one with the find func
			a := newTag(nil, nil, typ, fast, []byte(s), []byte(s))
			b := Tag{typ: TagTypeText, v: a.v}.as(typ, fast)
			exp := NewTag(typ, slow.fn, []byte(s), []byte(s))
			for _, got := range []Tag{a, b} {
				if got.ninfo != exp.ninfo || got.info != exp.info || got.Normalize() != exp.Normalize() {
					t.Errorf("%s: tag of %q: expected rows %s and %s, got: %s and %s", name, s, tag(exp.ninfo), tag(exp.info), tag(got.ninfo), tag(got.info))
				}
			}
			// a tag of another type with the rows of this one
			if other := others[(k+pass)%len(others)]; other != typ {
				got, exp := newTag(nil, nil, other, fast, []byte(s), []byte(s)), NewTag(other, slow.fn, []byte(s), []byte(s))
				if got.ninfo != exp.ninfo || got.info != exp.info {
					t.Errorf("%s: %s tag of %q: expected rows %s and %s, got: %s and %s", name, other, s, tag(exp.ninfo), tag(exp.info), tag(got.ninfo), tag(got.info))
				}
			}
		}
	}
}

// TestFinderSameRows checks that a finder finds the same rows whether it
// compares bytes or runs the regexps of the rows, for the rows of every type.
func TestFinderSameRows(t *testing.T) {
	infos, types := taginfo.All(), finderTypes()
	var spelled, rows int
	for _, name := range slices.Sorted(maps.Keys(infos)) {
		typ, ok := types[name]
		if !ok {
			t.Fatalf("%s: expected a tag type", name)
		}
		checkFinder(t, name, typ, infos[name], finderStrings(infos[name]))
		f := newFinder(typ, infos[name]...)
		for i := range infos[name] {
			rows++
			if f.spellings(int32(i)).strs != nil {
				spelled++
			}
		}
	}
	// the finder is for the rows that are spellings, so that a change which
	// sends them back to a regexp does not go unseen
	if spelled*100 < rows*85 {
		t.Errorf("expected at least 85%% of the rows as spellings, got: %d of %d", spelled, rows)
	}
	t.Logf("%d of %d rows are spellings", spelled, rows)
}

// TestFinderCustomRows checks rows of a caller's own that the built-in rows
// have no like of.
func TestFinderCustomRows(t *testing.T) {
	tests := []struct {
		name string
		typ  TagType
		// tag and regexp of each row
		rows [][2]string
		strs []string
	}{
		{"a row that is case sensitive in part", TagTypeRegion, [][2]string{{"QQ", `(?-i:QQ|QQA)`}, {"qqb", `qq[ab]`}}, []string{"QQ", "qq", "QQA", "qqa", "qqA", "QQB"}},
		{"a spelling in two rows", TagTypeOther, [][2]string{{"AB", `ab|abc`}, {"ABC", `abc`}, {"abC", `(?-i:abC)`}}, []string{"abc", "ab", "abC", "ABC", "abcd"}},
		{"a placeholder in the tag", TagTypeLanguage, [][2]string{{"ZZ$1", `zx(\d)`}, {"Q", `zz1`}, {"YY$1", `yy([ab])`}, {"R", `yyb`}}, []string{"zx1", "ZX2", "zx3", "zz1", "yya", "YYb", "yy"}},
		{"a placeholder in the title alone", TagTypeCodec, [][2]string{{"WW", `ww([ab])`}}, []string{"wwa", "WWB", "ww"}},
		{"a row that matches nothing of its own tag", TagTypeSource, [][2]string{{"NoSuchSpelling", `zzq|zzr`}}, []string{"zzq", "ZZR", "NoSuchSpelling"}},
		{"a tag that another row matches", TagTypeAudio, [][2]string{{"zzs", `zzt`}, {"ZZS.2", `zzs`}}, []string{"zzt", "zzs", "ZZS"}},
		{"a long s and a kelvin sign", TagTypeCut, [][2]string{{"Stuff", "\u017ftuff"}, {"Kat", "\u212aat"}, {"stuck", "stuck"}}, []string{"stuff", "STUFF", "\u017ftuff", "kat", "\u212aAT", "stuck", "\u017ftuc\u212a"}},
		{"a repeat and a class", TagTypeEdition, [][2]string{{"Zork", `zo+rk`}, {"Dee", `de[\d]e`}, {"Opt", `op(?:t)?`}}, []string{"zork", "zooork", "de1e", "dexe", "op", "OPT", "o"}},
		{"a row that is no regexp by itself", TagTypeOther, [][2]string{{"ZZQQ", `zz)|(?:qq`}, {"plain", `zzplain`}}, []string{"zz", "qq", "ZZQQ", "zzx", "xqq", "zzplain", "x"}},
		{"channels that are no row", TagTypeChannels, [][2]string{{"9.1", `9\.1|91`}}, []string{"9.1", "91", "9 1", "910", "51", "5.1"}},
	}
	names := finderTypes()
	for _, test := range tests {
		var rows []*taginfo.Taginfo
		for _, row := range test.rows {
			title := ""
			if test.name == "a placeholder in the title alone" {
				title = "W $1"
			}
			info, err := taginfo.New(row[0], title, row[1], "", "", "")
			if err != nil {
				t.Fatalf("%s: expected no error, got: %v", test.name, err)
			}
			rows = append(rows, info)
		}
		var key string
		for name, typ := range names {
			if typ == test.typ {
				key = name
			}
		}
		// the rows alone, and after the built-in rows of the type
		checkFinder(t, test.name, test.typ, rows, test.strs)
		all := taginfo.All(map[string][]*taginfo.Taginfo{key: rows})[key]
		checkFinder(t, test.name, test.typ, all, append(finderStrings(rows), test.strs...))
	}
}

// TestFinderCallerFunc checks that a tag made or retyped with a find func of
// a caller's has the rows that func gives, and calls it as before.
func TestFinderCallerFunc(t *testing.T) {
	infos := taginfo.All()["source"]
	var calls []string
	f := func(s string) *taginfo.Taginfo {
		calls = append(calls, s)
		return taginfo.Find(infos...)(s)
	}
	tag := NewTag(TagTypeSource, f, []byte("web-dl"), []byte("web-dl"))
	if tag.Source() != "WEB-DL" || tag.Info() == nil || tag.Info().Tag() != "WEB-DL" {
		t.Errorf("expected the WEB-DL row, got: %q %v", tag.Source(), tag.Info())
	}
	if exp := []string{"web-dl", "WEB-DL"}; !slices.Equal(calls, exp) {
		t.Errorf("expected the calls %q, got: %q", exp, calls)
	}
	calls = nil
	text := tag.As(TagTypeText, nil)
	if text.Info() != nil || text.Revert().Info() != tag.Info() || !slices.Equal(calls, []string{"web-dl", "WEB-DL"}) {
		t.Errorf("expected the rows back after Revert, got: %v with the calls %q", text.Revert().Info(), calls)
	}
	calls = nil
	if again := text.As(TagTypeSource, f); again.Info() != tag.Info() || !slices.Equal(calls, []string{"web-dl", "WEB-DL"}) {
		t.Errorf("expected the rows back after As, got: %v with the calls %q", again.Info(), calls)
	}
	// a tag of a parse keeps a find func that a caller can use as before
	tags, _ := ParseTagsString("Title.2020.1080p.WEB-DL.x264-GRP")
	for _, tag := range tags {
		if !tag.Is(TagTypeSource) {
			continue
		}
		if tag.f == nil || tag.f("bluray") == nil || tag.f("bluray").Tag() != "BluRay" {
			t.Errorf("expected the find func of the tag to find rows")
		}
		if as := tag.As(TagTypeSource, tag.f); as.Info() != tag.Info() || as.Normalize() != tag.Normalize() {
			t.Errorf("expected the same rows from As with the tag's own find func")
		}
		if !tag.Match("web-dl", 's') {
			t.Errorf("expected the tag to match another spelling of its row")
		}
	}
}
