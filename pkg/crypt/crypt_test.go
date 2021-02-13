// Taken and adapted from: https://github.com/schollz/croc/blob/8dc5bd6e046194d0c5b1dc34b0fd1602f8f6c7ad/src/crypt/crypt_test.go#L1
package crypt

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/pbkdf2"
)

// new
func new(passphrase []byte, usersalt []byte) (key []byte, salt []byte, err error) {
	if len(passphrase) < 1 {
		err = fmt.Errorf("need more than that for passphrase")
		return
	}
	if usersalt == nil {
		salt = make([]byte, 8)
		// http://www.ietf.org/rfc/rfc2898.txt
		// Salt.
		if _, err := rand.Read(salt); err != nil {
			log.Fatalf("can't get random salt: %v", err)
		}
	} else {
		salt = usersalt
	}
	key = pbkdf2.Key(passphrase, salt, 100, 32, sha256.New)
	return
}

func BenchmarkEncrypt(b *testing.B) {
	bob, _, _ := new([]byte("password"), nil)
	for i := 0; i < b.N; i++ {
		Encrypt([]byte("hello, world"), bob)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	key, _, _ := new([]byte("password"), nil)
	msg := []byte("hello, world")
	enc, _ := Encrypt(key, msg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decrypt(key, enc)
	}
}

func TestEncryption(t *testing.T) {
	key, salt, err := new([]byte("password"), nil)
	assert.Nil(t, err)
	msg := []byte("hello, world!")
	enc, err := Encrypt(key, msg)
	assert.Nil(t, err)
	dec, err := Decrypt(key, enc)
	assert.Nil(t, err)
	assert.Equal(t, msg, dec)

	// check reusing the salt
	key2, _, err := new([]byte("password"), salt)
	dec, err = Decrypt(key2, enc)
	assert.Nil(t, err)
	assert.Equal(t, msg, dec)

	// check reusing the salt
	key2, _, err = new([]byte("wrong password"), salt)
	dec, err = Decrypt(key2, enc)
	assert.NotNil(t, err)
	assert.NotEqual(t, msg, dec)

	// error with no password
	dec, err = Decrypt([]byte(""), key)
	assert.NotNil(t, err)

	// error with small password
	_, _, err = new([]byte(""), nil)
	assert.NotNil(t, err)
}
