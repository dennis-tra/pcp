// Package progress provides io.Reader and io.Writer with progress and remaining time estimation.
//  ctx := context.Background()
//
//  // get a reader and the total expected number of bytes
//  s := `Now that's what I call progress`
//  size := len(s)
//  r := progress.NewReader(strings.NewReader(s))
//
//  // Start a goroutine printing progress
//  go func() {
//      ctx := context.Background()
//      progressChan := progress.NewTicker(ctx, r, size, 1*time.Second)
//      for p := range progressChan {
//      	fmt.Fprintf(os.Stderr, "\r%v remaining...", p.Remaining().Round(time.Second))
//      }
//      fmt.Fprintln(os.Stderr, "\rdownload is completed")
//  }()
//
//  // use the Reader as normal
//  if _, err := io.Copy(dest, r); err != nil {
//  	log.Fatalln(err)
//  }
package progress

import (
	"context"
	"io"
	"time"
)

// Counter counts bytes.
// Both Reader and Writer are Counter types.
type Counter interface {
	// N gets the current count value.
	// For readers and writers, this is the number of bytes
	// read or written.
	// For other contexts, the number may be anything.
	N() int64
	// Err gets the last error from the Reader or Writer.
	// When the process is finished, this will be io.EOF.
	Err() error
}

// Progress represents a moment of progress.
type Progress struct {
	n         float64
	size      float64
	estimated time.Time
	err       error
}

// N gets the total number of bytes read or written
// so far.
func (p Progress) N() int64 {
	return int64(p.n)
}

// Size gets the total number of bytes that are expected to
// be read or written.
func (p Progress) Size() int64 {
	return int64(p.size)
}

// Complete gets whether the operation is complete or not.
// Always returns false if the Size is unknown (-1).
func (p Progress) Complete() bool {
	if p.err == io.EOF {
		return true
	}
	if p.size == -1 {
		return false
	}
	return p.n >= p.size
}

// Percent calculates the percentage complete.
func (p Progress) Percent() float64 {
	if p.n == 0 {
		return 0
	}
	if p.n >= p.size {
		return 100
	}
	return 100.0 / (p.size / p.n)
}

// Remaining gets the amount of time until the operation is
// expected to be finished. Use Estimated to get a fixed completion time.
// Returns -1 if no estimate is available.
func (p Progress) Remaining() time.Duration {
	if p.estimated.IsZero() {
		return -1
	}
	return time.Until(p.estimated)
}

// Estimated gets the time at which the operation is expected
// to finish. Use Remaining to get a Duration.
// Estimated().IsZero() is true if no estimate is available.
func (p Progress) Estimated() time.Time {
	return p.estimated
}

// NewTicker gets a channel on which ticks of Progress are sent
// at duration d intervals until the operation is complete at which point
// the channel is closed.
// The counter is either a Reader or Writer (or any type that can report its progress).
// The size is the total number of expected bytes being read or written.
// If the context cancels the operation, the channel is closed.
func NewTicker(ctx context.Context, counter Counter, size int64, d time.Duration) <-chan Progress {
	var (
		started = time.Now()
		ch      = make(chan Progress)
	)
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d):
			}
			progress := Progress{
				n:    float64(counter.N()),
				size: float64(size),
				err:  counter.Err(),
			}
			ratio := progress.n / progress.size
			past := float64(time.Since(started))
			if progress.n > 0.0 {
				total := time.Duration(past / ratio)
				if total < 168*time.Hour {
					// don't send estimates that are beyond a week
					progress.estimated = started.Add(total)
				}
			}
			ch <- progress
			if progress.Complete() {
				return
			}
		}
	}()
	return ch
}
