// Package crypto implements envelope encryption for personal data (MSISDN, KRA
// PIN) and keyed hashing for equality lookups. See docs/data-model.md §4.
//
// Stored blob layout (version 1):
//
//	version(1) ‖ dekNonce(12) ‖ wrappedDEK(48) ‖ dataNonce(12) ‖ ciphertext
//
// The data encryption key (DEK) is random per record and wrapped with the
// master key, so rotating the master key means re-wrapping DEKs without
// touching the ciphertext.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
)

const (
	version1     = 0x01
	nonceSize    = 12
	dekSize      = 32
	wrappedSize  = dekSize + 16 // AES-GCM tag
	headerSize   = 1 + nonceSize + wrappedSize + nonceSize
	masterKeyLen = 32
)

// ErrMalformed is returned when a blob cannot be parsed.
var ErrMalformed = errors.New("crypto: malformed ciphertext")

// Keyring holds the master key and the hash pepper.
type Keyring struct {
	master cipher.AEAD
	pepper []byte
}

// New builds a Keyring from a 32-byte master key and a non-empty pepper.
func New(masterKey []byte, pepper string) (*Keyring, error) {
	if len(masterKey) != masterKeyLen {
		return nil, fmt.Errorf("crypto: master key must be %d bytes, got %d", masterKeyLen, len(masterKey))
	}
	if pepper == "" {
		return nil, errors.New("crypto: hash pepper must not be empty")
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Keyring{master: aead, pepper: []byte(pepper)}, nil
}

// Encrypt envelope-encrypts plaintext. An empty plaintext yields nil.
func (k *Keyring) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, nil
	}
	dek := make([]byte, dekSize)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	dataAEAD, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, headerSize+len(plaintext)+16)
	out = append(out, version1)

	dekNonce := make([]byte, nonceSize)
	if _, err := rand.Read(dekNonce); err != nil {
		return nil, err
	}
	out = append(out, dekNonce...)
	out = k.master.Seal(out, dekNonce, dek, []byte{version1})

	dataNonce := make([]byte, nonceSize)
	if _, err := rand.Read(dataNonce); err != nil {
		return nil, err
	}
	out = append(out, dataNonce...)
	out = dataAEAD.Seal(out, dataNonce, plaintext, nil)
	return out, nil
}

// Decrypt reverses Encrypt. A nil or empty blob yields nil.
func (k *Keyring) Decrypt(blob []byte) ([]byte, error) {
	if len(blob) == 0 {
		return nil, nil
	}
	if len(blob) < headerSize || blob[0] != version1 {
		return nil, ErrMalformed
	}
	off := 1
	dekNonce := blob[off : off+nonceSize]
	off += nonceSize
	wrapped := blob[off : off+wrappedSize]
	off += wrappedSize
	dataNonce := blob[off : off+nonceSize]
	off += nonceSize
	ciphertext := blob[off:]

	dek, err := k.master.Open(nil, dekNonce, wrapped, []byte{version1})
	if err != nil {
		return nil, fmt.Errorf("crypto: unwrap dek: %w", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	dataAEAD, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := dataAEAD.Open(nil, dataNonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: open: %w", err)
	}
	return plain, nil
}

// EncryptString is Encrypt for strings.
func (k *Keyring) EncryptString(s string) ([]byte, error) { return k.Encrypt([]byte(s)) }

// DecryptString is Decrypt for strings.
func (k *Keyring) DecryptString(blob []byte) (string, error) {
	b, err := k.Decrypt(blob)
	return string(b), err
}

// Hash returns HMAC-SHA256(pepper, value) for equality lookups.
// Callers must normalise first (NormaliseMSISDN, NormalisePIN).
func (k *Keyring) Hash(value string) []byte {
	if value == "" {
		return nil
	}
	m := hmac.New(sha256.New, k.pepper)
	m.Write([]byte(value))
	return m.Sum(nil)
}

// HashMSISDN normalises then hashes a phone number.
func (k *Keyring) HashMSISDN(msisdn string) []byte {
	n, err := NormaliseMSISDN(msisdn)
	if err != nil {
		return nil
	}
	return k.Hash(n)
}

// NormaliseMSISDN converts Kenyan numbers to the 2547XXXXXXXX / 2541XXXXXXXX form.
// Accepts "+254...", "254...", "07...", "01...", "7..." and "1..." with spaces or dashes.
func NormaliseMSISDN(raw string) (string, error) {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case len(d) == 12 && strings.HasPrefix(d, "254"):
	case len(d) == 10 && (strings.HasPrefix(d, "07") || strings.HasPrefix(d, "01")):
		d = "254" + d[1:]
	case len(d) == 9 && (d[0] == '7' || d[0] == '1'):
		d = "254" + d
	default:
		return "", fmt.Errorf("crypto: %q is not a Kenyan MSISDN", raw)
	}
	return d, nil
}

// NormalisePIN upper-cases and trims a KRA PIN.
func NormalisePIN(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}
