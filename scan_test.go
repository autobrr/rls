package rls

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestScanner_library(t *testing.T) {
	if s := os.Getenv("TESTS"); s != "library" {
		return
	}
	f, err := os.OpenFile("library.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scan(t, newBaseScanner(f))
}

func TestScanner_releaselist(t *testing.T) {
	if s := os.Getenv("TESTS"); s != "releaselist" {
		return
	}
	f, err := os.OpenFile("releaselist.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scan(t, bufio.NewScanner(f) /*, WithWorkers(1)*/)
}

func scan(t *testing.T, scanner Scanner, opts ...ReleaseScannerOption) {
	start, prev, i := time.Now(), time.Now(), 0
	progress := func(typ string) {
		n := time.Now()
		if i != 0 {
			d := n.Sub(start)
			avg := d / time.Duration(i)
			t.Logf("%d: %s (%s) RUNTIME: %s DELTA: %s AVG: %v",
				i, typ, n.Format(time.RFC3339), d.Truncate(time.Millisecond), n.Sub(prev).Truncate(time.Millisecond), avg)
		}
		prev = n
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	unused := struct {
		count map[string]uint64
		items map[string][]string
	}{
		count: make(map[string]uint64),
		items: make(map[string][]string),
	}
	m, s := make(map[string]uint64), NewScanner(opts...)
loop:
	for ch := s.Scan(ctx, scanner); ; i++ {
		if i != 0 && i%10000 == 0 {
			progress("PROGRESS")
		}
		select {
		case <-ctx.Done():
			if err := ctx.Err(); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case v := <-ch:
			if v == nil || v.ID == 0 {
				break loop
			}
			if id, ok := m[v.Line]; ok {
				t.Errorf("%d: %d: DUPLICATE %q (%d)", i, v.ID, string(v.Line), id)
				continue
			}
			if u := v.Release.Unused(); len(u) != 0 {
				for _, tag := range u {
					if s := tag.Text(); !num.MatchString(s) {
						unused.count[s]++
						unused.items[s] = append(unused.items[s], v.Line)
					}
				}
				t.Logf("%d: %d: UNUSED: %q - %q - %s", i, v.ID, string(v.Line), v.Release.Type, joinTags(u, "%s", " "))
			}
		}
	}
	progress("DONE")
	keys := make([]kv, len(unused.count))
	for k := range unused.count {
		keys = append(keys, kv{
			k: k,
			v: unused.count[k],
		})
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].v > keys[j].v
	})
	for i := 0; i < 2000 && i < len(keys); i++ {
		t.Logf("TOP %02d: %q %d", 2000-i, keys[i].k, keys[i].v)
		for j := uint64(0); j < 10 && j < keys[i].v; j++ {
			t.Logf("    %02d: % 2d: %q", 2000-i, j, unused.items[keys[i].k][j])
		}
	}
}

var num = regexp.MustCompile(`^\d+$`)

type kv struct {
	k string
	v uint64
}

type baseScanner struct {
	s *bufio.Scanner
}

func newBaseScanner(r io.Reader) *baseScanner {
	return &baseScanner{
		s: bufio.NewScanner(r),
	}
}

func (s *baseScanner) Scan() bool {
	return s.s.Scan()
}

func (s *baseScanner) Text() string {
	return filepath.Base(strings.TrimSuffix(s.s.Text(), "\n")) + "\n"
}

func (s *baseScanner) Err() error {
	return s.s.Err()
}

// panicParser panics on any line containing "BOOM", and parses normally
// otherwise.
type panicParser struct{ parser Parser }

func (p panicParser) Parse(src []byte) ([]Tag, int) { return p.parser.Parse(src) }

func (p panicParser) ParseRelease(src []byte) Release {
	if bytes.Contains(src, []byte("BOOM")) {
		panic("boom")
	}
	return p.parser.ParseRelease(src)
}

// collect drains ch, returning the scanned lines, or fails when the scanner has
// not finished within the timeout.
func collect(t *testing.T, ch <-chan *Scan, timeout time.Duration) []string {
	t.Helper()
	done := make(chan []string, 1)
	go func() {
		var v []string
		for scan := range ch {
			v = append(v, scan.Line)
		}
		done <- v
	}()
	select {
	case v := <-done:
		return v
	case <-time.After(timeout):
		t.Fatalf("scanner did not close its output channel within %s", timeout)
		return nil
	}
}

func TestScanner(t *testing.T) {
	const n = 50
	lines := strings.Repeat("The.Matrix.1999.1080p.BluRay.x264-GRP\n", n)
	s := NewScanner(WithWorkers(4))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if v := collect(t, s.ScanReader(ctx, strings.NewReader(lines)), 10*time.Second); len(v) != n {
		t.Errorf("expected %d results, got: %d", n, len(v))
	}
	if err := s.Err(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// TestScanner_panic checks that a release which panics the parser does not
// retire the worker that hit it. Before this was fixed, `workers` panicking
// lines retired every worker, the producer then blocked forever, and the out
// channel was never closed.
func TestScanner_panic(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		t.Run(strconv.Itoa(workers), func(t *testing.T) {
			const good = 100
			// one more panicking line than there are workers
			var buf strings.Builder
			for i := 0; i <= workers; i++ {
				buf.WriteString("BOOM.line\n")
			}
			for i := 0; i < good; i++ {
				buf.WriteString("The.Matrix.1999.1080p.BluRay.x264-GRP\n")
			}
			s := NewReleaseScanner(panicParser{DefaultParser}, WithWorkers(workers))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			v := collect(t, s.ScanReader(ctx, strings.NewReader(buf.String())), 10*time.Second)
			if len(v) != good {
				t.Errorf("expected %d good results to survive, got: %d", good, len(v))
			}
			// the panics are reported, not swallowed
			var err *ScanRecoverError
			if e := s.Err(); e == nil {
				t.Error("expected a ScanRecoverError, got nil")
			} else if !errors.As(e, &err) {
				t.Errorf("expected a *ScanRecoverError, got: %T", e)
			}
		})
	}
}

func TestScanner_cancel(t *testing.T) {
	lines := strings.Repeat("The.Matrix.1999.1080p.BluRay.x264-GRP\n", 10000)
	s := NewScanner(WithWorkers(2))
	ctx, cancel := context.WithCancel(context.Background())
	ch := s.ScanReader(ctx, strings.NewReader(lines))
	<-ch
	cancel()
	// draining must terminate rather than hang
	collect(t, ch, 10*time.Second)
}
