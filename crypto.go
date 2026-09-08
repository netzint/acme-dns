package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// CredentialCipher is the process wide cipher used to store a recoverable copy
// of the per-domain API passwords. It stays nil when no credentials_key is
// configured, in which case passwords are only ever shown once on registration.
var CredentialCipher *credentialCipher

// credentialCipher provides authenticated encryption (AES-256-GCM) for the
// recoverable password copies. The key is derived from the credentials_key
// config value, which is expected to live in a 0600 config file on the host and
// never inside the container image.
type credentialCipher struct {
	aead cipher.AEAD
}

var errNoCipher = errors.New("credential encryption is not configured")

func newCredentialCipher(secret string) (*credentialCipher, error) {
	if secret == "" {
		return nil, errNoCipher
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
	return &credentialCipher{aead: aead}, nil
}

// Encrypt returns a base64 encoded nonce||ciphertext blob.
func (c *credentialCipher) Encrypt(plaintext string) (string, error) {
	if c == nil {
		return "", errNoCipher
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
func (c *credentialCipher) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if c == nil {
		return "", errNoCipher
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

// encryptCredential is a nil-safe helper for the database layer.
func encryptCredential(plaintext string) string {
	if CredentialCipher == nil {
		return ""
	}
	enc, err := CredentialCipher.Encrypt(plaintext)
	if err != nil {
		return ""
	}
	return enc
}

// decryptCredential is a nil-safe helper for the database layer. It returns an
// empty string for records that predate credential storage.
func decryptCredential(encoded string) string {
	if CredentialCipher == nil || encoded == "" {
		return ""
	}
	plain, err := CredentialCipher.Decrypt(encoded)
	if err != nil {
		return ""
	}
	return plain
}
