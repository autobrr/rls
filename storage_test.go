package rls

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

// storageNames are names that between them reach every lexer. They are
// literals, so the linker puts them in read-only memory.
var storageNames = []string{
	"[SubsPlease] Show Title - 21.5v2 (1080p) [ABCD1234].mkv",
	"Show.Title.S01E02-E04.The.Episode.2160p.UHD.BluRay.REMUX.HDR10+.DV.TrueHD.7.1.Atmos.HEVC-GRP",
	"Movie.Title.2019.Directors.Cut.MULTi5.1080p60.WEB-DL.DDP5.1.H.264.REPACK3-GRP.mkv",
	"Artist_Name-Album_Title-2CD-FLAC-2020-GRP",
	"-={site}=- Some.Game.v1.2.3.Plus.10.Trainer.x64.MacOS-GRP {{pass}} [[k:v]] (REQ)",
	"Event.2024-25.Round.05.Team.A.vs.Team.B.12.10.2024.720p50.HDTV.x264-GRP",
	"Comic Title 001 (2024) (digital) (Scan-Team).cbr",
	"The.Show.1x05.BDRemux.DTS-HD.MA.5.1.25th.Anniversary.Edition.12.inch.vinyl.4.7GB.D01",
	"\t [Group] Title_With_Underscores,and+plus - 01 ~ 12 [BD 1920x1080 x265 10bit FLAC] \n",
}

// TestParseOwnsName checks that a parse keeps nothing of what it was given: the
// bytes can be written over and the string is not the one the tags hold.
func TestParseOwnsName(t *testing.T) {
	for _, name := range storageNames {
		b := []byte(name)
		tags, _ := DefaultParser.Parse(b)
		r := Parse(b)
		h := ParseHint(b, Hint{Type: Movie, DayFirst: true})
		exp := fmt.Sprintf("%q %q %q %q %q", tags, r.Tags(), h.Tags(), r.Title, h.Subtitle)
		for i := range b {
			b[i] = 'x'
		}
		if s := fmt.Sprintf("%q %q %q %q %q", tags, r.Tags(), h.Tags(), r.Title, h.Subtitle); s != exp {
			t.Errorf("%q: expected the tags to keep their values when the bytes change\n%s\ngot:\n%s", name, exp, s)
		}
		// a string made at run time, which a release must not point into
		s := strings.Clone(name)
		lo := uintptr(unsafe.Pointer(unsafe.StringData(s)))
		tags, _ = ParseTagsString(s)
		for _, tags := range [][]Tag{tags, ParseString(s).Tags(), ParseStringHint(s, Hint{}).Tags()} {
			for _, tag := range tags {
				for _, v := range tag.v {
					if p := uintptr(unsafe.Pointer(unsafe.StringData(v))); v != "" && lo <= p && p < lo+uintptr(len(s)) {
						t.Fatalf("%q: expected no value in the string given, got: %q", name, v)
					}
				}
			}
		}
	}
}

// TestParseSharesName checks what the storage is for: the text of every tag
// of a parse is a piece of one copy of the name.
func TestParseSharesName(t *testing.T) {
	p := NewParser()
	for _, name := range storageNames {
		own := []byte(name)
		tags, _ := p.parse(own)
		lo := uintptr(unsafe.Pointer(unsafe.SliceData(own)))
		for _, tag := range tags {
			if v := tag.v[0]; v != "" {
				if p := uintptr(unsafe.Pointer(unsafe.StringData(v))); p < lo || lo+uintptr(len(own)) < p+uintptr(len(v)) {
					t.Errorf("%q: expected the text %q to be a piece of the name", name, v)
				}
			}
		}
	}
}

// TestParseReadOnly parses names held in read-only memory, which a lexer that
// wrote to its input would fault on.
func TestParseReadOnly(t *testing.T) {
	p := NewParser()
	for _, name := range storageNames {
		tags, end := p.parse(unsafe.Slice(unsafe.StringData(name), len(name)))
		r := p.builder.Build(tags, end)
		if s := fmt.Sprintf("%o", r); s != name {
			t.Errorf("expected %q, got: %q", name, s)
		}
		exercise(r)
	}
}

// TestParseAllocs keeps the allocations of a parse, and the size of what it
// allocates most of, from creeping back up.
func TestParseAllocs(t *testing.T) {
	// a release holds a tag for every token of its name, so the size of one
	// is most of what a kept release costs
	if n := unsafe.Sizeof(Tag{}); n > 72 {
		t.Errorf("expected a tag of at most 72 bytes, got: %d", n)
	}
	if poolDrops() {
		t.Skip("the regexp keeps its machines in a pool, which drops them under the race detector")
	}
	const name = "Show.Title.S01E02.The.Episode.1080p.WEB-DL.DDP5.1.H.264-GRP"
	b := []byte(name)
	// 33 when this was written: the name with its working copy, the tags,
	// for each of the 20 tags its values, and what the regexps that match
	// give back
	if n := testing.AllocsPerRun(100, func() { DefaultParser.Parse(b) }); n > 34 {
		t.Errorf("expected at most 34 allocations for the tags, got: %v", n)
	}
	if n := testing.AllocsPerRun(100, func() { ParseTagsString(name) }); n > 34 {
		t.Errorf("expected at most 34 allocations for the tags of a string, got: %v", n)
	}
	// 49 when this was written
	if n := testing.AllocsPerRun(100, func() { ParseString(name) }); n > 51 {
		t.Errorf("expected at most 51 allocations for the release, got: %v", n)
	}
}

// scratchNames are names that leave the scratch of a parse in every state:
// short ones, one with more tags than the scratch has room for at first, one
// that is longer than the scratch that is kept, and ones whose tags are read
// from the end.
var scratchNames = sync.OnceValue(func() []string {
	return append(slices.Clone(storageNames),
		"x",
		"",
		strings.Repeat("a.b-c_d ", 40)+"1080p.WEB-DL.x264-GRP",
		strings.Repeat("WEB-DL.", 2*maxScratch)+"mkv",
		"Title.2020.1080p.WEB-DL.x264-GRP"+strings.Repeat(" [site] {{pass}} [ABCD1234] (REQ).mkv", 12),
		"Title"+strings.Repeat("-", 3*maxScratch),
	)
})

// TestParseExactTags checks that the tags a parse gives have no room to spare
// and are a slice of their own: not the scratch they were lexed into, and not
// the tags of another parse.
func TestParseExactTags(t *testing.T) {
	var prev []Tag
	for _, name := range scratchNames() {
		for k, tags := range [][]Tag{
			tagsOf(DefaultParser.Parse([]byte(name))),
			tagsOf(ParseTagsString(name)),
			ParseString(name).Tags(),
			Parse([]byte(name)).Tags(),
			ParseStringHint(name, Hint{DayFirst: true}).Tags(),
			ParseHint([]byte(name), Hint{Type: Movie}).Tags(),
		} {
			if cap(tags) != len(tags) {
				t.Errorf("%.40q: parse %d: expected room for the %d tags and no more, got: %d", name, k, len(tags), cap(tags))
			}
			if len(tags) != 0 && len(prev) != 0 && &tags[0] == &prev[0] {
				t.Errorf("%.40q: parse %d: expected tags of its own, got those of the parse before", name, k)
			}
			if len(tags) != 0 {
				prev = tags
			}
		}
	}
}

// tagsOf returns the tags of a parse without where they end.
func tagsOf(tags []Tag, _ int) []Tag {
	return tags
}

// TestParseKeepsAfterReuse checks that a release and its tags stay what they
// were while later parses use the scratch again, and that the scratch gives
// every later parse what a first parse gives.
func TestParseKeepsAfterReuse(t *testing.T) {
	names := append(scratchNames(), benchCorpus(t)[:100]...)
	type kept struct {
		r    Release
		tags []Tag
		end  int
		// what they printed as right after the parse
		release, list string
	}
	keep := make([]kept, len(names))
	for round := range 3 {
		for k, name := range names {
			r := ParseString(name)
			tags, end := ParseTagsString(name)
			if round == 0 {
				keep[k] = kept{r, tags, end, fmt.Sprintf("%q %q", r.Tags(), r.Title), tagList(tags)}
				continue
			}
			// every parse of a name gives what the first one gave
			if !sameRelease(r, keep[k].r) {
				t.Fatalf("%.40q: round %d: expected the release of the first parse:%s", name, round, diffRelease(r, keep[k].r))
			}
			if !slices.EqualFunc(tags, keep[k].tags, sameTag) || end != keep[k].end {
				t.Fatalf("%.40q: round %d: expected the tags of the first parse\n%s\ngot:\n%s", name, round, tagList(keep[k].tags), tagList(tags))
			}
		}
	}
	// and what was kept is what it was
	for k, name := range names {
		if s := fmt.Sprintf("%q %q", keep[k].r.Tags(), keep[k].r.Title); s != keep[k].release {
			t.Errorf("%.40q: expected the kept release to stay\n%s\ngot:\n%s", name, keep[k].release, s)
		}
		if s := tagList(keep[k].tags); s != keep[k].list {
			t.Errorf("%.40q: expected the kept tags to stay\n%s\ngot:\n%s", name, keep[k].list, s)
		}
		if s := fmt.Sprintf("%o", keep[k].r); s != name {
			t.Errorf("%.40q: expected the kept release to print as the name, got: %.40q", name, s)
		}
		exercise(keep[k].r)
	}
}

// TestParseConcurrent checks that parses running at once, which share the
// parser, its finders and the pool of scratch, give what the same parses give
// one after the other.
func TestParseConcurrent(t *testing.T) {
	names := append(scratchNames(), benchCorpus(t)...)
	if testing.Short() {
		names = names[:len(scratchNames())+150]
	}
	p := NewParser()
	exp := make([]Release, len(names))
	for k, name := range names {
		exp[k] = p.ParseRelease([]byte(name))
	}
	const workers = 8
	got := make([][]Release, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			got[w] = make([]Release, len(names))
			// each worker starts somewhere else, so that a name meets every
			// other name in the scratch
			for k := range names {
				k = (k + w*len(names)/workers) % len(names)
				switch (k + w) % 3 {
				case 0:
					got[w][k] = p.ParseRelease([]byte(names[k]))
				case 1:
					got[w][k] = p.ParseHint([]byte(names[k]), Hint{})
				default:
					got[w][k] = p.builder.Build(p.Parse([]byte(names[k])))
				}
			}
		})
	}
	wg.Wait()
	for w := range workers {
		for k, name := range names {
			if !sameRelease(got[w][k], exp[k]) {
				t.Fatalf("%.40q: worker %d: expected the release of a parse on its own:%s", name, w, diffRelease(got[w][k], exp[k]))
			}
			if s := fmt.Sprintf("%o", got[w][k]); s != name {
				t.Fatalf("%.40q: worker %d: expected the release to print as the name, got: %.40q", name, w, s)
			}
		}
	}
}

// TestParseCallerLexerKeepsTags checks that a parser with a lexer of a
// caller's does not lex into scratch that later parses use again: the slices
// such a lexer was handed, which it may keep, stay what they were.
func TestParseCallerLexerKeepsTags(t *testing.T) {
	if !NewParser().pooled {
		t.Errorf("expected a parser with the default lexers to lex into scratch")
	}
	var kept [][]Tag
	var lists []string
	keeper := func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
		kept, lists = append(kept, start, end), append(lists, tagList(start), tagList(end))
		return start, end, i, n, false
	}
	wrapped := NewRegexpLexer(TagTypeContainer, true)
	inner := wrapped.Lex
	wrapped.Lex = func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
		keeper(src, buf, start, end, i, n)
		return inner(src, buf, start, end, i, n)
	}
	for name, lexers := range map[string][]Lexer{
		"a lexer of its own":         append(DefaultLexers(), TagLexer{Lex: keeper}),
		"a lexer that runs once":     append([]Lexer{TagLexer{Lex: keeper, Once: true}}, DefaultLexers()...),
		"a wrapped lex func":         append(DefaultLexers(), wrapped),
		"a lex func handed on as is": append(DefaultLexers(), handedOn{TagLexer{Lex: keeper}}),
	} {
		p := NewParser(WithLexers(lexers...))
		if p.pooled {
			t.Errorf("%s: expected the parser not to lex into scratch", name)
		}
		kept, lists = nil, nil
		var releases []Release
		for range 2 {
			for _, name := range scratchNames() {
				releases = append(releases, p.ParseRelease([]byte(name)))
			}
		}
		if len(kept) == 0 {
			t.Errorf("%s: expected the lexer to be called", name)
		}
		for k := range kept {
			if s := tagList(kept[k]); s != lists[k] {
				t.Fatalf("%s: expected the tags the lexer kept to stay\n%.300s\ngot:\n%.300s", name, lists[k], s)
			}
		}
		for k, r := range releases {
			if exp := DefaultParser.ParseRelease([]byte(scratchNames()[k%len(scratchNames())])); !sameRelease(r, exp) {
				t.Fatalf("%s: expected the release of the default parser:%s", name, diffRelease(r, exp))
			}
		}
	}
	// a lexer of this package under a type of a caller's is still one of
	// this package's
	if !NewParser(WithLexers(append(DefaultLexers(), handedOn{NewEpisodeLexer()})...)).pooled {
		t.Errorf("expected a parser with a lexer of this package handed on to lex into scratch")
	}
}

// TestTagScratch checks the room a parse lexes into: that it is enough for a
// name without growing, that a name too long for the scratch that is kept
// gets room of its own, and that the scratch is given back empty, whatever
// the lexers made of it.
func TestTagScratch(t *testing.T) {
	tag := NewTag(TagTypeText, nil, []byte("x"), []byte("x"))
	empty := func(tags []Tag) bool {
		return !slices.ContainsFunc(tags[:cap(tags)], func(tag Tag) bool { return tag.v != nil })
	}
	sc := new(tagScratch)
	for _, n := range []int{0, 60, 300, 2*maxScratch - 16} {
		start, end := sc.take(n)
		if len(start) != 0 || len(end) != 0 || cap(start) < n/2+8 || cap(end) < endScratch {
			t.Fatalf("%d bytes: expected empty room for %d and %d tags, got: %d of %d and %d of %d", n, n/2+8, endScratch, len(start), cap(start), len(end), cap(end))
		}
		if unsafe.SliceData(start) != unsafe.SliceData(sc.start) || unsafe.SliceData(end) != unsafe.SliceData(sc.end) {
			t.Fatalf("%d bytes: expected the room of the scratch", n)
		}
		for range n / 3 {
			start = append(start, tag)
		}
		sc.give(start, append(end, tag, tag))
		if !empty(sc.start) || !empty(sc.end) {
			t.Fatalf("%d bytes: expected the scratch to be given back empty", n)
		}
	}
	// a long name gets room that is not kept
	start, end := sc.take(2*maxScratch + 2)
	if cap(start) <= maxScratch || cap(sc.start) > maxScratch || unsafe.SliceData(start) == unsafe.SliceData(sc.start) {
		t.Fatalf("expected room of its own for a long name, got: %d, and %d in the scratch", cap(start), cap(sc.start))
	}
	sc.give(append(start, tag), end)
	if !empty(sc.start) || !empty(sc.end) {
		t.Fatalf("expected the scratch to be given back empty after a long name")
	}
	// lexers that outgrow the room, and lexers that hand back a slice of
	// their own, leave the scratch as it was made, and empty
	room := unsafe.SliceData(sc.start)
	start, end = sc.take(0)
	for range cap(start) + 1 {
		start = append(start, tag)
	}
	if unsafe.SliceData(start) == room {
		t.Fatalf("expected the tags to outgrow the room")
	}
	sc.give(start, []Tag{tag, tag})
	if unsafe.SliceData(sc.start) != room || !empty(sc.start) || !empty(sc.end) || cap(sc.end) != endScratch {
		t.Fatalf("expected the scratch to keep its own room, empty")
	}
	if start, _ = sc.take(0); !empty(start) {
		t.Fatalf("expected empty room after the lexers left it")
	}
}
