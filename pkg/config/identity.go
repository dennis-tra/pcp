package config

import (
	"github.com/libp2p/go-libp2p-core/crypto"
)

type Identity struct {
	// Public/Private key information.
	Key string

	// The path to the location where the identity file is saved.
	Path string `json:"-"`

	// Whether the identity file exists.
	Exists bool `json:"-"`
}

// SetPrivateKey takes a crypto.PrivKey and encodes it, so that
// it can be persisted to disk.
func (i *Identity) SetPrivateKey(key crypto.PrivKey) error {
	data, err := crypto.MarshalPrivateKey(key)
	if err != nil {
		return err
	}

	i.Key = crypto.ConfigEncodeKey(data)

	return nil
}

// PrivateKey returns the private key - who would have thought.
func (i *Identity) PrivateKey() (crypto.PrivKey, error) {
	data, err := crypto.ConfigDecodeKey(i.Key)
	if err != nil {
		return nil, err
	}

	return crypto.UnmarshalPrivateKey(data)
}

// Initialize generates a new key pair and replaces
// the currently set one.
func (i *Identity) GenerateKeyPair() error {

	key, _, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
	if err != nil {
		return err
	}

	err = i.SetPrivateKey(key)
	if err != nil {
		return err
	}

	return nil
}

// IsInitialized checks if a key pair is associated with this
// identity object.
func (i *Identity) IsInitialized() bool {
	return i.Key != ""
}

// Save persists the identity object to disk. The location can
// be retrieved via the Path field.
func (i *Identity) Save() error {
	err := save(identityFile, i, 0700)
	if err == nil {
		i.Exists = true
	}
	return err
}
