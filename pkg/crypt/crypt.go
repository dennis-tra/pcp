// Taken and adapted from: https://github.com/schollz/croc/blob/8dc5bd6e046194d0c5b1dc34b0fd1602f8f6c7ad/src/crypt/crypt.go#L1
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
)

const NonceLength = 12

// Encrypt will encrypt the data with the given key.
func Encrypt(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	nonce := make([]byte, NonceLength)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Prefix sealed data with nonce for decryption - see below.
	return append(nonce, gcm.Seal(nil, nonce, data, nil)...), nil
}

// Decrypt uses key to decrypt the given data.
func Decrypt(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Use prefixed nonce from above.
	return gcm.Open(nil, data[:NonceLength], data[NonceLength:], nil)
}
