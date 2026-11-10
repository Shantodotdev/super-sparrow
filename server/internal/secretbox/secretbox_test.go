package secretbox

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

func newKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestSealOpensOnlyForItsContextAndKey(t *testing.T) {
	box, err := New(newKey(t))
	if err != nil {
		t.Fatal(err)
	}

	sealed, err := box.Seal([]byte("sk-live"), []byte("secret-1\x001"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("sk-live")) {
		t.Fatal("sealed value contains the plaintext")
	}

	opened, err := box.Open(sealed, box.KeyID(), []byte("secret-1\x001"))
	if err != nil || string(opened) != "sk-live" {
		t.Fatalf("Open = %q, %v", opened, err)
	}

	if _, err := box.Open(sealed, box.KeyID(), []byte("secret-2\x001")); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("other context: err = %v, want ErrDecrypt", err)
	}

	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	if _, err := box.Open(tampered, box.KeyID(), []byte("secret-1\x001")); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("tampered: err = %v, want ErrDecrypt", err)
	}

	other, err := New(newKey(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Open(sealed, box.KeyID(), []byte("secret-1\x001")); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("other key: err = %v, want ErrDecrypt", err)
	}
}

func TestSealUsesFreshNonces(t *testing.T) {
	box, err := New(newKey(t))
	if err != nil {
		t.Fatal(err)
	}

	first, _ := box.Seal([]byte("same"), nil)
	second, _ := box.Seal([]byte("same"), nil)
	if bytes.Equal(first, second) {
		t.Fatal("two seals of the same value are equal")
	}
}

func TestParseKey(t *testing.T) {
	key := newKey(t)
	parsed, err := ParseKey(" " + base64.StdEncoding.EncodeToString(key) + "\n")
	if err != nil || !bytes.Equal(parsed, key) {
		t.Fatalf("ParseKey = %x, %v", parsed, err)
	}

	for _, bad := range []string{"", "not base64!", base64.StdEncoding.EncodeToString(key[:16])} {
		if _, err := ParseKey(bad); err == nil {
			t.Errorf("ParseKey(%q) succeeded", bad)
		}
	}
}
