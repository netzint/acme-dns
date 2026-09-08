package acmedns

import "testing"

func TestCredentialCipherRoundTrip(t *testing.T) {
	cipher, err := NewCredentialCipher("a passphrase")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secret := GeneratePassword(40)
	encrypted, err := cipher.Encrypt(secret)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if encrypted == secret || encrypted == "" {
		t.Fatal("ciphertext must differ from plaintext and not be empty")
	}

	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if decrypted != secret {
		t.Fatalf("round trip mismatch: got %q, want %q", decrypted, secret)
	}

	// A different key must not be able to read it.
	other, _ := NewCredentialCipher("another passphrase")
	if _, err := other.Decrypt(encrypted); err == nil {
		t.Fatal("decrypting with the wrong key should fail")
	}
}

func TestCredentialCipherEmptyValues(t *testing.T) {
	if _, err := NewCredentialCipher(""); err == nil {
		t.Fatal("an empty key should be rejected")
	}

	cipher, _ := NewCredentialCipher("key")
	got, err := cipher.Decrypt("")
	if err != nil || got != "" {
		t.Fatalf("empty ciphertext should decode to empty string without error, got %q / %v", got, err)
	}
}

// A nil cipher means credential storage is switched off. The database layer
// relies on the helpers staying usable in that state.
func TestCredentialCipherNilIsUsable(t *testing.T) {
	var cipher *CredentialCipher

	if cipher.EncryptOrEmpty("secret") != "" {
		t.Fatal("EncryptOrEmpty should return empty when no cipher is configured")
	}
	if cipher.DecryptOrEmpty("anything") != "" {
		t.Fatal("DecryptOrEmpty should return empty when no cipher is configured")
	}
	if _, err := cipher.Encrypt("secret"); err != ErrNoCipher {
		t.Fatalf("Encrypt on a nil cipher should report ErrNoCipher, got %v", err)
	}
}
