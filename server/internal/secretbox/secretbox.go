// Package secretbox encrypts secret values with AES-256-GCM under a key that the server reads from
// its settings and never stores in the database, so a copy of the database or a backup does not
// reveal secrets on its own.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// KeySize is the length of an encryption key in bytes.
const KeySize = 32

// ErrDecrypt means a value could not be decrypted: it was changed, moved to another record, or
// encrypted under a different key.
var ErrDecrypt = errors.New("secret value cannot be decrypted")

// Box seals and opens secret values. It is safe for concurrent use.
type Box struct {
	aead  cipher.AEAD
	keyID string
}

// ParseKey decodes a base64 key of exactly KeySize bytes, such as the output of
// openssl rand -base64 32.
func ParseKey(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(key) != KeySize {
		return nil, fmt.Errorf("must be %d random bytes in base64, such as the output of openssl rand -base64 32", KeySize)
	}
	return key, nil
}

// New returns a Box that encrypts with key, which must be KeySize bytes.
func New(key []byte) (*Box, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("secret encryption key must be %d bytes", KeySize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	sum := sha256.Sum256(key)
	return &Box{aead: aead, keyID: hex.EncodeToString(sum[:8])}, nil
}

// KeyID names the key without revealing it, so each stored value records which key sealed it.
func (b *Box) KeyID() string { return b.keyID }

// Seal encrypts plaintext bound to context, which names the record the value belongs to. The same
// context must be passed to Open, so a value copied to another record cannot be decrypted.
func (b *Box) Seal(plaintext, context []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, plaintext, context), nil
}

// Open decrypts a value sealed under keyID for context. It fails with ErrDecrypt when the value
// was sealed with another key or for another context, or was changed.
func (b *Box) Open(ciphertext []byte, keyID string, context []byte) ([]byte, error) {
	if keyID != b.keyID {
		return nil, fmt.Errorf("%w: sealed with key %s, the server has key %s", ErrDecrypt, keyID, b.keyID)
	}
	size := b.aead.NonceSize()
	if len(ciphertext) < size {
		return nil, ErrDecrypt
	}

	plaintext, err := b.aead.Open(nil, ciphertext[:size], ciphertext[size:], context)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}
