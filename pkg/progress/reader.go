package progress

import (
	"io"
	"sync"
)

// Reader counts the bytes read through it.
type Reader struct {
	r io.Reader

	lock sync.RWMutex // protects n and err
	n    int64
	err  error
}

// NewReader makes a new Reader that counts the bytes
// read through it.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		r: r,
	}
}

func (r *Reader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	r.lock.Lock()
	r.n += int64(n)
	r.err = err
	r.lock.Unlock()
	return
}

// N gets the number of bytes that have been read
// so far.
func (r *Reader) N() int64 {
	var n int64
	r.lock.RLock()
	n = r.n
	r.lock.RUnlock()
	return n
}

// Err gets the last error from the Reader.
func (r *Reader) Err() error {
	var err error
	r.lock.RLock()
	err = r.err
	r.lock.RUnlock()
	return err
}
