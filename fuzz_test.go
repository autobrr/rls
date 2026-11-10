package rls

import (
	"fmt"
	"testing"
)

// exercise walks everything reachable from a parsed release, so a panic deeper
// than ParseString is reachable from the fuzzer too.
func exercise(r Release) {
	_ = r.String()
	_ = fmt.Sprintf("%o|%s|%e|%q", r, r, r, r)
	_ = r.SeriesEpisodes()
	for _, tag := range r.Tags() {
		_ = tag.Normalize()
		_ = fmt.Sprintf("%o|%s|%v|%q|%e", tag, tag, tag, tag, tag)
		_ = tag.Info()
		_ = tag.InfoTitle()
		_ = tag.InfoType()
		_ = tag.InfoExcl()
		_ = tag.Text()
		_ = tag.TextReplace(".", " ", -1)
		_ = tag.SingleEp()
	}
	for _, tag := range r.Unused() {
		_ = tag.Text()
	}
	for _, tag := range r.Dates() {
		_ = tag.Normalize()
	}
	_ = MustNormalize(r.Title)
	_ = MustClean(r.Title)
	_ = Compare(r, r)
}

// FuzzParseString checks that no release name can panic the parser. autobrr and
// friends feed this attacker-influencable strings from IRC announces and RSS
// feeds, so this is the property that matters most.
//
// It also checks that the tags reproduce the input: a lexer that skips bytes
// without tagging them loses part of the name silently, and nothing else would
// notice.
func FuzzParseString(f *testing.F) {
	for _, test := range rlsTests(f) {
		f.Add(test.s)
	}
	f.Add("")
	f.Add("5.1")
	f.Add("x")
	// a version or disc after an episode, written with a byte that is a
	// space to the lexers
	f.Add("Show.13v_2.720p")
	f.Add("0v+0")
	f.Add("s02dvd3")
	f.Fuzz(func(t *testing.T, s string) {
		r := ParseString(s)
		if o := fmt.Sprintf("%o", r); o != s {
			t.Errorf("round trip failed:\n in: %q\nout: %q", s, o)
		}
		exercise(r)
	})
}
