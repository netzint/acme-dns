package acmedns

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// CredentialCipher provides authenticated encryption (AES-256-GCM) for the
// recoverable copies of the generated API passwords. The key is derived from the
// credentials_key config value, which is expected to live in a 0600 config file
// on the host and never inside the container image.
//
// A nil *CredentialCipher is valid and means credential storage is switched off;
// all methods handle that case, so callers do not need to check first.
type CredentialCipher struct {
	aead cipher.AEAD
}

// ErrNoCipher is returned when credential storage was not configured.
var ErrNoCipher = errors.New("credential encryption is not configured")

// NewCredentialCipher derives a cipher from the configured passphrase. An empty
// secret yields ErrNoCipher, which callers treat as "storage disabled".
func NewCredentialCipher(secret string) (*CredentialCipher, error) {
	if secret == "" {
		return nil, ErrNoCipher
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &CredentialCipher{aead: aead}, nil
}

// Encrypt returns a base64 encoded nonce||ciphertext blob.
func (c *CredentialCipher) Encrypt(plaintext string) (string, error) {
	if c == nil {
		return "", ErrNoCipher
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. An empty input yields an empty string without error
// so that records created before encryption was enabled do not error out.
func (c *CredentialCipher) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if c == nil {
		return "", ErrNoCipher
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(raw) < c.aead.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// EncryptOrEmpty is a nil-safe helper for the database layer: it yields an empty
// string when credential storage is off or encryption fails, which the caller
// stores as "no recoverable password".
func (c *CredentialCipher) EncryptOrEmpty(plaintext string) string {
	if c == nil {
		return ""
	}
	enc, err := c.Encrypt(plaintext)
	if err != nil {
		return ""
	}
	return enc
}

// DecryptOrEmpty is the counterpart to EncryptOrEmpty. It returns an empty
// string for records that predate credential storage.
func (c *CredentialCipher) DecryptOrEmpty(encoded string) string {
	if c == nil || encoded == "" {
		return ""
	}
	plain, err := c.Decrypt(encoded)
	if err != nil {
		return ""
	}
	return plain
}
