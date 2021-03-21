// Taken and adapted from:
// - https://github.com/schollz/croc/blob/8dc5bd6e046194d0c5b1dc34b0fd1602f8f6c7ad/src/crypt/crypt.go#L1
// - https://github.com/blend/go-sdk/blob/29e67762ae016aba504d9de96bd99cd4b23728f7/crypto/stream.go#L23

package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
)

// StreamEncrypter implements the Reader interface to be used in
// streaming scenarios.
type StreamEncrypter struct {
	dest   io.Writer
	block  cipher.Block
	stream cipher.Stream
	mac    hash.Hash
	iv     []byte
}

// NewStreamEncrypter initializes a stream encrypter.
func NewStreamEncrypter(key []byte, dest io.Writer) (*StreamEncrypter, error) {
	// Create a new AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, block.BlockSize())
	if _, err = rand.Read(iv); err != nil {
		return nil, err
	}

	return &StreamEncrypter{
		dest:   dest,
		block:  block,
		stream: cipher.NewCTR(block, iv),
		mac:    hmac.New(sha256.New, key),
		iv:     iv,
	}, nil
}

// Write writes bytes encrypted to the writer interface.
func (s *StreamEncrypter) Write(p []byte) (int, error) {
	buf := make([]byte, len(p)) // Could we get rid of this allocation?
	s.stream.XORKeyStream(buf, p)
	n, writeErr := s.dest.Write(buf)
	if err := writeHash(s.mac, buf[:n]); err != nil {
		return n, err
	}

	return n, writeErr
}

func (s *StreamEncrypter) InitializationVector() []byte {
	return s.iv
}

// Hash should be called after the whole payload was read. It
// returns the SHA-256 hash of the payload.
func (s *StreamEncrypter) Hash() []byte {
	return s.mac.Sum(nil)
}

// StreamDecrypter is a decrypter for a stream of data with authentication
type StreamDecrypter struct {
	src    io.Reader
	block  cipher.Block
	stream cipher.Stream
	mac    hash.Hash
}

// NewStreamDecrypter creates a new stream decrypter
func NewStreamDecrypter(key []byte, iv []byte, src io.Reader) (*StreamDecrypter, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return &StreamDecrypter{
		src:    src,
		block:  block,
		stream: cipher.NewCTR(block, iv),
		mac:    hmac.New(sha256.New, key),
	}, nil
}

// Read reads bytes from the underlying reader and then decrypts them.
func (s *StreamDecrypter) Read(p []byte) (int, error) {
	n, readErr := s.src.Read(p)
	if n > 0 {
		err := writeHash(s.mac, p[:n])
		if err != nil {
			return n, err
		}
		s.stream.XORKeyStream(p[:n], p[:n])
		return n, readErr
	}
	return 0, io.EOF
}

// Authenticate verifies that the hash of the stream is correct. This should only be called after processing is finished
func (s *StreamDecrypter) Authenticate(hash []byte) error {
	if !hmac.Equal(hash, s.mac.Sum(nil)) {
		return fmt.Errorf("authentication failed")
	}
	return nil
}

// writeHash takes the given bytes and "extends" the current hash value.
// if not all bytes could be written an error is returned.
func writeHash(mac hash.Hash, p []byte) error {
	m, err := mac.Write(p)
	if err != nil {
		return err
	}

	if m != len(p) {
		return fmt.Errorf("could not write all bytes to hmac")
	}

	return nil
}
