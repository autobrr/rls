package rls

import (
	"strings"
	"testing"

	"github.com/cytec/releaseparser"
)

func BenchmarkMoistari(b *testing.B) {
	tests := rlsTests(b)
	for b.Loop() {
		for _, test := range tests {
			ParseString(test.s)
		}
	}
}

func BenchmarkCytec(b *testing.B) {
	tests := rlsTests(b)
	for b.Loop() {
		for _, test := range tests {
			releaseparser.Parse(test.s)
		}
	}
}

// benchCorpus returns the release names from tests.yaml.
func benchCorpus(tb testing.TB) []string {
	tests := rlsTests(tb)
	v := make([]string, len(tests))
	for i, test := range tests {
		v[i] = test.s
	}
	return v
}

// BenchmarkParse measures a single release parse. Unlike BenchmarkMoistari
// (which parses the whole corpus per op), each iteration handles one release,
// so ns/op, B/op and allocs/op read directly as per-release figures.
func BenchmarkParse(b *testing.B) {
	corpus := benchCorpus(b)
	n, i := len(corpus), 0
	b.ReportAllocs()
	for b.Loop() {
		ParseString(corpus[i%n])
		i++
	}
}

// BenchmarkParseTags measures the lexing phase alone (src -> []Tag), the first
// of the two phases in ParseRelease.
func BenchmarkParseTags(b *testing.B) {
	corpus := benchCorpus(b)
	src := make([][]byte, len(corpus))
	for i, s := range corpus {
		src[i] = []byte(s)
	}
	n, i := len(src), 0
	b.ReportAllocs()
	for b.Loop() {
		DefaultParser.Parse(src[i%n])
		i++
	}
}

// BenchmarkBuild measures the build phase alone ([]Tag -> Release), the second
// of the two phases in ParseRelease.
//
// Build reclassifies tags in place, so each iteration works on a copy of the
// pre-lexed tags to keep runs independent. The copy is a shallow slice copy of
// ~19 Tag values on the default corpus and is included in the measurement.
func BenchmarkBuild(b *testing.B) {
	corpus := benchCorpus(b)
	// the builder held by DefaultParser is the one initialized with tag info;
	// the package level DefaultBuilder has no infos and would not be
	// representative.
	builder := DefaultParser.(*TagParser).builder
	tags := make([][]Tag, len(corpus))
	ends := make([]int, len(corpus))
	var max int
	for i, s := range corpus {
		tags[i], ends[i] = DefaultParser.Parse([]byte(s))
		if len(tags[i]) > max {
			max = len(tags[i])
		}
	}
	scratch := make([]Tag, max)
	n, i := len(tags), 0
	b.ReportAllocs()
	for b.Loop() {
		v := tags[i%n]
		buf := scratch[:len(v)]
		copy(buf, v)
		builder.Build(buf, ends[i%n])
		i++
	}
}

// longName exercises the per-position cost of the lexers: the work grows with
// the number of tokens, so a regression in how often a lexer's regexp runs
// shows up here long before it does on the corpus average.
var longName = strings.Repeat("Some.Very.Long.Anime.Title.Words.Here.", 12) + "S01E05.1080p.WEB-DL.x264-GRP"

func BenchmarkParseLong(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		ParseString(longName)
	}
}
