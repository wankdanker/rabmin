// Package secret encrypts sensitive values (NATS credentials, TLS keys) before
// they are written to the rabmin database.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Box seals and opens values with AES-256-GCM.
type Box struct {
	aead cipher.AEAD
}

// New returns a Box for a 32 byte key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Load resolves the key from a base64 string when given, otherwise from
// keyFile, generating that file on first run.
func Load(encoded, keyFile string) (*Box, error) {
	if encoded != "" {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return nil, fmt.Errorf("decode secret key: %w", err)
		}
		return New(key)
	}
	data, err := os.ReadFile(keyFile)
	if errors.Is(err, os.ErrNotExist) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(keyFile), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
			return nil, err
		}
		return New(key)
	}
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", keyFile, err)
	}
	return New(key)
}

// Seal encrypts plaintext and returns a base64 string.
func (b *Box) Seal(plaintext []byte) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Open decrypts a value produced by Seal.
func (b *Box) Open(sealed string) ([]byte, error) {
	if sealed == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return nil, err
	}
	ns := b.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("sealed value too short")
	}
	return b.aead.Open(nil, data[:ns], data[ns:], nil)
}
