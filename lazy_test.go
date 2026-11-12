package rls

import (
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
)

// lazyChildEnv names the case a child process of TestDefaultParserLazy runs.
const lazyChildEnv = "RLS_LAZY_CHILD"

// TestDefaultParserLazy runs each case in a child process of its own, where
// nothing has parsed before the case starts.
func TestDefaultParserLazy(t *testing.T) {
	if os.Getenv(lazyChildEnv) != "" {
		t.Skip("running in a child process")
	}
	for _, name := range []string{"import", "assign", "builder", "copy", "concurrent"} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDefaultParserLazyChild$", "-test.v")
			cmd.Env = append(os.Environ(), lazyChildEnv+"="+name)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("child %s: %v\n%s", name, err, out)
			}
		})
	}
}

// TestDefaultParserLazyChild is the child process of TestDefaultParserLazy.
func TestDefaultParserLazyChild(t *testing.T) {
	mode := os.Getenv(lazyChildEnv)
	if mode == "" {
		t.Skip("run by TestDefaultParserLazy")
	}
	if n := defaultBuilds.Load(); n != 0 {
		t.Fatalf("expected the default parser not to be built at start-up, built %d times", n)
	}
	const name = "Show.Title.S01E02.The.Episode.1080p.WEB-DL.DDP5.1.H.264-GRP"
	switch mode {
	case "import":
		if _, ok := DefaultParser.(*TagParser); !ok {
			t.Fatalf("expected DefaultParser to be a *TagParser, got: %T", DefaultParser)
		}
		if r := ParseString(name); r.Group != "GRP" {
			t.Errorf("expected group GRP, got: %q", r.Group)
		}
		if n := defaultBuilds.Load(); n != 1 {
			t.Errorf("expected the default parser to be built once, built %d times", n)
		}
	case "assign":
		p := &countParser{Parser: NewParser()}
		DefaultParser = p
		r := ParseString(name)
		Parse([]byte(name))
		ParseTagsString(name)
		ParseHint([]byte(name), Hint{})
		if r.Group != "GRP" {
			t.Errorf("expected group GRP, got: %q", r.Group)
		}
		if n := p.n.Load(); n != 4 {
			t.Errorf("expected the assigned parser to parse 4 times, got: %d", n)
		}
		if n := defaultBuilds.Load(); n != 0 {
			t.Errorf("expected the built-in default parser never to be built, built %d times", n)
		}
	case "builder":
		// the default parser has the builder the package set at start-up,
		// whatever DefaultBuilder is when it first parses
		DefaultBuilder = markBuilder{}
		if r := ParseString(name); r.Title != "Show Title" {
			t.Errorf("expected the builder of start-up, got title: %q", r.Title)
		}
		if r := NewParser().ParseRelease([]byte(name)); r.Title != "marked" {
			t.Errorf("expected a new parser to have the builder set, got title: %q", r.Title)
		}
		DefaultBuilder = nil
		if r := ParseString(name); r.Group != "GRP" {
			t.Errorf("expected group GRP with no DefaultBuilder, got: %q", r.Group)
		}
	case "copy":
		// a copy made before the first parse parses as the default parser
		// does, with what is built once for both
		cp := *DefaultParser.(*TagParser)
		if r := cp.ParseRelease([]byte(name)); r.Group != "GRP" || r.Title != "Show Title" {
			t.Errorf("expected the copy to parse, got title %q, group %q", r.Title, r.Group)
		}
		if r := ParseString(name); r.Group != "GRP" {
			t.Errorf("expected group GRP, got: %q", r.Group)
		}
		if n := defaultBuilds.Load(); n != 1 {
			t.Errorf("expected the default parser to be built once, built %d times", n)
		}
		// a builder set on a copy is the copy's alone
		cp2 := *DefaultParser.(*TagParser)
		cp2.SetBuilder(markBuilder{})
		if r := cp2.ParseRelease([]byte(name)); r.Title != "marked" {
			t.Errorf("expected the builder set on the copy, got title: %q", r.Title)
		}
		if r := ParseString(name); r.Title != "Show Title" {
			t.Errorf("expected the default parser to keep its builder, got title: %q", r.Title)
		}
		if r := cp.ParseRelease([]byte(name)); r.Title != "Show Title" {
			t.Errorf("expected the first copy to keep its builder, got title: %q", r.Title)
		}
	case "concurrent":
		names := storageNames
		const workers = 32
		got := make([][]Release, workers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for w := range workers {
			wg.Go(func() {
				<-start
				for _, s := range names {
					got[w] = append(got[w], ParseString(s))
				}
			})
		}
		close(start)
		wg.Wait()
		if n := defaultBuilds.Load(); n != 1 {
			t.Errorf("expected the default parser to be built once, built %d times", n)
		}
		p := NewParser()
		for w := range workers {
			for i, s := range names {
				if exp := p.ParseRelease([]byte(s)); !sameRelease(got[w][i], exp) {
					t.Errorf("worker %d, %q: expected %#v, got: %#v", w, s, exp, got[w][i])
				}
			}
		}
	default:
		t.Fatalf("unknown case %q", mode)
	}
}

// countParser counts the parses it is asked for.
type countParser struct {
	Parser
	n atomic.Int32
}

func (p *countParser) Parse(src []byte) ([]Tag, int) {
	p.n.Add(1)
	return p.Parser.Parse(src)
}

func (p *countParser) ParseRelease(src []byte) Release {
	p.n.Add(1)
	return p.Parser.ParseRelease(src)
}

// markBuilder builds every release with the title 'marked'.
type markBuilder struct{}

func (markBuilder) Build([]Tag, int) Release { return Release{Title: "marked"} }
