package guardian

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	keyByteLength            = 32
	KeyHexLength             = keyByteLength * 2
	EnvelopeVersion   uint16 = 1
	EnvelopeAlgorithm        = "AES-256-GCM"
	envelopeAAD              = "synctv:managed-password:v1:"
)

var (
	ErrKeyNotConfigured = errors.New("guardian credential key is not configured")
	ErrInvalidKey       = fmt.Errorf(
		"guardian credential key must be exactly %d hexadecimal characters",
		KeyHexLength,
	)
	ErrKeyMismatch = errors.New("guardian credential key is incorrect")
)

type Key struct {
	value [32]byte
	id    [32]byte
}

func ParseKey(value string) (Key, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Key{}, ErrKeyNotConfigured
	}
	if len(value) != KeyHexLength {
		return Key{}, ErrInvalidKey
	}

	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != keyByteLength {
		return Key{}, ErrInvalidKey
	}

	var key Key
	copy(key.value[:], decoded)
	key.id = sha256.Sum256(key.value[:])
	return key, nil
}

func (k Key) ID() string {
	return hex.EncodeToString(k.id[:])
}

func (k Key) Matches(other Key) bool {
	return subtle.ConstantTimeCompare(k.id[:], other.id[:]) == 1
}

func Encrypt(key Key, userID, password string) ([]byte, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate managed-password nonce: %w", err)
	}

	return aead.Seal(nonce, nonce, []byte(password), additionalData(userID)), nil
}

func Decrypt(key Key, userID string, envelope []byte) (string, error) {
	aead, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(envelope) < aead.NonceSize()+aead.Overhead() {
		return "", errors.New("managed-password envelope is invalid")
	}

	nonce := envelope[:aead.NonceSize()]
	plaintext, err := aead.Open(nil, nonce, envelope[aead.NonceSize():], additionalData(userID))
	if err != nil {
		return "", errors.New("managed password cannot be decrypted with this key")
	}
	return string(plaintext), nil
}

func newGCM(key Key) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key.value[:])
	if err != nil {
		return nil, fmt.Errorf("initialize managed-password cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize managed-password GCM: %w", err)
	}
	return aead, nil
}

func additionalData(userID string) []byte {
	return []byte(envelopeAAD + userID)
}
