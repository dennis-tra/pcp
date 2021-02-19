// Taken and adapted from:
// - https://github.com/schollz/croc/blob/8dc5bd6e046194d0c5b1dc34b0fd1602f8f6c7ad/src/crypt/crypt.go#L1
// - https://github.com/blend/go-sdk/blob/29e67762ae016aba504d9de96bd99cd4b23728f7/crypto/stream.go#L23
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"golang.org/x/crypto/scrypt"
)

const NonceLength = 12

// DeriveKey uses scrypt to generated a fixed size key for further
// encryption/decryption steps aka. to initialise the block cipher.
// After minimal research I chose scrypt over pbkdf2 as apparently
// scrypt contains pbkdf2? Which is a good thing I guess. The
// parameters below are taken from example test in the scrypt
// package.
func DeriveKey(pw []byte, salt []byte) ([]byte, error) {
	return scrypt.Key(pw, salt, 1<<15, 8, 1, 32)
}

// Encrypt will encrypt the data with the given key.
func Encrypt(key []byte, data []byte) ([]byte, error) {
	// Create a new AES cipher
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
	// Create a new AES cipher
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
