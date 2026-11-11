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

	"golang.org/x/sync/errgroup"
)

// Scanner is the scanner interface. Compatible with bufio.Scanner.
type Scanner interface {
	Scan() bool
	Text() string
	Err() error
}

// ReleaseScanner is a release scanner that parses lines with a pool of
// workers.
//
// Results arrive on the channel in the order the workers finish, which is the
// input order only with one worker (WithWorkers(1)). Each Scan carries an ID,
// the 1-based position of its line in the input, so a consumer can restore the
// input order by it. A line that panicked the parser is skipped, which leaves a
// gap in the IDs, and Err reports the first such panic. NewReleaseScanner
// starts runtime.NumCPU() workers unless WithWorkers says otherwise.
//
// A scanner runs one Scan at a time: a second Scan may start once the channel
// of the first is closed. Each Scan starts its IDs at 1 and Err reports the
// current scan only.
type ReleaseScanner struct {
	parser  Parser
	workers int

	// mu guards err and errs, which the goroutines of a scan write and Err
	// reads.
	mu   sync.Mutex
	err  error
	errs []error
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

// Scan scans until the context is closed, or the scanner is exhausted, and
// then closes the returned channel.
//
// Results arrive in the order the workers finish, which is the input order
// only with one worker (WithWorkers(1)). Scan.ID is the 1-based position of
// the line in the input of this call, so a consumer can restore the order by
// it. A line that panicked the parser is skipped, which leaves a gap in the
// IDs, and Err reports the first such panic.
func (s *ReleaseScanner) Scan(ctx context.Context, scanner Scanner) <-chan *Scan {
	s.mu.Lock()
	s.err, s.errs = nil, nil
	s.mu.Unlock()

	eg, ctx := errgroup.WithContext(ctx)
	in, out := make(chan *Scan), make(chan *Scan)
	eg.Go(func() error {
		defer close(in)
		// the IDs belong to this call, so every Scan numbers its lines from 1
		var next int64
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				next++
				v := Scan{
					Line: strings.TrimSuffix(scanner.Text(), "\n"),
					ID:   next,
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
	for i := range max(s.workers, 1) {
		eg.Go(func(worker int) func() error {
			return func() error {
				return s.run(ctx, worker, in, out)
			}
		}(i))
	}
	go func() {
		defer close(out)
		err := eg.Wait()
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
	}()
	return out
}

// ScanReader scans a reader until the context is closed, or the reader is
// exhausted. See Scan for the order of the results.
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
			if scan == nil {
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

// Err returns the first recovered parser panic of the current scan if there
// is one, otherwise the error of the scanner or the context, and nil when the
// context was cancelled. It is safe to call at any time, but it is complete
// only once the channel of the scan is closed.
func (s *ReleaseScanner) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.errs) != 0 {
		return s.errs[0]
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
	// ID is the 1-based position of the line in the input of the Scan call.
	// Results arrive in the order the workers finish, so a consumer restores
	// the input order by ID. A line that panicked the parser is skipped, which
	// leaves a gap in the IDs.
	ID int64
}

// ScanRecoverError is a scan recover error.
type ScanRecoverError struct {
	Worker int
	ID     int64
	S      string
	Stack  []byte
	Err    any
}

// Error satisfies the error interface.
func (err *ScanRecoverError) Error() string {
	return fmt.Sprintf("%d: %d %q: %v", err.Worker, err.ID, err.S, err.Err)
}

// ReleaseScannerOption is a release scanner option.
type ReleaseScannerOption func(*ReleaseScanner)

// WithWorkers is a release scanner option to set the number of workers. A
// number below 1 gives one worker.
func WithWorkers(workers int) ReleaseScannerOption {
	return func(scn *ReleaseScanner) {
		scn.workers = workers
	}
}
