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

	var buf bytes.Buffer
	se, err := NewStreamEncrypter(key, &buf)
	assert.Nil(t, err)
	assert.NotNil(t, se)

	payload := []byte("some text")
	_, _ = se.Write(payload)

	sd, err := NewStreamDecrypter(key, se.InitializationVector(), &buf)
	assert.Nil(t, err)
	assert.NotNil(t, sd)

	decrypted, err := ioutil.ReadAll(sd)
	assert.Nil(t, err)
	assert.Equal(t, payload, decrypted)

	assert.Nil(t, sd.Authenticate(se.Hash()))
}
