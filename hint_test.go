package rls

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/autobrr/rls/taginfo"
)

// sameRelease reports whether a and b are the same release: the same exported
// fields, and the same tags, unused text and dates. reflect.DeepEqual cannot
// compare two releases whole, as a tag holds the find func of its type, and
// func values are never deeply equal. Two tags are the same when they have
// the same type, values and previous type, and find the same tag infos by
// their tag, which also holds for two parsers that loaded the infos apart.
func sameRelease(a, b Release) bool {
	if !slices.EqualFunc(a.tags, b.tags, sameTag) ||
		!slices.Equal(a.dates, b.dates) ||
		!slices.Equal(a.unused, b.unused) ||
		a.end != b.end {
		return false
	}
	a.tags, a.dates, a.unused, a.end = nil, nil, nil, 0
	b.tags, b.dates, b.unused, b.end = nil, nil, nil, 0
	return reflect.DeepEqual(a, b)
}

// sameTag reports whether a and b are the same tag, as sameRelease means it.
func sameTag(a, b Tag) bool {
	info := func(t *taginfo.Taginfo) string {
		if t == nil {
			return ""
		}
		return t.Tag()
	}
	return a.typ == b.typ && a.was == b.was && slices.Equal(a.v, b.v) &&
		(a.f == nil) == (b.f == nil) && (a.prevf == nil) == (b.prevf == nil) &&
		(a.info == nil) == (b.info == nil) && info(a.info) == info(b.info) &&
		(a.ninfo == nil) == (b.ninfo == nil) && info(a.ninfo) == info(b.ninfo)
}

// diffRelease shows a and b for a failed comparison.
func diffRelease(a, b Release) string {
	return fmt.Sprintf("\n  got: %#v\n  exp: %#v", a, b)
}

func TestNewParserIsDefault(t *testing.T) {
	t.Parallel()
	tests := rlsTests(t)
	parsers := []struct {
		name string
		p    *TagParser
	}{
		{"no options", NewParser()},
		{"default lexers", NewParser(WithLexers(DefaultLexers()...))},
		{"no lexers", NewParser(WithLexers())},
		{"nil taginfo", NewParser(WithTaginfo(nil))},
	}
	for _, pp := range parsers {
		t.Run(pp.name, func(t *testing.T) {
			t.Parallel()
			for i, test := range tests {
				src := []byte(test.s)
				if r, exp := pp.p.ParseRelease(src), DefaultParser.ParseRelease(src); !sameRelease(r, exp) {
					t.Errorf("test %d %q differs from the default parser:%s", i, test.s, diffRelease(r, exp))
				}
			}
		})
	}
}

// extraCollections builds a collection map from the given collections the way
// autobrr adds the streaming services it knows.
func extraCollections(t *testing.T, collections ...[]string) map[string][]*taginfo.Taginfo {
	t.Helper()
	extra := make(map[string][]*taginfo.Taginfo)
	for _, c := range collections {
		info, err := taginfo.New(c...)
		if err != nil {
			t.Fatal(err)
		}
		extra["collection"] = append(extra["collection"], info)
	}
	return extra
}

func TestNewParserWithTaginfo(t *testing.T) {
	t.Parallel()
	footv := []string{"FOOTV", "Foo TV", "(?-i:FOOTV)", "", "", ""}
	bartv := []string{"BARTV", "Bar TV", "(?-i:BARTV)", "", "", ""}
	const name = "Show.S01E02.1080p.FOOTV.WEB-DL.AAC2.0.H.264-GRP"
	const other = "Show.S01E02.1080p.BARTV.WEB-DL.AAC2.0.H.264-GRP"
	// the default parser does not know either service
	for _, s := range []string{name, other} {
		if r := ParseString(s); r.Collection != "" {
			t.Fatalf("%q: the default parser knows collection %q, pick another for this test", s, r.Collection)
		}
	}
	one := NewParser(WithTaginfo(extraCollections(t, footv)))
	if r := one.ParseRelease([]byte(name)); r.Collection != "FOOTV" {
		t.Errorf("%q: expected collection %q, got: %q", name, "FOOTV", r.Collection)
	}
	// the name without the service reads the same with either parser
	const without = "Show.S01E02.1080p.WEB-DL.AAC2.0.H.264-GRP"
	if r, exp := one.ParseRelease([]byte(without)), ParseString(without); !sameRelease(r, exp) {
		t.Errorf("%q: differs from the default parser:%s", without, diffRelease(r, exp))
	}
	// two options add up
	two := NewParser(WithTaginfo(extraCollections(t, footv)), WithTaginfo(extraCollections(t, bartv)))
	for s, c := range map[string]string{name: "FOOTV", other: "BARTV"} {
		if r := two.ParseRelease([]byte(s)); r.Collection != c {
			t.Errorf("%q: expected collection %q, got: %q", s, c, r.Collection)
		}
	}
	if r := one.ParseRelease([]byte(other)); r.Collection != "" {
		t.Errorf("%q: expected no collection with one option, got: %q", other, r.Collection)
	}
	// every name that carries neither service reads as the default parser
	// reads it
	for i, test := range rlsTests(t) {
		if strings.Contains(test.s, "FOOTV") || strings.Contains(test.s, "BARTV") {
			continue
		}
		src := []byte(test.s)
		if r, exp := two.ParseRelease(src), DefaultParser.ParseRelease(src); !sameRelease(r, exp) {
			t.Errorf("test %d %q differs from the default parser:%s", i, test.s, diffRelease(r, exp))
		}
	}
}

func TestParseHintZero(t *testing.T) {
	t.Parallel()
	p := DefaultParser.(*TagParser)
	for i, test := range rlsTests(t) {
		src := []byte(test.s)
		exp := p.ParseRelease(src)
		for name, r := range map[string]Release{
			"method":          p.ParseHint(src, Hint{}),
			"ParseHint":       ParseHint(src, Hint{}),
			"ParseStringHint": ParseStringHint(test.s, Hint{}),
		} {
			if !sameRelease(r, exp) {
				t.Errorf("test %d %q %s differs from ParseRelease:%s", i, test.s, name, diffRelease(r, exp))
			}
		}
	}
}

// TestParseHintTypeAll checks that any type given for any name is the type of
// the release, and that nothing panics on a type that does not fit the name.
func TestParseHintTypeAll(t *testing.T) {
	t.Parallel()
	p := DefaultParser.(*TagParser)
	tests := rlsTests(t)
	for typ := Unknown + 1; typ <= Series; typ++ {
		for i, test := range tests {
			if r := p.ParseHint([]byte(test.s), Hint{Type: typ, DayFirst: true}); r.Type != typ {
				t.Errorf("test %d %q hinted %s: got type %s", i, test.s, typ, r.Type)
			}
		}
	}
}

func TestParseHintType(t *testing.T) {
	t.Parallel()
	p := DefaultParser.(*TagParser)
	tests := []struct {
		s string
		// own is the type the parser picks on its own
		own Type
		typ Type
		// check holds the fields the hinted parse must have
		check func(Release) bool
	}{
		// a music release whose 'BD' reads as a video source: an album of
		// 'Artist' named 'Album' from a Blu-ray disc
		{
			"Artist-Album-(BD)-2020-GRP", Movie, Music,
			func(r Release) bool {
				return r.Artist == "Artist" && r.Title == "Album" && r.Year == 2020 && r.Source == "BluRay" && r.Group == "GRP"
			},
		},
		// a film with no tags but its year is read as music
		{
			"Live.Die.Repeat.2014", Music, Movie,
			func(r Release) bool {
				return r.Artist == "" && r.Title == "Live Die Repeat" && r.Year == 2014
			},
		},
		// a film from the web with no resolution is read as music
		{
			"Movie.Title.2020.WEB-GRP", Music, Movie,
			func(r Release) bool {
				return r.Title == "Movie Title" && r.Year == 2020 && r.Source == "WEB" && r.Group == "GRP"
			},
		},
		// a film called 'The Mix' is read as a remix, and as a film the
		// word is only its title
		{
			"The.Mix.2021.DVDRip.XviD-GRP", Music, Movie,
			func(r Release) bool {
				return r.Title == "The Mix" && r.Year == 2021 && r.Source == "DVDRiP" &&
					slices.Equal(r.Codec, []string{"XViD"}) && len(r.Other) == 0 && r.Group == "GRP"
			},
		},
		// a special of a show, with no episode number, is read as a film
		{
			"Show.Name.2023.Christmas.Special.720p.WEB.h264-GRP", Movie, Episode,
			func(r Release) bool {
				return r.Title == "Show Name" && r.Subtitle == "Christmas Special" && r.Year == 2023 &&
					r.Resolution == "720p" && r.Source == "WEB" && r.Group == "GRP"
			},
		},
	}
	for _, test := range tests {
		if r := p.ParseRelease([]byte(test.s)); r.Type != test.own {
			t.Errorf("%q: expected the parser to pick %s on its own, got: %s", test.s, test.own, r.Type)
		}
		r := p.ParseHint([]byte(test.s), Hint{Type: test.typ})
		if r.Type != test.typ {
			t.Errorf("%q: expected type %s, got: %s", test.s, test.typ, r.Type)
		}
		if !test.check(r) {
			t.Errorf("%q hinted %s: unexpected fields:\n%#v", test.s, test.typ, r)
		}
		if s := fmt.Sprintf("%o", r); s != test.s {
			t.Errorf("%q: expected %%o to be the name, got: %q", test.s, s)
		}
	}
}

func TestParseHintTypeAgrees(t *testing.T) {
	t.Parallel()
	p := DefaultParser.(*TagParser)
	for _, test := range []struct {
		s   string
		typ Type
	}{
		{"Movie.2020.1080p.BluRay.x264-GRP", Movie},
		{"Show.S01E02.1080p.WEB.h264-GRP", Episode},
		{"Artist-Album-WEB-2020-GRP", Music},
	} {
		exp := p.ParseRelease([]byte(test.s))
		if exp.Type != test.typ {
			t.Fatalf("%q: expected the parser to pick %s, got: %s", test.s, test.typ, exp.Type)
		}
		if r := p.ParseHint([]byte(test.s), Hint{Type: test.typ}); !sameRelease(r, exp) {
			t.Errorf("%q: hinted %s differs from the parse without it:%s", test.s, test.typ, diffRelease(r, exp))
		}
	}
}

// TestParseHintOwnType checks that a hint which agrees with the parser changes
// nothing: every test name parsed with the type the parser picks on its own
// is the release the parser gives without a hint.
func TestParseHintOwnType(t *testing.T) {
	t.Parallel()
	p := DefaultParser.(*TagParser)
	for _, test := range rlsTests(t) {
		exp := p.ParseRelease([]byte(test.s))
		if got := p.ParseHint([]byte(test.s), Hint{Type: exp.Type}); !sameRelease(got, exp) {
			t.Errorf("%q hinted %s:%s", test.s, exp.Type, diffFields(got, exp))
		}
	}
}

// diffFields lists the exported fields that differ between a and b.
func diffFields(a, b Release) string {
	var s string
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := range av.NumField() {
		if f := av.Type().Field(i); f.IsExported() && !reflect.DeepEqual(av.Field(i).Interface(), bv.Field(i).Interface()) {
			s += fmt.Sprintf("\n  %s: got %v, expected %v", f.Name, av.Field(i).Interface(), bv.Field(i).Interface())
		}
	}
	return s + fmt.Sprintf("\n  tags: got %e\n   expected %e", a, b)
}

// TestNewParserWithLexers checks that the lexers given are the ones used: with
// the one lexer that trims the ends of a name, nothing in it is a tag.
func TestNewParserWithLexers(t *testing.T) {
	t.Parallel()
	const name = "Show.S01E02.1080p.WEB.h264-GRP"
	r := NewParser(WithLexers(NewTrimWhitespaceLexer())).ParseRelease([]byte(name))
	if r.Series != 0 || r.Episode != 0 || r.Resolution != "" || r.Source != "" || len(r.Codec) != 0 {
		t.Errorf("expected no season, episode, resolution, source or codec, got: %d %d %q %q %q", r.Series, r.Episode, r.Resolution, r.Source, r.Codec)
	}
	for _, tag := range r.Tags() {
		if !tag.Is(TagTypeText, TagTypeDelim, TagTypeWhitespace) {
			t.Errorf("expected text and delimiters alone, got: %e", tag)
		}
	}
	if s := fmt.Sprintf("%o", r); s != name {
		t.Errorf("expected %%o to be the name, got: %q", s)
	}
	// a later option replaces an earlier one
	r = NewParser(WithLexers(NewTrimWhitespaceLexer()), WithLexers(DefaultLexers()...)).ParseRelease([]byte(name))
	if r.Series != 1 || r.Episode != 2 || r.Resolution != "1080p" {
		t.Errorf("expected S01E02 in 1080p, got: %d %d %q", r.Series, r.Episode, r.Resolution)
	}
}

// TestNewParserNilTaginfo checks that an info that failed to build does not
// take the parser down with it.
func TestNewParserNilTaginfo(t *testing.T) {
	t.Parallel()
	bad, err := taginfo.New("X", "X", "(", "", "", "")
	if err == nil || bad != nil {
		t.Fatalf("expected taginfo.New to fail on the regexp '(', got: %v, %v", bad, err)
	}
	extra := extraCollections(t, []string{"FOOTV", "FOOTV", "", "", "", ""})
	extra["collection"] = append(extra["collection"], nil)
	const name = "Show.S01E02.1080p.FOOTV.WEB-DL.AAC2.0.H.264-GRP"
	if r := NewParser(WithTaginfo(extra)).ParseRelease([]byte(name)); r.Collection != "FOOTV" {
		t.Errorf("expected collection FOOTV, got: %q", r.Collection)
	}
}

// TestParseHintTypeOutOfRange checks that a type that is not a release type
// lets the parser decide.
func TestParseHintTypeOutOfRange(t *testing.T) {
	t.Parallel()
	p := DefaultParser.(*TagParser)
	const name = "Movie.2020.1080p.BluRay.x264-GRP"
	exp := p.ParseRelease([]byte(name))
	for _, typ := range []Type{-1, Series + 1, 99} {
		if r := p.ParseHint([]byte(name), Hint{Type: typ}); !sameRelease(r, exp) {
			t.Errorf("type %d: differs from the parse without a hint:%s", int(typ), diffRelease(r, exp))
		}
	}
}

// otherBuilder is a Builder that is not a *TagBuilder.
type otherBuilder struct{ b Builder }

func (o otherBuilder) Build(tags []Tag, end int) Release { return o.b.Build(tags, end) }

// TestParseHintOtherBuilder checks that with a builder of the caller's own
// the type in the hint has no effect and DayFirst still does.
func TestParseHintOtherBuilder(t *testing.T) {
	t.Parallel()
	p := NewParser()
	p.SetBuilder(otherBuilder{NewTagBuilder().Init(taginfo.All())})
	const name = "UEFA.Champions.League.05.06.2024.Arsenal.vs.Chelsea.1080p.WEB.h264-GRP"
	r := p.ParseHint([]byte(name), Hint{Type: Music, DayFirst: true})
	if r.Type != Episode || r.Month != 6 || r.Day != 5 {
		t.Errorf("expected the episode of 5 June, got: %s %d-%d", r.Type, r.Month, r.Day)
	}
}

func TestParseHintTypeWrong(t *testing.T) {
	t.Parallel()
	const name = "Movie.2020.1080p.BluRay.x264-GRP"
	r := DefaultParser.(*TagParser).ParseHint([]byte(name), Hint{Type: Music})
	if r.Type != Music {
		t.Errorf("%q: expected type %s, got: %s", name, Music, r.Type)
	}
	if s := fmt.Sprintf("%o", r); s != name {
		t.Errorf("%q: expected %%o to be the name, got: %q", name, s)
	}
}

func TestParseHintDayFirst(t *testing.T) {
	t.Parallel()
	p := DefaultParser.(*TagParser)
	const format = "UEFA.Champions.League.%s.Arsenal.vs.Chelsea.1080p.WEB.h264-GRP"
	tests := []struct {
		date string
		// month and day without the hint, then with it
		month, day     int
		dfMonth, dfDay int
	}{
		{"05.06.2024", 5, 6, 6, 5},
		{"05-06-2024", 5, 6, 6, 5},
		{"05 06 2024", 5, 6, 6, 5},
		{"05_06_2024", 5, 6, 6, 5},
		{"05,06,2024", 5, 6, 6, 5},
		{"05+06+2024", 5, 6, 6, 5},
		{"13.06.2024", 6, 13, 6, 13},
		{"13-06-2024", 6, 13, 6, 13},
		{"06.13.2024", 6, 13, 6, 13},
		{"06 13 2024", 6, 13, 6, 13},
		{"2024.06.05", 6, 5, 6, 5},
		{"2024-06-05", 6, 5, 6, 5},
		{"05.05.2024", 5, 5, 5, 5},
		{"12.01.2024", 12, 1, 1, 12},
		{"5th.Jun.2024", 6, 5, 6, 5},
		{"Jun.05.2024", 6, 5, 6, 5},
	}
	for _, test := range tests {
		name := fmt.Sprintf(format, test.date)
		for _, h := range []struct {
			hint       Hint
			month, day int
		}{
			{Hint{}, test.month, test.day},
			{Hint{DayFirst: true}, test.dfMonth, test.dfDay},
		} {
			r := p.ParseHint([]byte(name), h.hint)
			if r.Year != 2024 || r.Month != h.month || r.Day != h.day {
				t.Errorf("%q %+v: expected 2024-%02d-%02d, got: %d-%02d-%02d", name, h.hint, h.month, h.day, r.Year, r.Month, r.Day)
			}
			if r.Type != Episode || r.Title != "UEFA Champions League" {
				t.Errorf("%q %+v: expected the episode 'UEFA Champions League', got: %s %q", name, h.hint, r.Type, r.Title)
			}
			if s := fmt.Sprintf("%o", r); s != name {
				t.Errorf("%q %+v: expected %%o to be the name, got: %q", name, h.hint, s)
			}
			if len(r.Dates()) == 0 {
				t.Errorf("%q %+v: expected a date tag", name, h.hint)
			}
		}
	}
	// a two-digit year is not touched
	const short = "Show.24.05.06.1080p.WEB.h264-GRP"
	if r, exp := p.ParseHint([]byte(short), Hint{DayFirst: true}), p.ParseRelease([]byte(short)); !sameRelease(r, exp) {
		t.Errorf("%q: day first differs from the parse without it:%s", short, diffRelease(r, exp))
	}
	// a name with no date reads the same
	for _, s := range []string{
		"Show.S01E02.1080p.WEB.h264-GRP",
		"Movie.2020.1080p.BluRay.x264-GRP",
		"Artist-Album-WEB-2020-GRP",
	} {
		if r, exp := p.ParseHint([]byte(s), Hint{DayFirst: true}), p.ParseRelease([]byte(s)); !sameRelease(r, exp) {
			t.Errorf("%q: day first differs from the parse without it:%s", s, diffRelease(r, exp))
		}
	}
}

// plainParser is a Parser without a ParseHint method.
type plainParser struct{ parser Parser }

func (p plainParser) Parse(src []byte) ([]Tag, int) { return p.parser.Parse(src) }

func (p plainParser) ParseRelease(src []byte) Release { return p.parser.ParseRelease(src) }

// TestParseHintFallback replaces DefaultParser, so it does not run in
// parallel.
func TestParseHintFallback(t *testing.T) {
	saved := DefaultParser
	t.Cleanup(func() { DefaultParser = saved })
	DefaultParser = plainParser{saved}
	const name = "UEFA.Champions.League.05.06.2024.Arsenal.vs.Chelsea.1080p.WEB.h264-GRP"
	h := Hint{Type: Music, DayFirst: true}
	exp := saved.ParseRelease([]byte(name))
	for fn, r := range map[string]Release{
		"ParseHint":       ParseHint([]byte(name), h),
		"ParseStringHint": ParseStringHint(name, h),
	} {
		if !sameRelease(r, exp) {
			t.Errorf("%s: expected the hint to be ignored:%s", fn, diffRelease(r, exp))
		}
		if r.Type != Episode || r.Month != 5 || r.Day != 6 {
			t.Errorf("%s: expected the episode of 6 May, got: %s %d-%d", fn, r.Type, r.Month, r.Day)
		}
	}
}

// poolDrops reports whether a sync.Pool drops what is put in it, as it does at
// random under the race detector. The normalizers keep their buffers in a
// pool, so the allocations of a parse then vary from run to run.
func poolDrops() bool {
	pool := sync.Pool{New: func() any { return new([64]byte) }}
	// a quarter of the puts are dropped, and AllocsPerRun rounds the average
	// down, so each run puts many
	return testing.AllocsPerRun(10, func() {
		for range 100 {
			pool.Put(pool.Get())
		}
	}) != 0
}

func TestParseHintAllocs(t *testing.T) {
	if poolDrops() {
		t.Skip("sync.Pool drops items, so allocations cannot be compared")
	}
	p := DefaultParser.(*TagParser)
	for _, s := range []string{
		"Show.S01E02.1080p.WEB.h264-GRP",
		"UEFA.Champions.League.05.06.2024.Arsenal.vs.Chelsea.1080p.WEB.h264-GRP",
		"Artist-Album-WEB-2020-GRP",
	} {
		src := []byte(s)
		release := testing.AllocsPerRun(100, func() { _ = p.ParseRelease(src) })
		hint := testing.AllocsPerRun(100, func() { _ = p.ParseHint(src, Hint{}) })
		if hint > release {
			t.Errorf("%q: ParseHint allocates %v, ParseRelease %v", s, hint, release)
		}
	}
}

// TestParseLexerNoProgress checks that a lexer that reports a match and
// consumes nothing does not hang the parse: its match is dropped, and the
// input goes to the lexers after it.
func TestParseLexerNoProgress(t *testing.T) {
	var calls int
	stuck := TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			calls++
			return append(start, NewTag(TagTypeText, nil, nil, nil)), end, i, n, true
		},
	}
	back := TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			return start, end, i - 1, n + 1, true
		},
	}
	const name = "The.Matrix.1999.1080p.BluRay.x264-GRP"
	p := NewParser(WithLexers(append([]Lexer{stuck, back}, DefaultLexers()...)...))
	done := make(chan Release, 1)
	go func() { done <- p.ParseRelease([]byte(name)) }()
	select {
	case r := <-done:
		if exp := ParseString(name); r.Title != exp.Title || r.Year != exp.Year || r.Group != exp.Group || len(r.Tags()) != len(exp.Tags()) {
			t.Errorf("expected the parse of the default lexers, got: %q %d %q, %d tags", r.Title, r.Year, r.Group, len(r.Tags()))
		}
		if s := fmt.Sprintf("%o", r); s != name {
			t.Errorf("expected %q, got: %q", name, s)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("parse did not return")
	}
	if calls == 0 {
		t.Error("expected the lexer to be called")
	}
}
