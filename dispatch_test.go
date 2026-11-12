package rls

import (
	"maps"
	"math/bits"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/autobrr/rls/taginfo"
)

// ungated returns a parser that tries every lexer at every position, for a
// test to compare with one that has the gates.
func ungated(opts ...ParserOption) *TagParser {
	noGates = true
	defer func() { noGates = false }()
	return NewParser(opts...)
}

// mutations returns names made from the given ones, each of them several
// times: with pieces cut out, cases swapped and fragments put in that the
// gates of the lexers look at. The same names come back on every run.
func mutations(names []string, each int) []string {
	frag := []string{
		"[", "]", "(", ")", "{", "}", "-", "_", ".", " ", "+", ",", "~", "/", "\t", "\u212a", "\u017f", "⭐", "é", "\xff", "!", "&", "#", "'", "@",
		"REQ", "[REQ]", "{{pw}}", "[[k:v]]", "[ABCD1234]", "-={site}=-", "_INT", "-CODEX", ".mkv", ".tar.gz", ".mKv", "1999", " 2020 ", "S01E02", " - 05 ", "- 1100 [",
		"(ABC-123)", "DDP5.1", "AACx264", "DTS-HD.MA.7.1", "Sep 2020", "12 Dec 2011", "01-August-1998", "v2", "10v2", "2xCD", "WEB2007", "4CD2003",
		"(Drama)", "Docu", "MULTi5", "12.inch.vinyl", "25th.Anniversary", "plus.10.trainer", "-sign-language", "19xx", "4.5GB", "1080p60", "23.976fps", "Season 2",
		"Ep. 07", "2x05", "Disc.1", "17.12.15", "11.09.1", "BDRemux", "\u017fDTV", "WE\u212a", "HDR10+", "DD+",
	}
	// a generator of its own, so that the names do not change with the one
	// of a Go release
	seed := uint64(20261112)
	rnd := func(n int) int {
		seed = seed*6364136223846793005 + 1442695040888963407
		return int((seed >> 33) % uint64(n))
	}
	mutate := func(s string) string {
		if s == "" {
			return frag[rnd(len(frag))]
		}
		p := rnd(len(s) + 1)
		switch rnd(8) {
		case 0:
			return s[:p] + frag[rnd(len(frag))] + s[p:]
		case 1:
			return s[:p] + s[min(len(s), p+1+rnd(7)):]
		case 2:
			return s[p:]
		case 3:
			return s[:p]
		case 4:
			return s + frag[rnd(len(frag))]
		case 5:
			return frag[rnd(len(frag))] + s
		case 6:
			p = min(p, len(s)-1)
			c := s[p]
			switch {
			case c >= 'a' && c <= 'z':
				c -= 32
			case c >= 'A' && c <= 'Z':
				c += 32
			}
			return s[:p] + string(c) + s[p+1:]
		}
		return s[:p] + frag[rnd(len(frag))] + frag[rnd(len(frag))] + s[p:]
	}
	var v []string
	for _, name := range names {
		for range each {
			s := name
			for range 1 + rnd(3) {
				s = mutate(s)
			}
			v = append(v, s)
		}
	}
	return v
}

// spellingNames returns every spelling a row of the vocabulary can be
// expanded to with what may stand around it, and the shapes of the patterns
// that are no rows.
func spellingNames() []string {
	var names []string
	affixes := []string{"", ".", "x", "_", "é", "-rest", "+", " 5.1", ")", "\u017f", "_x", ".1080p", "-", "é.x", "\xff", "\u212a", "!", "&x", "'s"}
	prefixes := []string{"Title.", "", "\u212a", "é", "!", "Title 日本語 ", "#", "Title.S01E02.", "Title - 05 - "}
	infos := taginfo.All()
	var row int
	for _, typ := range slices.Sorted(maps.Keys(infos)) {
		for _, info := range infos[typ] {
			v, err := expandRE(info.RE(), true)
			if err != nil {
				v = []string{info.Tag()}
			}
			for k, s := range v {
				// a third of the affixes and of the prefixes on the first
				// spelling of a row, in turn from row to row, and one of each
				// on the others
				if k == 0 {
					for l := row % 3; l < len(affixes); l += 3 {
						names = append(names, prefixes[0]+s+affixes[l])
					}
					for m := 1 + row%3; m < len(prefixes); m += 3 {
						names = append(names, prefixes[m]+s)
					}
				}
				names = append(names, prefixes[k%len(prefixes)]+s+affixes[(k/len(prefixes)+k)%len(affixes)])
			}
			row++
		}
	}
	for k, s := range []string{
		"S01E02", "s1e2", "2x05", "1080p60", "23.976fps", "Ep 5.5", "E05-07", "2nd Season", "Vol.1.No.2", "v2", "1.2.3", "CD1", "2xDVD9", "12 Jan 2020",
		"2020-01-02", "2020-21", "01-12", "- 05 -", "Season 1+2", "S01-03", "10v2", "4.5GB", "Folge 15", "S1957E01", "2024x05", "WEB2007", "BDRemux", "(ABC-123)",
	} {
		for l, affix := range affixes {
			for m, prefix := range prefixes {
				switch (k + l + m) % 3 {
				case 0:
					names = append(names, prefix+s+affix)
				case 1:
					names = append(names, prefix+strings.ToLower(s)+affix+" x")
				default:
					names = append(names, prefix+s+affix+".mkv")
				}
			}
		}
	}
	return names
}

var (
	sameNamesOnce sync.Once
	sameNames     []string
)

// sameParseNames returns the names the same-parse tests compare two parsers
// on: those of tests.yaml, names made from them, the spellings of the rows
// and, when the corpora are present and the test is not short, the corpora.
func sameParseNames(tb testing.TB) []string {
	sameNamesOnce.Do(func() {
		names := benchCorpus(tb)
		names = append(names, mutations(names, 6)...)
		names = append(names, spellingNames()...)
		if !testing.Short() {
			for _, file := range []string{"nyaa.txt", "sports.txt"} {
				v, _ := readCorpus(file)
				names = append(names, v...)
			}
		}
		sameNames = names
	})
	return sameNames
}

// TestGatesSameParse checks that the gates only save work: a parser that
// tries every lexer at every position gives the same tags for every name of
// tests.yaml, for names made from those, for every spelling a row can be
// expanded to with what may stand around it, and for the corpora when they
// are present.
func TestGatesSameParse(t *testing.T) {
	fast, slow := NewParser(), ungated()
	if all := uint64(1)<<len(slow.multi) - 1; fast.dispatch.lead[0] != 0 || slow.dispatch.lead[0] != all {
		t.Fatalf("expected gates on the one parser alone")
	}
	names := sameParseNames(t)
	for _, name := range names {
		sameTags(t, fast, slow, name)
	}
	t.Logf("%d names", len(names))
}

// TestDispatchGates checks that the dispatch holds what it is for: a gate for
// every default lexer, so that a lexer added without one, which is then
// called at every position, does not go unseen, and few lexers called for a
// word that is no tag.
func TestDispatchGates(t *testing.T) {
	p := NewParser()
	// no gate lets a NUL through, so a lexer that is called there has none
	if m := p.dispatch.lead[0]; m != 0 {
		t.Errorf("expected no lexer without a gate, got: %b", m)
	}
	for _, word := range []string{"Frieren", "the", "Bundesliga", "zzz"} {
		if m := p.dispatch.mask([]byte(word), 0, len(word)); bits.OnesCount64(m) > 8 {
			t.Errorf("%q: expected at most 8 of the %d lexers, got: %d (%b)", word, len(p.multi), bits.OnesCount64(m), m)
		}
	}
	for _, test := range []struct {
		m      uint64
		a, exp int
	}{
		{0, 0, 64}, {0, 64, 64}, {0, 70, 70}, {1, 0, 0}, {1, 1, 64}, {0b101000, 0, 3}, {0b101000, 4, 5}, {0b101000, 6, 64}, {1 << 63, 0, 63}, {1 << 63, 63, 63},
	} {
		if a := nextLexer(test.m, test.a); a != test.exp {
			t.Errorf("nextLexer(%b, %d): expected %d, got: %d", test.m, test.a, test.exp, a)
		}
	}
}

// TestDispatchManyLexers checks that the lexers past those a dispatch can
// tell apart are called.
func TestDispatchManyLexers(t *testing.T) {
	lexers := func() []Lexer {
		v := DefaultLexers()
		for len(v) < maxDispatch+8 {
			v = append(v, NewIDLexer())
		}
		// reads 'zzq', which nothing in front of it does
		return append(v, TagLexer{Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if j := i + 3; j <= n && string(src[i:j]) == "zzq" {
				return append(start, NewTag(TagTypeOther, nil, src[i:j], []byte("ZZQ"))), end, j, n, true
			}
			return start, end, i, n, false
		}}, NewEpisodeLexer())
	}
	fast, slow := NewParser(WithLexers(lexers()...)), ungated(WithLexers(lexers()...))
	if len(fast.multi) <= maxDispatch {
		t.Fatalf("expected more than %d lexers, got: %d", maxDispatch, len(fast.multi))
	}
	for _, name := range append(benchCorpus(t)[:100], "Title.zzq.1080p-GRP", "zzq", "Title - 05 zzq [1080p]") {
		sameTags(t, fast, slow, name)
	}
	if tags, _ := fast.Parse([]byte("Title.zzq.1080p-GRP")); !slices.ContainsFunc(tags, func(tag Tag) bool { return tag.Other() == "ZZQ" }) {
		t.Errorf("expected the tag of the last lexer, got: %s", tagList(tags))
	}
}

// TestDispatchCallerLexers checks that a lexer of a caller's is called at
// every position: one of its own, one that wraps the lex func of a lexer of
// this package, and one that hands that lex func on under its own type, which
// keeps its gate.
func TestDispatchCallerLexers(t *testing.T) {
	// a lexer that reads 'zzq' and words that begin with '!', which no gate
	// of this package lets through
	own := func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
		if j := i + 3; j <= n && (string(src[i:j]) == "zzq" || src[i] == '!') {
			return append(start, NewTag(TagTypeOther, nil, src[i:j], []byte("ZZQ"))), end, j, n, true
		}
		return start, end, i, n, false
	}
	wrapped := NewRegexpLexer(TagTypeContainer, true)
	inner := wrapped.Lex
	wrapped.Lex = func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
		if s, e, j, k, ok := own(src, buf, start, end, i, n); ok {
			return s, e, j, k, ok
		}
		return inner(src, buf, start, end, i, n)
	}
	tests := []struct {
		name   string
		lexers func() []Lexer
		gates  int
	}{
		{"a lex func of its own", func() []Lexer { return []Lexer{TagLexer{Lex: own}} }, 0},
		{"a wrapped lex func", func() []Lexer { return []Lexer{wrapped} }, 0},
		{"a lex func handed on", func() []Lexer {
			return []Lexer{handedOn{NewRegexpLexer(TagTypeContainer, true)}, TagLexer{Lex: own}}
		}, 1},
	}
	for _, test := range tests {
		front := len(test.lexers())
		fast := NewParser(WithLexers(append(test.lexers(), DefaultLexers()...)...))
		slow := ungated(WithLexers(append(test.lexers(), DefaultLexers()...)...))
		// the lexers in front are called everywhere unless they are of this
		// package
		var always int
		for a := range front {
			if fast.dispatch.lead[0]&(1<<a) != 0 {
				always++
			}
		}
		if exp := front - test.gates; always != exp {
			t.Errorf("%s: expected %d lexers without a gate, got: %d", test.name, exp, always)
		}
		for _, name := range []string{"Title.zzq.1080p-GRP", "zzq", "Title !ab mkv", "!zq.Title.2020.mkv.x264", "Title.S01E02.zzq!.mkv-GRP"} {
			sameTags(t, fast, slow, name)
			tags, _ := fast.Parse([]byte(name))
			if !slices.ContainsFunc(tags, func(tag Tag) bool { return tag.Is(TagTypeOther) && tag.Other() == "ZZQ" }) {
				t.Errorf("%s: %q: expected the tag of the caller's lexer, got: %s", test.name, name, tagList(tags))
			}
		}
	}
}

// handedOn is a lexer of a caller's that gives the lex func of another.
type handedOn struct {
	lexer Lexer
}

func (l handedOn) Initialize(infos map[string][]*taginfo.Taginfo, delim *regexp.Regexp, short map[string]bool) (LexFunc, bool, bool) {
	return l.lexer.Initialize(infos, delim, short)
}

// TestGateOffersCleared checks that a parser leaves nothing behind in the
// table its lexers offer their gates through.
func TestGateOffersCleared(t *testing.T) {
	var delim *regexp.Regexp
	var offered bool
	NewParser(WithLexers(append(DefaultLexers(), TagLexer{
		Init: func(_ map[string][]*taginfo.Taginfo, re *regexp.Regexp, _ map[string]bool) {
			delim = re
			_, offered = gateOffers.Load(re)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			return start, end, i, n, false
		},
	})...))
	if !offered {
		t.Errorf("expected the parser to take offers while it is built")
	}
	if _, ok := gateOffers.Load(delim); ok || delim == nil {
		t.Errorf("expected no offers left for the parser")
	}
}
