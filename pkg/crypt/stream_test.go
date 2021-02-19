package crypt

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStreamEncrypterDecrypter(t *testing.T) {
	salt := []byte("salt")
	pw := []byte("password")
	key, err := DeriveKey(pw, salt)
	assert.Nil(t, err)

	payload := []byte("some text")
	src := bytes.NewReader(payload)
	se, err := NewStreamEncrypter(key, salt, src)
	assert.Nil(t, err)
	assert.NotNil(t, se)

	encrypted, err := ioutil.ReadAll(se)
	assert.Nil(t, err)
	assert.NotNil(t, encrypted)

	sd, err := NewStreamDecrypter(key, salt, se.InitializationVector(), bytes.NewReader(encrypted))
	assert.Nil(t, err)
	assert.NotNil(t, sd)

	decrypted, err := ioutil.ReadAll(sd)
	assert.Nil(t, err)
	assert.Equal(t, payload, decrypted)

	assert.Nil(t, sd.Authenticate(se.Hash()))
}
