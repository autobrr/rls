package rls

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

// Scanner is the scanner interface. Compatible with bufio.Scanner.
type Scanner interface {
	Scan() bool
	Text() string
	Err() error
}

// ReleaseScanner is a release scanner.
type ReleaseScanner struct {
	parser  Parser
	workers int
	next    int64
	err     error
	mu      sync.Mutex
	errs    []error
}

// NewReleaseScanner creates a new release scanner.
func NewReleaseScanner(p Parser, opts ...ReleaseScannerOption) *ReleaseScanner {
	scn := &ReleaseScanner{
		parser:  p,
		workers: runtime.NumCPU(),
	}
	for _, o := range opts {
		o(scn)
	}
	return scn
}

// NewScanner creates a new release scanner using the default parser.
func NewScanner(opts ...ReleaseScannerOption) *ReleaseScanner {
	return NewReleaseScanner(DefaultParser, opts...)
}

// Scan scans until the context is closed, or the scanner is exhausted.
func (s *ReleaseScanner) Scan(ctx context.Context, scanner Scanner) <-chan *Scan {
	eg, ctx := errgroup.WithContext(ctx)
	in, out := make(chan *Scan), make(chan *Scan)
	eg.Go(func() error {
		defer close(in)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				v := Scan{
					Line: strings.TrimSuffix(scanner.Text(), "\n"),
					ID:   atomic.AddInt64(&s.next, 1),
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case in <- &v:
				}
			}
		}
		return scanner.Err()
	})
	for i := 0; i < s.workers; i++ {
		eg.Go(func(worker int) func() error {
			return func() error {
				return s.run(ctx, worker, in, out)
			}
		}(i))
	}
	go func() {
		defer close(out)
		s.err = eg.Wait()
	}()
	return out
}

// ScanReader scans a reader until the context is closed, or the reader is
// exhausted.
func (s *ReleaseScanner) ScanReader(ctx context.Context, r io.Reader) <-chan *Scan {
	return s.Scan(ctx, bufio.NewScanner(r))
}

// run parses from in, places on out.
func (s *ReleaseScanner) run(ctx context.Context, worker int, in, out chan *Scan) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case scan := <-in:
			if scan == nil || scan.ID == 0 {
				return nil
			}
			// a release that panics the parser must not take the worker with
			// it: recovering here and continuing keeps the remaining input
			// flowing. recovering at the func level instead would retire the
			// worker, and once every worker has retired the producer blocks
			// forever and the out channel is never closed.
			if !s.parse(worker, scan) {
				continue
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- scan:
			}
		}
	}
}

// parse parses scan, recovering from a panic in the parser. Reports false when
// the parse panicked, in which case the error has been recorded and the scan
// should be skipped.
func (s *ReleaseScanner) parse(worker int, scan *Scan) (ok bool) {
	defer func() {
		if err := recover(); err != nil {
			s.mu.Lock()
			s.errs = append(s.errs, &ScanRecoverError{worker, scan.ID, scan.Line, debug.Stack(), err})
			s.mu.Unlock()
			ok = false
		}
	}()
	scan.Release = s.parser.ParseRelease([]byte(scan.Line))
	return true
}

// Err returns the last encountered error.
func (s *ReleaseScanner) Err() error {
	s.mu.Lock()
	errs := len(s.errs)
	var first error
	if errs != 0 {
		first = s.errs[0]
	}
	s.mu.Unlock()
	if errs != 0 {
		return first
	}
	if !errors.Is(s.err, context.Canceled) {
		return s.err
	}
	return nil
}

// Scan represents scanned work.
type Scan struct {
	Release Release
	Line    string
	ID      int64
}

// ScanRecoverError is a scan recover error.
type ScanRecoverError struct {
	Worker int
	ID     int64
	S      string
	Stack  []byte
	Err    interface{}
}

// Error satisfies the error interface.
func (err *ScanRecoverError) Error() string {
	return fmt.Sprintf("%d: %d %q: %v", err.Worker, err.ID, err.S, err.Err)
}

// ReleaseScannerOption is a release scanner option.
type ReleaseScannerOption func(*ReleaseScanner)

// WithWorkers is a release scanner option to set the number of workers.
func WithWorkers(workers int) ReleaseScannerOption {
	return func(scn *ReleaseScanner) {
		scn.workers = workers
	}
}
