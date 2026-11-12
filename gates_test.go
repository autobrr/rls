package rls

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/autobrr/rls/reutil"
	"github.com/autobrr/rls/taginfo"
)

// TestMetaSuffixFrom checks that the suffix of a meta pattern is found from
// where suffixFrom says the search can begin as it is found from the start
// of the window: for the default patterns and for patterns with values of
// other shapes, on every string of a few bytes that a pattern looks at.
func TestMetaSuffixFrom(t *testing.T) {
	tests := []struct {
		open, cl, value string
		narrow          bool
	}{
		{`[[`, `]]`, `([a-zA-Z][a-zA-Z0-9_]{0,15}):\s*([^ \t\]]{1,32})`, true},
		{`[`, `]`, `(REQ(?:UEST)?)`, true},
		{`(`, `)`, `(REQ(?:UEST)?)`, true},
		{`{`, `}`, `(REQ(?:UEST)?)`, true},
		{`[`, `]`, `((?i:[0-9A-F]{8}))`, true},
		{`[`, `]`, `([^ \t\]]{1,32})`, true},
		{`-={`, `}=-`, `([^ \t\}]{1,32})`, true},
		{`{{`, `}}`, `([^ \t\}]{1,32})`, true},
		// values of other shapes: any rune, a letter that folds to a rune
		// outside ASCII, a rune outside ASCII, the bytes of the opening
		// literal, nothing at all, and assertions, which take the pattern out
		{`[`, `]`, `(.{1,4})`, true},
		{`[`, `]`, `((?i:k[a-c]s)+)`, true},
		{`[`, `]`, `(é+|a)`, true},
		{`<<`, `>`, `([<a]{1,3})`, true},
		{`(`, `)`, `(a?)`, true},
		{`[`, `]`, `(\ba+)`, false},
		{`[`, `]`, `(a+$|b)`, false},
		{`[`, `]`, `(^a)`, false},
		// and values that are no regexp by themselves
		{`[`, `]`, `(a)|(b)`, false},
		{`[`, `]`, `(a)\Q`, false},
		{`[`, `]`, `?(a)`, false},
	}
	// every string of three of the symbols, and longer ones made of two or
	// three of those
	symbols := []string{"[", "]", "(", ")", "{", "}", "=", "-", "<", ">", ":", " ", "\t", "a", "A", "k", "\u212a", "1", "REQ", "é", "\xff", "ABCD1234"}
	var inputs []string
	for a := range symbols {
		for b := range symbols {
			for c := range symbols {
				inputs = append(inputs, symbols[a]+symbols[b]+symbols[c])
			}
		}
	}
	short := len(inputs)
	for k := range short {
		inputs = append(inputs, inputs[k]+inputs[(k*7+3)%short], inputs[(k*13+5)%short]+" "+inputs[k]+" ")
	}
	var narrowed, searched, matched int
	for _, test := range tests {
		s := `\s*\Q` + test.open + `\E\s*` + test.value + `\s*\Q` + test.cl + `\E\s*`
		suffix := regexp.MustCompile(s + `$`)
		ok, never := metaNever(test.open, test.cl, test.value)
		if ok != test.narrow {
			t.Errorf("%q %q %q: expected narrow %t, got: %t", test.open, test.value, test.cl, test.narrow, ok)
		}
		if !ok {
			continue
		}
		// what the pattern is made to match, after what it may or may not
		// take as its own
		made := inputs
		for k, junk := range symbols {
			for _, value := range []string{"a", "REQ", "REQUEST", "ABCD1234", "k:v", "kas", "\u212aa\u017f", "é", "<a", "", "a b", "site", "x]y", "aa"} {
				made = append(made,
					junk+test.open+value+test.cl,
					junk+symbols[(k+5)%len(symbols)]+" "+test.open+" "+value+" "+test.cl+" ",
					test.open+junk+test.open+value+test.cl+"\t",
				)
			}
		}
		for _, in := range made {
			src := []byte(in)
			for _, lo := range []int{0, min(2, len(src))} {
				tail := bytes.TrimRight(src[lo:], "\t\n\f\r ")
				if !bytes.HasSuffix(tail, []byte(test.cl)) {
					continue
				}
				exp := suffix.FindSubmatchIndex(src[lo:])
				if exp != nil {
					matched++
				}
				for k := range exp {
					if exp[k] != -1 {
						exp[k] += lo
					}
				}
				var got []int
				from := suffixFrom(src, lo, lo+len(tail)-len(test.cl), test.open[0], &never)
				if searched++; from != -1 {
					got = suffix.FindSubmatchIndex(src[from:])
					for k := range got {
						if got[k] != -1 {
							got[k] += from
						}
					}
					if from > lo {
						narrowed++
					}
				} else {
					narrowed++
				}
				if !slices.Equal(got, exp) {
					t.Fatalf("%q %q %q on %q from %d: expected the match %v, got: %v when searched from %d", test.open, test.value, test.cl, in, lo, exp, got, from)
				}
			}
		}
	}
	if narrowed*2 < searched || matched < 1000 {
		t.Errorf("expected most searches to begin later or not at all and many to match, got: %d and %d of %d", narrowed, matched, searched)
	}
	t.Logf("%d of %d searches begin later or not at all, %d match", narrowed, searched, matched)
}

// TestExtTail checks that the test the ext lexer makes before its regexp
// never rules out a name the regexp matches: for the extensions as they are
// written and in other cases, with what may stand in front of and after them,
// and for the names of tests.yaml.
func TestExtTail(t *testing.T) {
	ext := taginfo.All()["ext"]
	re := regexp.MustCompile(`(?i:\.` + reutil.Taginfo("$", ext...) + `)`)
	tails := make(map[string]struct{})
	names := benchCorpus(t)
	for _, info := range ext {
		exp, err := expandRE(info.RE(), true)
		if err != nil {
			t.Fatalf("%s: expected the row to expand", info.Tag())
		}
		for _, s := range exp {
			tails[strings.ToUpper(s)] = struct{}{}
			for _, v := range []string{s, strings.ToUpper(s), strings.ToLower(s), strings.Replace(s, "s", "\u017f", 1), strings.Replace(s, "k", "\u212a", 1)} {
				names = append(names, "Title."+v, "."+v, v, "Title."+v+".", "Title."+v+".x", "Title.x."+v, "Title.é."+v, "Title."+v+"é", "Title."+v+" ", "Title.."+v, "Title.tar."+v)
			}
		}
	}
	names = append(names, "", ".", "..", "Title", "Title.", "Title.zzz", "Title.\xff", "Title."+strings.Repeat("a", 60))
	var ruledOut int
	for _, name := range names {
		b := []byte(name)
		matched, tail := re.Match(b), extTail(b, tails)
		if matched && !tail {
			t.Errorf("%q: the regexp matches and the test rules it out", name)
		}
		if !tail {
			ruledOut++
		}
		if extTail(b, nil) != (strings.IndexByte(name, '.') != -1) {
			t.Errorf("%q: expected any period to do without tails", name)
		}
	}
	if ruledOut == 0 {
		t.Errorf("expected names to be ruled out")
	}
	t.Logf("%d of %d names ruled out", ruledOut, len(names))
}
