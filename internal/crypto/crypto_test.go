package crypto

import (
	"bytes"
	"strings"
	"testing"
)

func TestCipherAndPassphraseRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	c, err := NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}

	enc, err := c.EncryptString("super-secret")
	if err != nil {
		t.Fatal(err)
	}
	dec, err := c.DecryptString(enc)
	if err != nil || dec != "super-secret" {
		t.Fatalf("DecryptString = %q, err=%v", dec, err)
	}

	// Passphrase encryption (both non-empty and empty)
	for _, pass := range []string{"", "my-pass-123"} {
		blob, err := EncryptWithPassphrase([]byte("bundle-data"), pass)
		if err != nil {
			t.Fatalf("EncryptWithPassphrase(%q): %v", pass, err)
		}
		plain, err := DecryptWithPassphrase(blob, pass)
		if err != nil || !bytes.Equal(plain, []byte("bundle-data")) {
			t.Fatalf("DecryptWithPassphrase(%q) = %q, err=%v", pass, string(plain), err)
		}
	}

	if _, err := EncryptWithPassphrase([]byte("x"), "short"); err == nil {
		t.Fatal("expected error for short passphrase")
	}

	plainKey, prefix, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plainKey, "ms_") || !strings.HasPrefix(plainKey, prefix) || HashAPIKey(plainKey) != hash {
		t.Fatalf("unexpected generated key: key=%q prefix=%q hash=%q", plainKey, prefix, hash)
	}
}
