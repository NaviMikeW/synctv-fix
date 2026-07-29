package guardian

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
)

const (
	testKeyHex      = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	differentKeyHex = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
)

func TestManagedPasswordRoundTrip(t *testing.T) {
	key, err := ParseKey(testKeyHex)
	if err != nil {
		t.Fatalf("ParseKey() error = %v", err)
	}

	first, err := Encrypt(key, "user-one", "ChildPassword1!")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	second, err := Encrypt(key, "user-one", "ChildPassword1!")
	if err != nil {
		t.Fatalf("second Encrypt() error = %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("managed-password encryption reused a nonce")
	}

	got, err := Decrypt(key, "user-one", first)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != "ChildPassword1!" {
		t.Fatalf("Decrypt() = %q, want original password", got)
	}
}

func TestManagedPasswordIsBoundToUserAndKey(t *testing.T) {
	key, _ := ParseKey(testKeyHex)
	otherKey, _ := ParseKey(differentKeyHex)
	envelope, err := Encrypt(key, "user-one", "ChildPassword1!")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	if _, err = Decrypt(key, "user-two", envelope); err == nil {
		t.Fatal("Decrypt() accepted ciphertext moved to another user")
	}
	if _, err = Decrypt(otherKey, "user-one", envelope); err == nil {
		t.Fatal("Decrypt() accepted a different key")
	}
	if key.Matches(otherKey) {
		t.Fatal("different keys unexpectedly matched")
	}
}

func TestGuardianKeyValidation(t *testing.T) {
	if _, err := ParseKey(""); !errors.Is(err, ErrKeyNotConfigured) {
		t.Fatalf("empty key error = %v, want ErrKeyNotConfigured", err)
	}
	for _, value := range []string{"short", testKeyHex[:63] + "z"} {
		if _, err := ParseKey(value); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("ParseKey(%q) error = %v, want ErrInvalidKey", value, err)
		}
	}
}

func TestDecryptsWebCryptoEnvelopeFixture(t *testing.T) {
	key, err := ParseKey(testKeyHex)
	if err != nil {
		t.Fatalf("ParseKey() error = %v", err)
	}
	envelope, err := hex.DecodeString(
		"000102030405060708090a0bc26cf37162b7437be9bca661138e83cbe65b97db51190bd35f57d3776b9d43",
	)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	password, err := Decrypt(
		key,
		"0123456789abcdef0123456789abcdef",
		envelope,
	)
	if err != nil {
		t.Fatalf("Decrypt(Web Crypto fixture) error = %v", err)
	}
	if password != "ChildPassword1!" {
		t.Fatalf("fixture password = %q, want original password", password)
	}
}
