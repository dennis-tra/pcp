package progress

import (
	"io"
	"sync"
)

// Writer counts the bytes written through it.
type Writer struct {
	w io.Writer

	lock sync.RWMutex // protects n and err
	n    int64
	err  error
}

// NewWriter gets a Writer that counts the number
// of bytes written.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: w,
	}
}

func (w *Writer) Write(p []byte) (n int, err error) {
	n, err = w.w.Write(p)
	w.lock.Lock()
	w.n += int64(n)
	w.err = err
	w.lock.Unlock()
	return
}

// N gets the number of bytes that have been written
// so far.
func (w *Writer) N() int64 {
	var n int64
	w.lock.RLock()
	n = w.n
	w.lock.RUnlock()
	return n
}

// Err gets the last error from the Writer.
func (w *Writer) Err() error {
	var err error
	w.lock.RLock()
	err = w.err
	w.lock.RUnlock()
	return err
}
