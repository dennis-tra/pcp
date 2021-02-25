// Taken and adapted from: https://github.com/schollz/croc/blob/8dc5bd6e046194d0c5b1dc34b0fd1602f8f6c7ad/src/crypt/crypt_test.go#L1
package crypt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkEncrypt(b *testing.B) {
	salt := []byte{}
	bob, _ := DeriveKey([]byte("password"), salt)
	for i := 0; i < b.N; i++ {
		Encrypt(bob, []byte("hello, world"))
	}
}

func BenchmarkDecrypt(b *testing.B) {
	salt := []byte{}
	key, _ := DeriveKey([]byte("password"), salt)
	msg := []byte("hello, world")
	enc, _ := Encrypt(key, msg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decrypt(key, enc)
	}
}

func TestEncryption(t *testing.T) {
	salt := []byte("some bytes")
	key, err := DeriveKey([]byte("password"), salt)
	assert.Nil(t, err)
	msg := []byte("hello, world!")
	enc, err := Encrypt(key, msg)
	assert.Nil(t, err)
	dec, err := Decrypt(key, enc)
	assert.Nil(t, err)
	assert.Equal(t, msg, dec)

	// check reusing the salt
	key2, err := DeriveKey([]byte("password"), salt)
	dec, err = Decrypt(key2, enc)
	assert.Nil(t, err)
	assert.Equal(t, msg, dec)

	// check reusing the salt
	key2, err = DeriveKey([]byte("wrong password"), salt)
	dec, err = Decrypt(key2, enc)
	assert.NotNil(t, err)
	assert.NotEqual(t, msg, dec)

	// error with no password
	dec, err = Decrypt([]byte(""), key)
	assert.NotNil(t, err)
}
