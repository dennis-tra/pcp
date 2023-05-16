package io

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/multiformats/go-varint"
	log "github.com/sirupsen/logrus"
)

// WriteBytes writes the given bytes to the destination writer and
// prefixes it with an uvarint indicating the length of the data.
func WriteBytes(w io.Writer, data []byte) (int, error) {
	size := varint.ToUvarint(uint64(len(data)))
	return w.Write(append(size, data...))
}

// byteReader turns an io.byteReader into an io.ByteReader.
type byteReader struct {
	io.Reader
}

func (r *byteReader) ReadByte() (byte, error) {
	var buf [1]byte
	n, err := r.Reader.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, io.ErrNoProgress
	}
	return buf[0], nil
}

// ReadBytes reads an uvarint from the source reader to know how
// much data is following.
func ReadBytes(r io.Reader) ([]byte, error) {
	// bufio.NewReader wouldn't work because it would read the bytes
	// until its buffer (4096 bytes by default) is full. This data
	// isn't available outside of this function.
	br := &byteReader{r}
	l, err := varint.ReadUvarint(br)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, l)
	_, err = br.Read(buf)

	return buf, err
}

// ResetOnShutdown resets the given stream if the host receives a shutdown
// signal to indicate to our peer that we're not interested in the conversation
// anymore. Put the following at the top of a new stream handler:
//
//	defer n.ResetOnShutdown(stream)()
func ResetOnShutdown(ctx context.Context, s network.Stream) context.CancelFunc {
	cancel := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			s.Reset()
		case <-cancel:
		}
	}()
	return func() { close(cancel) }
}

// WaitForEOF waits for an EOF signal on the stream. This indicates that the peer
// has received all data and won't read from this stream anymore. Alternatively
// there is a 10-second timeout.
func WaitForEOF(ctx context.Context, s network.Stream) error {
	log.Debugln("Waiting for stream reset from peer...")
	timeout := time.After(5 * time.Minute)
	done := make(chan error)
	go func() {
		buf := make([]byte, 1)
		n, err := s.Read(buf)
		if err == io.EOF && n == 0 {
			err = nil
		} else if n != 0 {
			err = fmt.Errorf("stream returned data unexpectedly")
		}
		done <- err
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timeout:
		return fmt.Errorf("timeout")
	case err := <-done:
		return err
	}
}
