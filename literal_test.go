package rls

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/autobrr/rls/taginfo"
)

// parsers returns the default parser built with the matchers and without
// them, the second of which confirms every row with its alternation.
var parsers = sync.OnceValues(func() (*TagParser, *TagParser) {
	noMatcher = true
	slow := NewParser()
	noMatcher = false
	return NewParser(), slow
})

// sameTags checks that the two parsers give the same tags for name.
func sameTags(tb testing.TB, fast, slow *TagParser, name string) {
	tb.Helper()
	a, an := fast.Parse([]byte(name))
	b, bn := slow.Parse([]byte(name))
	if s, exp := fmt.Sprintf("%q", a), fmt.Sprintf("%q", b); s != exp || an != bn {
		tb.Fatalf("%q: expected the tags of the alternation\n%s (%d)\ngot:\n%s (%d)", name, exp, bn, s, an)
	}
}

// TestMatcherSameParse checks that a parser gives the same tags with its
// matchers as it gives with the alternations they stand in for: for every
// name of tests.yaml, for every spelling a row can be expanded to with what
// may stand around it, and for the corpora when they are present.
func TestMatcherSameParse(t *testing.T) {
	fast, slow := parsers()
	var names []string
	for _, test := range rlsTests(t) {
		names = append(names, test.s)
	}
	affixes := []string{"", ".", "x", "_", "é", "-rest", "+", " 5.1", ")", "ſ", "_x", ".1080p", "-", "é.x", "\xff"}
	for _, info := range taginfo.All() {
		for _, i := range info {
			v, err := expandRE(i.RE(), true)
			if err != nil {
				v = []string{i.Tag()}
			}
			for k, s := range v {
				// every affix on the first spellings of a row, and one on
				// the others
				for l, affix := range affixes {
					if k >= 4 && l != k%len(affixes) {
						continue
					}
					names = append(names, "Title."+s+affix, s+affix, "Title "+strings.ToLower(s)+affix+" 720p", "K"+s+affix)
				}
			}
		}
	}
	for _, name := range []string{"nyaa.txt", "sports.txt"} {
		if buf, err := os.ReadFile("testdata/corpus/" + name); err == nil && !testing.Short() {
			names = append(names, strings.Split(string(buf), "\n")...)
		}
	}
	for _, name := range names {
		sameTags(t, fast, slow, name)
	}
	t.Logf("%d names", len(names))
}

// TestMatcherRows checks that the matchers hold what they are for: most rows
// of the vocabulary as spellings, so that a change to the rows or to the
// matcher that sends them back to a regexp does not go unseen.
func TestMatcherRows(t *testing.T) {
	var rows, spelled int
	for typ, info := range taginfo.All() {
		m := newMatcher(info, true, false)
		if m == nil {
			t.Errorf("%s: expected a matcher", typ)
			continue
		}
		for _, row := range m.rows {
			rows++
			if row.spellings != nil {
				spelled++
			}
		}
	}
	if spelled*100 < rows*90 {
		t.Errorf("expected at least 90%% of the rows as spellings, got: %d of %d", spelled, rows)
	}
	t.Logf("%d of %d rows are spellings", spelled, rows)
}

// TestMatcherCustomRows checks rows of a caller's own that the built-in rows
// have no like of. Each set has to give the tags its alternation gives,
// which for some means that the type goes without a matcher.
func TestMatcherCustomRows(t *testing.T) {
	tests := []struct {
		name string
		// tag and regexp of each row
		rows   [][2]string
		names  []string
		refuse bool
	}{
		{"flags set for the rows after", [][2]string{{"QQA", `(?-i)QQA`}, {"zzq", ``}}, []string{"ZZQ", "zzq", "x.ZZQ.y", "QQA"}, true},
		{"a row that is two halves", [][2]string{{"Unb", `qqz)|(rrz`}}, []string{"qqz", "rrz", "x.rrz"}, true},
		{"a row that matches nothing", [][2]string{{"Empty", `(?:zqx)?`}}, []string{"abc", "zqx", "x.y"}, true},
		{"a quantifier after a quote", [][2]string{{"Wobble", `\QWOBBLE\E?`}, {"WOBBL2", `WOBBL[\-\._ ]?2`}}, []string{"WOBBL", "WOBBLE", "WOBBL.2", "x.WOBBL.y"}, false},
		{"a long s that folds to s", [][2]string{{"Stuff", `ſtuff`}, {"StuffX", `stuff[\-\._ ]x`}}, []string{"stuff", "STUFF.y", "stuff.x"}, false},
		{"a kelvin sign that folds to k", [][2]string{{"Kat", "Kat+"}, {"KatX", `kat[\-\._ ]x`}}, []string{"kat", "kaat", "KAT.y", "kat.x"}, false},
		{"more groups than the room for values", [][2]string{{"Cap$1", `cap(a)(b)(c)(d)(e)(f)(g)(h)(i)(j)(k)(l)(m)(n)(o)(p)(q)(r)`}}, []string{"capabcdefghijklmnopqr", "x.capabcdefghijklmnopqr.y"}, false},
		{"a repeat beside its spelling", [][2]string{{"Zork", `zo+rk`}, {"Zoork", `zoork`}}, []string{"zoork", "zooork", "ZORK"}, false},
		{"a lazy optional", [][2]string{{"Lazy", `laz(?:y)??`}}, []string{"lazy", "laz", "lazy.x"}, false},
		{"a spelling that begins another row", [][2]string{{"AB", `ab|abc`}, {"ABC", `abc`}}, []string{"abc", "ab", "abc.x", "abcd"}, false},
		{"a class written with an escape", [][2]string{{"Dee", `de[\d]e`}}, []string{"de1e", "dexe"}, false},
	}
	for _, test := range tests {
		var rows []*taginfo.Taginfo
		for _, row := range test.rows {
			info, err := taginfo.New(row[0], "", row[1], "", "", "")
			if err != nil {
				t.Fatalf("%s: expected no error, got: %v", test.name, err)
			}
			rows = append(rows, info)
		}
		extra := map[string][]*taginfo.Taginfo{"other": rows}
		if m := newMatcher(taginfo.All(extra)["other"], true, false); (m == nil) != test.refuse {
			t.Errorf("%s: expected refused %t, got: %t", test.name, test.refuse, m == nil)
		}
		noMatcher = true
		slow := NewParser(WithTaginfo(extra))
		noMatcher = false
		fast := NewParser(WithTaginfo(extra))
		for _, name := range test.names {
			for _, s := range []string{name, "Title.2020." + name + ".1080p-GRP", "Title " + name + "_x", name + "+"} {
				sameTags(t, fast, slow, s)
			}
		}
	}
}
