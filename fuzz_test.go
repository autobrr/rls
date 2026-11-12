package rls

import (
	"fmt"
	"strings"
	"testing"
)

// exercise walks everything reachable from a parsed release, so a panic deeper
// than ParseString is reachable from the fuzzer too.
func exercise(r Release) {
	_ = r.String()
	_ = fmt.Sprintf("%o|%s|%e|%q", r, r, r, r)
	_ = r.SeriesEpisodes()
	_ = r.EpisodeSpans()
	_, _ = r.Event()
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

// checkTags lexes s and checks that the tags reproduce it as they come out of
// Parse, before Build has touched them. The lexer contract lets a lexer skip
// input without tagging it, and the round trip on the built release does not
// say which step lost the bytes.
//
// The report gives the byte at which the tags leave the input and the tags
// around it. The lexer at fault made the last of them or one before it: bytes
// that were skipped go unnoticed for as long as the text after them repeats
// them ('-Retail-GRP' with '-Retail' skipped still matches at the '-').
//
// A tag may have no text. The series lexer adds a Date tag for a year written
// as a season ('S2024E05'), and the Series tag before it holds the bytes.
func checkTags(tb testing.TB, s string) {
	tb.Helper()
	tags, _ := ParseTagsString(s)
	var b strings.Builder
	at := -1
	for i, tag := range tags {
		fmt.Fprintf(&b, "%o", tag)
		if at < 0 && !strings.HasPrefix(s, b.String()) {
			at = i
		}
	}
	out := b.String()
	if out == s {
		return
	}
	// every tag matched, so the tags stop short of the input
	if at < 0 {
		at = len(tags) - 1
	}
	n := 0
	for n < len(s) && n < len(out) && s[n] == out[n] {
		n++
	}
	tb.Errorf("tags leave the input at byte %d:\n in: %q\nout: %q\ntags %d to %d: %q",
		n, s, out, max(at-2, 0), at, tags[max(at-2, 0):at+1])
}

// FuzzParseString checks that no release name can panic the parser. autobrr and
// friends feed this attacker-influencable strings from IRC announces and RSS
// feeds, so this is the property that matters most.
//
// It also checks that the tags reproduce the input: a lexer that skips bytes
// without tagging them loses part of the name silently, and nothing else would
// notice. checkTags makes the same comparison on the tags straight out of
// Parse, with no Build in between, so a failure names the lexing step.
//
// Plain go test runs the seeds, which are every name in tests.yaml and the
// inputs below, so both checks run without -fuzz.
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
	// half an episode that the builder joins onto its tag, once, twice and
	// with more digits after it
	f.Add("Show.S01E06.5.WEB.x264-GRP")
	f.Add("Show.S01E06.5.S01E07.5.WEB")
	f.Add("S01E06.5.5.5.5.WEB")
	// a range in the bracket that closes the name
	f.Add("[Grp] Show [1-13]")
	f.Add("Show (01-12).mkv")
	// a list of seasons, whole, cut short and at the end of the name
	f.Add("Show (Season 1+2+3) [1080p]")
	f.Add("Show S01+02 & 03, 04")
	f.Add("Seasons 1 &")
	f.Add("S1+")
	f.Add("S1 , 2")
	// the edges of the lexers that read the start and the end of a name
	for _, s := range []string{
		" ", "...", "-", "[]", "[[", "\x00", "\xff\xfe", "-GROUP", ".mkv",
		"[ABCD1234]", "a.S01E01.\xff\xff.1080p-g",
		"Show.S01E02.1080p.WEB.h264-GRP  ", "  Show.S01E02",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		checkTags(t, s)
		r := ParseString(s)
		if o := fmt.Sprintf("%o", r); o != s {
			t.Errorf("round trip failed:\n in: %q\nout: %q", s, o)
		}
		exercise(r)
	})
}

// FuzzParseMatcher checks that the matchers of the vocabulary lexers give
// the tags their alternations give.
func FuzzParseMatcher(f *testing.F) {
	for _, test := range rlsTests(f) {
		f.Add(test.s)
	}
	for _, s := range []string{
		"", "x", "HDR10+", "HDR10+.x", "DD+5.1", "WEB-DL_x", "xvid_iso", "Remaſter", "M\u212aV", "MULTi9", "1600p",
		"Dual Audio", "DTS-HD.MA.7.1", "Blu-ray", "BLU RAY", "h.264", "H 265", "10bit", "REPACK3", "re-packed",
	} {
		f.Add(s)
	}
	fast, slow := parsers()
	f.Fuzz(func(t *testing.T, s string) {
		sameTags(t, fast, slow, s)
	})
}

// FuzzParseHint checks that no name and no hint can panic the parser, a wrong
// type for the name least of all: the steps after the type is known index the
// tags they expect a release of that type to have. The tags still reproduce
// the input, since a hint changes how tags are read and never their text.
func FuzzParseHint(f *testing.F) {
	for i, test := range rlsTests(f) {
		f.Add(test.s, uint8(i), i%2 == 0)
	}
	f.Add("", uint8(Magazine), true)
	f.Add("05.06.2024", uint8(Magazine), true)
	f.Add("Mag.S2024E05.2024", uint8(Magazine), true)
	f.Fuzz(func(t *testing.T, s string, typ uint8, dayFirst bool) {
		r := ParseStringHint(s, Hint{Type: Type(typ % uint8(Series+1)), DayFirst: dayFirst, Sports: typ&0x80 != 0})
		if o := fmt.Sprintf("%o", r); o != s {
			t.Errorf("round trip failed:\n in: %q\nout: %q", s, o)
		}
		exercise(r)
	})
}
