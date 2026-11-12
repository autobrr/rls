package rls

import (
	"os"
	"strings"
	"testing"

	"github.com/cytec/releaseparser"
)

func BenchmarkMoistari(b *testing.B) {
	buildDefault()
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

// buildDefault builds the default parser, which is built when it first
// parses, so that a benchmark does not count its making.
func buildDefault() {
	DefaultParser.(*TagParser).state()
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
	buildDefault()
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
	buildDefault()
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
	builder := DefaultParser.(*TagParser).state().builder
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
	buildDefault()
	b.ReportAllocs()
	for b.Loop() {
		ParseString(longName)
	}
}

// trailingDelims is a short title followed by a long run of delimiters. The
// meta lexer steps back over that run one byte at a time, so the cost of each
// step has to stay bounded: when it scanned the whole name instead, this took
// seconds rather than microseconds.
var trailingDelims = "x" + strings.Repeat("-", 4000)

func BenchmarkParseTrailingDelims(b *testing.B) {
	buildDefault()
	b.ReportAllocs()
	for b.Loop() {
		ParseString(trailingDelims)
	}
}

// readCorpus returns the names of a file of testdata/corpus, one a line. The
// corpora are not in the repository, so the file may not be there.
func readCorpus(file string) ([]string, error) {
	buf, err := os.ReadFile("testdata/corpus/" + file)
	if err != nil {
		return nil, err
	}
	var v []string
	for l := range strings.SplitSeq(string(buf), "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			v = append(v, l)
		}
	}
	return v, nil
}

// corpusNames is readCorpus for what cannot do without the file, and is
// skipped when it is not there.
func corpusNames(tb testing.TB, file string) []string {
	v, err := readCorpus(file)
	if err != nil {
		tb.Skip(err)
	}
	if len(v) == 0 {
		tb.Skip("testdata/corpus/" + file + " holds no names")
	}
	return v
}

// BenchmarkParseNyaa is BenchmarkParse on the anime corpus, whose names are
// longer, hold more brackets and are often not ASCII.
func BenchmarkParseNyaa(b *testing.B) {
	corpus := corpusNames(b, "nyaa.txt")
	buildDefault()
	n, i := len(corpus), 0
	b.ReportAllocs()
	for b.Loop() {
		ParseString(corpus[i%n])
		i++
	}
}

// BenchmarkParseSports is BenchmarkParse on the sports corpus.
func BenchmarkParseSports(b *testing.B) {
	corpus := corpusNames(b, "sports.txt")
	buildDefault()
	n, i := len(corpus), 0
	b.ReportAllocs()
	for b.Loop() {
		ParseString(corpus[i%n])
		i++
	}
}
