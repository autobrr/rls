package rls

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/autobrr/rls/taginfo"
)

// checkDFA checks the one thing an automaton must never do: leave out a
// regexp that matches. Every regexp of res is tried at every offset of every
// input. It returns how many runs of a regexp the automaton ruled out, of how
// many.
func checkDFA(t *testing.T, name string, res []string, inputs []string) (int, int) {
	t.Helper()
	d := newPrefixDFA(res...)
	if d == nil {
		t.Errorf("%s: expected an automaton", name)
		return 0, 0
	}
	compiled := make([]*regexp.Regexp, len(res))
	for l, s := range res {
		compiled[l] = regexp.MustCompile(s)
	}
	var saved, runs int
	for _, s := range inputs {
		b := []byte(s)
		for i := range len(b) + 1 {
			m := d.mask(b, i, len(b))
			for l, re := range compiled {
				runs++
				if m&(1<<l) != 0 {
					continue
				}
				saved++
				if re.Match(b[i:]) {
					t.Fatalf("%s: %q matches %q at %d and the automaton left it out", name, res[l], s, i)
				}
			}
		}
	}
	return saved, runs
}

// TestPrefixDFASound checks the automatons of regexps of every shape the
// lexers have, and of shapes they do not have, on inputs made to match them
// and on the names of tests.yaml.
func TestPrefixDFASound(t *testing.T) {
	names := benchCorpus(t)
	inputs := append(mutations(names[:200], 2), names...)
	inputs = append(inputs,
		"", "s", "S01E02", "s1e2-04", "S01E06.5v2", "s90e01", "S01S02S03", "2x05", "Season.01.Episode.02", "2nd season", "second_season", "vol1no2",
		"Ep. 007", "ep05-07", "10v1.7", "S01.Disc02", "s1957e01", "2024x05", "2006-01-02", "2006–07", "13 Dec 2011", "01-August-1998", "MAY-30-1992",
		"17.12.15", "v1.2a", "version 21H2", "11.09.1", "1080p@59.94fps", "720p60", "23.976fps", "4CD2003", "WEB2007", "2xDVD9", "12DiSCS",
		"\u017f01e02", "\u017feason 1", "wee\u212a 1", "volüme 1", "S01\xffE02", "a\nb", "ab\n", "\n", "aXb", "axxb", "k1", "\u212a1", "1k", "1\u212a",
	)
	sets := map[string][]string{
		"series": {
			`(?i)^s(?P<s>[0-8]?\d)[\-\._ ]?(?:e(?P<e>\d{1,5})(?:(?P<r>[\-~]\d{1,4})|(?P<x>\.\d)?(?P<v>v\d+)?)?)?\b`,
			`(?i)^(?P<S>(?:s[0-8]?\d){2,4})\b`,
			`(?i)^(?P<s>[0-8]?\d)x(?P<e>\d{1,3})\b`,
			`(?i)^(?:series|seasons?|staffel|s)[\-\._ ]?(?P<s>[0-8]?\d)(?:[\-\._ ]?(?:episode|ep)(?P<e>\d{1,5}))?\b`,
			`(?i)^(?P<o>first|second|third|fourth|fifth|sixth)[\-\._ ]season\b`,
			`(?i)^(?:e(?:p(?:isode)?[\-\._ ]{0,3})?|folge[\-\._ ]{0,3})(?P<e>\d{1,5})(?:(?P<v>v\d+)|(?P<x>\.\d)|(?P<r>[\._ ]{0,2}[\-~][\._ ]{0,2}\d{1,3}|[\-~]\d{4}))?\b`,
			`(?i)^(?P<e>\d{1,5})(?P<x>\.\d)?(?P<v>v[\-\._ ]?\d+(?:\.\d){0,2})\b`,
		},
		"date": {
			`(?i)^(?P<2006>(?:19|20)\d{2})[\-\._ ](?P<01>\d{2})[\-\._ ](?P<02>\d{2})\b`,
			`(?i)^(?P<2006>(?:19|20)\d{2})(?:[\-/]|–)(?P<to>(?:19|20)?\d{2})\b`,
			`(?i)^(?:(?P<_2>\d{1,2})(?:th|st|nd|rd)?[\-\._ ])?(?P<Jan>Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			`(?i)^(?P<YY>[12]\d)[\-\._ ](?P<01>\d\d?)[\-\._ ](?P<02>\d\d?)\b`,
		},
		"version": {
			`(?i)^(version[\-\._ ])?(?P<v>v[\-\._ ]?\d{1,2}(?:[\._ ]\d{1,2}[a-z]?\d*){0,3})\b`,
			`(?i)^version[\-\._ ](?P<V>\d{2,}|\d{2}[a-z]{1,2}\d{1,2})\b`,
			`(?i)^(?P<u>\d{1,3}\.\d{1,3}\.\d{1,16}(\.\d{1,16})?)\b`,
		},
		// what the lexers do not have: any rune with and without the newline,
		// a class that is negated, letters that fold to a rune outside ASCII,
		// a rune outside ASCII, a regexp that matches nothing and one that
		// matches the empty string, assertions in the middle, and a lazy
		// repeat
		"shapes": {
			`^a.b`, `(?s)^a.b`, `^[^x]x`, `(?i)^k\d`, `(?i)^\dk`, `(?i)^wee[k]`, `^volüme`, `(?i)^\x{17f}eason`, `^[^\x00-\x{10FFFF}]`, `^(?:x)?`,
			`^a\bX`, `^(?:a$|ab)`, `^a+?b`, `^\b\w+\b$`, `^(?:ab|a)(?:c|bcd)`, `^\d+(?:\.\d+)*$`, `^\S+\s`, `(?i)^[k-l][r-s]`,
		},
	}
	for _, name := range slices.Sorted(maps.Keys(sets)) {
		saved, runs := checkDFA(t, name, sets[name], inputs)
		t.Logf("%s: %d regexps, %d states, %d of %d runs ruled out", name, len(sets[name]), len(newPrefixDFA(sets[name]...).next), saved, runs)
	}
}

// TestPrefixDFARows checks the automaton of every row of the vocabulary that
// a matcher confirms with a regexp, as the matcher builds it, on the names of
// tests.yaml.
func TestPrefixDFARows(t *testing.T) {
	inputs := benchCorpus(t)
	if testing.Short() {
		inputs = inputs[:150]
	}
	infos := taginfo.All()
	var rows int
	for _, typ := range slices.Sorted(maps.Keys(infos)) {
		for _, source := range []bool{false, true} {
			m := newMatcher(infos[typ], true, source)
			if m == nil {
				t.Fatalf("%s: expected a matcher", typ)
			}
			for k, row := range m.rows {
				if row.spellings != nil {
					continue
				}
				rows++
				name := fmt.Sprintf("%s row %s", typ, infos[typ][k].Tag())
				spellings := []string{infos[typ][k].Tag(), strings.ToLower(infos[typ][k].Tag()) + ".x", infos[typ][k].Tag() + "\u017f"}
				checkDFA(t, name, []string{row.re().String()}, append(spellings, inputs...))
			}
		}
	}
	if rows == 0 {
		t.Errorf("expected rows that are confirmed with a regexp")
	}
}

// TestPrefixDFARefused checks that regexps an automaton cannot stand for get
// none, and so are run everywhere.
func TestPrefixDFARefused(t *testing.T) {
	many := make([]string, maxDFARegexps+1)
	for l := range many {
		many[l] = fmt.Sprintf(`^x%d`, l)
	}
	tests := []struct {
		name string
		res  []string
	}{
		{"none", nil},
		{"more than it has bits for", many},
		{"not anchored", []string{`^a`, `b`}},
		{"anchored in one half", []string{`^a|b`}},
		{"anchored to a line", []string{`(?m)^a`}},
		{"anchored to the end", []string{`a$`}},
		{"does not compile", []string{`^a(`}},
		{"too many states", []string{`^[ab]*a[ab]{12}c`}},
	}
	for _, test := range tests {
		if d := newPrefixDFA(test.res...); d != nil {
			t.Errorf("%s: expected no automaton, got one of %d states", test.name, len(d.next))
		}
	}
	for _, res := range [][]string{{`^a`}, {`(?i)^(a|b)\b`, `^\Ab`}, many[:maxDFARegexps]} {
		if newPrefixDFA(res...) == nil {
			t.Errorf("%q: expected an automaton", res)
		}
	}
	// regexps without an automaton are matched as before
	lex := NamedCaptureLexer(`^(?P<a>q.x)\b|(?P<b>zz)`, `(?m)^(?P<c>yy)`)
	for in, exp := range map[string]bool{"qax": true, "zz": true, "yy": true, "qa": false, "ab": false} {
		if _, _, _, _, ok := lex([]byte(in), []byte(in), 0, len(in)); ok != exp {
			t.Errorf("%q: expected %t, got: %t", in, exp, ok)
		}
	}
}
