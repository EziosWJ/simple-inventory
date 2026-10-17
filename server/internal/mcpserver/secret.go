package mcpserver

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// Recoverable token secrets are stored with a display key derived from the
// existing jwt.secret through a separate HKDF purpose, so deployment keeps one
// configuration value and authentication never depends on this key: the
// SHA-256 digest remains the only credential check. Changing jwt.secret can
// therefore only make stored secrets unreadable, never silently valid.
const (
	displayKeyPurpose = "simple-inventory/mcp-token-display-secret/v1"
	secretVersion1    = 1
)

// ErrSecretUnavailable reports that a stored secret cannot be decrypted with
// the current configuration. The token still authenticates by digest; the
// administrator either restores the original configuration or resets it.
var ErrSecretUnavailable = errors.New("token secret needs the original configuration or a reset")

// SecretBox seals and opens recoverable token secrets.
type SecretBox struct{ aead cipher.AEAD }

// NewSecretBox derives the display key from the configured JWT secret.
func NewSecretBox(jwtSecret string) (*SecretBox, error) {
	if strings.TrimSpace(jwtSecret) == "" {
		return nil, errors.New("jwt secret is required to protect recoverable token secrets")
	}
	key, err := hkdf.Key(sha256.New, []byte(jwtSecret), nil, displayKeyPurpose, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

// Seal returns the stored version and base64 nonce-prefixed ciphertext. The
// digest binds a ciphertext to the exact credential it records, so a swapped
// or stale ciphertext fails to open instead of returning the wrong secret.
func (b *SecretBox) Seal(digest, plaintext string) (int, string, error) {
	if b == nil {
		return 0, "", ErrSecretUnavailable
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return 0, "", err
	}
	sealed := b.aead.Seal(nil, nonce, []byte(plaintext), []byte(digest))
	return secretVersion1, base64.RawStdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

// Open returns the original credential, or ErrSecretUnavailable when the
// ciphertext belongs to another key or version.
func (b *SecretBox) Open(version *int, digest, encoded string) (string, error) {
	if b == nil || version == nil || *version != secretVersion1 || encoded == "" {
		return "", ErrSecretUnavailable
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(raw) <= b.aead.NonceSize() {
		return "", ErrSecretUnavailable
	}
	plaintext, err := b.aead.Open(nil, raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():], []byte(digest))
	if err != nil {
		return "", ErrSecretUnavailable
	}
	return string(plaintext), nil
}
