package rls

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/autobrr/rls/taginfo"
)

// TestPrefilterSound checks the property the prefilter relies on: it must never
// reject a position where the type's alternation would have matched. Rejecting
// wrongly silently drops tags, so this is verified directly against every
// taginfo row rather than inferred.
func TestPrefilterSound(t *testing.T) {
	infos := taginfo.All()
	for typ, info := range infos {
		pf, ok := newPrefilter(info, true)
		// every type has one today. A row that takes it away costs a regexp
		// run on every token of every name
		if !ok {
			t.Errorf("%s: no prefilter (a row could not be reduced)", typ)
			continue
		}
		for _, i := range info {
			v, err := expandRE(i.RE(), true)
			if err != nil {
				continue // covered by the lead byte set, exercised below
			}
			for _, s := range v {
				if s == "" {
					continue
				}
				for _, b := range [][]byte{[]byte(s), []byte(s + ".1080p"), []byte(s + "-GRP")} {
					if !pf.maybe(b, 0, len(b)) {
						t.Errorf("%s: prefilter rejected %q, which row %q can match", typ, b, i.Tag())
					}
				}
			}
		}
	}
}

// TestPrefilterSameParse checks that the prefilters only save work: a parser
// built without them gives the same tags for every name of tests.yaml, for
// every spelling a row can be expanded to, and for the corpora when they are
// present.
func TestPrefilterSameParse(t *testing.T) {
	noPrefilter = true
	slow := NewParser()
	noPrefilter = false
	fast := NewParser()
	var names []string
	for _, test := range rlsTests(t) {
		names = append(names, test.s)
	}
	for _, info := range taginfo.All() {
		for _, i := range info {
			v, err := expandRE(i.RE(), true)
			if err != nil {
				continue
			}
			for _, s := range v {
				names = append(names, "Title."+s+".1080p-GRP")
			}
		}
	}
	for _, name := range []string{"nyaa.txt", "sports.txt"} {
		if buf, err := os.ReadFile("testdata/corpus/" + name); err == nil && !testing.Short() {
			names = append(names, strings.Split(string(buf), "\n")...)
		}
	}
	for _, name := range names {
		a, an := fast.Parse([]byte(name))
		b, bn := slow.Parse([]byte(name))
		if s, exp := fmt.Sprintf("%q", a), fmt.Sprintf("%q", b); s != exp || an != bn {
			t.Fatalf("%q: expected the tags without prefilters\n%s (%d)\ngot:\n%s (%d)", name, exp, bn, s, an)
		}
	}
	t.Logf("%d names", len(names))
}

func TestPrefilterRejects(t *testing.T) {
	infos := taginfo.All()
	pf, ok := newPrefilter(infos["resolution"], true)
	if !ok {
		t.Fatal("expected a resolution prefilter")
	}
	for _, s := range []string{"Frieren", "Sousou", "Bundesliga", "Djokovic"} {
		if pf.maybe([]byte(s), 0, len(s)) {
			t.Errorf("expected %q to be rejected by the resolution prefilter", s)
		}
	}
}

// TestFindFuncNonASCII checks that the indexed lookup finds the same row as
// taginfo.Find for a spelling with a non-ASCII letter, which a row's regexp
// can ask for ('ungek[uü]e?rzt') and the index cannot hold.
func TestFindFuncNonASCII(t *testing.T) {
	infos := taginfo.All()["cut"]
	f, find := findFunc(infos...), taginfo.Find(infos...)
	for _, s := range []string{"ungekürzt", "UNGEKÜRZT", "Ungekuerzt", "ungekurzt", "uncut"} {
		exp := find(s)
		if exp == nil {
			t.Fatalf("%q expected a cut tag info", s)
		}
		if info := f(s); info != exp {
			t.Errorf("%q expected %q, got: %v", s, exp.Tag(), info)
		}
	}
}

// firstBytesStrings gives the strings of up to three of the symbols, which
// checkFirstBytes tries a regexp on.
func firstBytesStrings(symbols ...string) []string {
	v, last := []string{""}, []string{""}
	for range 3 {
		var next []string
		for _, s := range last {
			for _, c := range symbols {
				next = append(next, s+c)
			}
		}
		v, last = append(v, next...), next
	}
	return v
}

// firstBytesFew is a short list of symbols for many regexps, and
// firstBytesAll a longer one with the runes that fold to a letter of ASCII
// (the long s and the Kelvin sign), the runes on either side of the
// surrogates, U+FFFD and its bytes, a rune of four bytes, and bytes that are
// no UTF-8.
var (
	firstBytesFew = sync.OnceValue(func() []string {
		return firstBytesStrings("a", "b", "q", "x", "A", "Q", "1", "(", "|", "]", "-", "{", "}", " ", "é", "ſ", "K", "\xff")
	})
	firstBytesAll = sync.OnceValue(func() []string {
		return firstBytesStrings(
			"a", "b", "q", "x", "A", "Q", "1", "(", "|", "]", "-", "{", "}", " ", "é", "ſ", "K", "\xff",
			"k", "s", "K", "\n", "_", "퟿", "", "�", "\U00010400", "\x80", "\xc0",
		)
	})
)

// checkFirstBytes fails when firstBytes gives a set for re that leaves out the
// first byte of a string re matches at its start, or gives a set at all for a
// regexp that matches the empty string. The strings are tried on the regexp
// as it is written and without regard to case, since it has callers of both
// kinds. It reports whether firstBytes gave a set.
func checkFirstBytes(t *testing.T, re string, strs []string) bool {
	t.Helper()
	var lead [256]bool
	if !firstBytes(re, &lead) {
		return false
	}
	var missing []string
	for _, flags := range []string{"", "(?i)"} {
		anchored, err := regexp.Compile(flags + `^(?:` + re + `)`)
		if err != nil {
			t.Errorf("%q: expected no first bytes for a regexp that does not compile", re)
			return true
		}
		for _, s := range strs {
			switch {
			case !anchored.MatchString(s):
			case s == "":
				t.Errorf("%q: expected no first bytes for a regexp that matches the empty string", re)
				return true
			case !lead[s[0]] && len(missing) < 4:
				missing = append(missing, s)
			}
		}
	}
	if len(missing) != 0 {
		t.Errorf("%q: expected the first byte of %q among the first bytes", re, missing)
	}
	return true
}

// TestFirstBytesSound checks the first bytes of a regexp against what the
// regexp matches: the first byte of every string it matches is in the set, or
// no set is given. A byte left out makes a lexer skip a position where the
// regexp matches, which loses the tag.
func TestFirstBytesSound(t *testing.T) {
	// with is whether firstBytes gives a set, which it does not for a regexp
	// that begins with anything
	for _, test := range []struct {
		re   string
		with bool
	}{
		// a class
		{`[0-9]x`, true},
		{`[\-\._ ]x`, true},
		{`[\d]x`, true},
		{`[\w]x`, true},
		{`[\d\-x]q`, true},
		{`[a\]]x`, true},
		{`[]a]x`, true},
		{`[+--]x`, true},
		{`[\s]x`, true},
		{`[\D]x`, true},
		{`[\W]x`, true},
		{`[[:digit:]]x`, true},
		{`[[:alpha:]]x`, true},
		{`[[:^alpha:]]x`, true},
		{`[\pN]x`, true},
		{`[\x30-\x39]x`, true},
		{`[\x{30}-\x{39}]x`, true},
		{`[\060-\071]x`, true},
		{`[é]x`, true},
		{`[a-é]x`, true},
		{`[^a]x`, true},
		{`[^k]x`, true},
		// a class or a literal around the surrogates, U+FFFD and U+10000
		{`[\x{80}-\x{7FF}]x`, true},
		{`[\x{100}-\x{2FFF}]x`, true},
		{`[\x{D000}-\x{E000}]x`, true},
		{`[\x{D900}-\x{DA00}]x`, true},
		{`[\x{FFFE}-\x{10400}]x`, true},
		{`[\x{E000}-\x{FFFD}]x`, true},
		{`\x{D800}x`, true},
		{`(?i)\x{DFFF}x`, true},
		{`\x{FFFD}x`, true},
		{`[^\x00-\x{10FFFF}]x`, true},
		// a bracket or a bar that is text
		{`a[(]|b`, true},
		{`a[)]|b`, true},
		{`q[|a]|b`, true},
		{`a\Q(\E|b`, true},
		{`(?:(?:[(])|)b`, true},
		// a group that can match nothing
		{`(a*)q`, true},
		{`(?:a|)q`, true},
		{`(?:a?)q`, true},
		{`(?:a?b?)q`, true},
		{`(?:(?:a|)b*)q`, true},
		{`(?P<e>(?:a?)\d)`, true},
		{`(a)q`, true},
		{`(a|b)?q`, true},
		// a lazy quantifier, a count, and a brace that is text
		{`a??q`, true},
		{`a*?q`, true},
		{`a+?q`, true},
		{`a{0,2}?q`, true},
		{`(?:ab)??q`, true},
		{`a{0}q`, true},
		{`a{0,}q`, true},
		{`a{1,2}q`, true},
		{`a{|b}`, true},
		{`(){}a`, true},
		{`a{00,1}`, true},
		// a quantifier after a rune outside ASCII, a quote or an assertion
		{`é?b`, true},
		{`ſ*b`, true},
		{`\Qab\E?x`, true},
		{`\Qa\E?x`, true},
		{`\Qé\E?x`, true},
		{`^*a`, true},
		{`^^*(?P<e>\d)`, true},
		{`\b*a`, true},
		// case, and the two runes that fold to a letter of ASCII
		{`k`, true},
		{`K1`, true},
		{`(?i)s1`, true},
		{`(?-i:Q)x`, true},
		{`(?-i:k)x`, true},
		{`ſ1`, true},
		{`\x{212A}1`, true},
		// what begins with anything
		{`a*`, false},
		{`a|`, false},
		{`(?:a)?`, false},
		{`(a*)(b?)`, false},
		{`é?`, false},
		{`^*`, false},
		{``, false},
		{`.x`, false},
		{`(?s:.)x`, false},
		// what does not compile
		{`a(`, false},
		{`[a`, false},
	} {
		if with := checkFirstBytes(t, test.re, firstBytesAll()); with != test.with {
			t.Errorf("%q: expected first bytes %t, got: %t", test.re, test.with, with)
		}
	}
	// every pair of these, which is where one piece changes how the next is
	// read
	if !testing.Short() {
		pieces := []string{
			`a`, `b?`, `q*`, `x+?`, `x??`, `(a*)`, `(?:a|)`, `(?:b|q)`, `(?P<n>a)?`, `[\d]`, `[a(]`, `[|]`, `[^a]`, `[é]`, `[\w-]`, `[]a]`,
			`[[:alpha:]]`, `é`, `é?`, `ſ`, `k`, `\Qa(\E`, `\Q|\E?`, `^`, `$`, `^*`, `\b`, `\d`, `\s?`, `a{0,1}`, `a{|b}`, `{`, `|`,
			`(?i)`, `(?i:q)`, `(?-i:Q)`, `(?:)`, `1+`, `-`,
		}
		for _, a := range pieces {
			for _, b := range pieces {
				if _, err := regexp.Compile(a + b); err == nil {
					checkFirstBytes(t, a+b, firstBytesFew())
				}
			}
		}
	}
	// every row of the tag table that is not a list of spellings, and every
	// pattern the lexers of this package are built from, has a set: without
	// one a regexp is run at every position of every name
	for typ, infos := range taginfo.All() {
		for _, info := range infos {
			var lead [256]bool
			if _, err := expandRE(info.RE(), true); err != nil && !firstBytes(info.RE(), &lead) {
				t.Errorf("%s: no first bytes for row %q", typ, info.Tag())
			}
		}
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	var patterns int
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil || !strings.Contains(s, "(?P<") {
				return true
			}
			if _, err := regexp.Compile(s); err != nil {
				return true
			}
			patterns++
			if _, ok := leadSet(s); !ok {
				t.Errorf("%s: no first bytes for %q", file, s)
			}
			return true
		})
	}
	if patterns < 40 {
		t.Errorf("expected the patterns of the lexers, got: %d", patterns)
	}
}

// TestPrefilterCustomRows checks that a row of a caller's own is found
// wherever its regexp matches, whatever the regexp begins with: a parser with
// the prefilters and matchers gives the tags of one without them.
func TestPrefilterCustomRows(t *testing.T) {
	for _, test := range []struct {
		re    string
		names []string
	}{
		{`[\d]quux`, []string{"1quux", "quux", "dquux"}},
		{`[[:digit:]]quux`, []string{"1quux", "quux"}},
		{`[\w]{3}zzq`, []string{"abczzq", "123zzq", "wwwzzq"}},
		{`[\pN]quux`, []string{"1quux", "٣quux"}},
		{`[\x30-\x39]quux`, []string{"1quux", "0quux"}},
		{`(a*)quux`, []string{"quux", "aquux", "aaquux"}},
		{`(?:a|)quux`, []string{"quux", "aquux"}},
		{`(?:a?b?)quux`, []string{"quux", "abquux", "bquux"}},
		{`a??quux`, []string{"quux", "aquux"}},
		{`a*?quux`, []string{"quux", "aaquux"}},
		{`a{0,2}?quux`, []string{"quux", "aaquux"}},
		{`\Qqu\E?ux`, []string{"quux", "qux", "ux"}},
		{`a+[(]|zqb`, []string{"zqb", "a("}},
		{`a+\Q(\E|zqb`, []string{"zqb", "a("}},
		{`é?zqb`, []string{"zqb", "ézqb"}},
		{`^*zqb`, []string{"zqb"}},
	} {
		info, err := taginfo.New("QQ", "", test.re, "", "", "")
		if err != nil {
			t.Fatalf("%q: expected no error, got: %v", test.re, err)
		}
		extra := map[string][]*taginfo.Taginfo{"other": {info}}
		noPrefilter, noMatcher = true, true
		slow := NewParser(WithTaginfo(extra))
		noPrefilter, noMatcher = false, false
		fast := NewParser(WithTaginfo(extra))
		for _, name := range test.names {
			for _, s := range []string{name, "Title.2020." + name + ".1080p-GRP", "Title " + name + "_x", "[Grp] Title - 01 [" + name + "]"} {
				sameTags(t, fast, slow, s)
			}
		}
		// the first name is one the row matches
		s := "Title.2020." + test.names[0] + ".1080p-GRP"
		if r := fast.ParseRelease([]byte(s)); !slices.Contains(r.Other, "QQ") {
			t.Errorf("%q: expected QQ in other for %q, got: %q", test.re, s, r.Other)
		}
	}
}

// TestNamedCaptureLead checks that a pattern of a caller's own is tried
// wherever it can match, whatever it begins with.
func TestNamedCaptureLead(t *testing.T) {
	for _, test := range []struct {
		re, s string
	}{
		{`^(?P<e>[\d]{2})`, "12"},
		{`^(?P<e>[[:digit:]]{2})`, "12"},
		{`^(?P<e>[\pN]{2})`, "12"},
		{`^(?P<e>(?:a?)\d\d)`, "12"},
		{`^(?:ep?|)(?P<e>\d\d)`, "12"},
		{`^e??(?P<e>\d\d)`, "12"},
		{`^e{0,2}?(?P<e>\d\d)`, "12"},
		{`^x[(]|(?P<e>\d\d)`, "12"},
		{`^é?(?P<e>\d\d)`, "12"},
		{`^^*(?P<e>\d\d)`, "12"},
	} {
		tags, _ := NewTagParser(nil, NewSeriesLexer(test.re)).Parse([]byte(test.s))
		if len(tags) != 1 || !tags[0].Is(TagTypeSeries) {
			t.Errorf("%q: expected a series tag for %q, got: %q", test.re, test.s, tags)
		}
	}
}
