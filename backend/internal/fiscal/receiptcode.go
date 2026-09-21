package fiscal

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidReceiptCode is returned when a receipt code is malformed or invalid.
var ErrInvalidReceiptCode = errors.New("fiscal: invalid receipt code")

// crockford is Douglas Crockford's base32 alphabet: no I, L, O, U so codes
// survive being read out over the phone or typed from a thermal receipt.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ReceiptCodeLen is the length of public receipt codes used in /r/{code}.
const ReceiptCodeLen = 6

// NewReceiptCode returns a random Crockford base32 code (32^6 ≈ 1.07e9).
// Uniqueness is enforced by invoices.receipt_code UNIQUE; callers retry on
// conflict.
func NewReceiptCode() (string, error) {
	b := make([]byte, ReceiptCodeLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, ReceiptCodeLen)
	for i, v := range b {
		out[i] = crockford[int(v)%len(crockford)]
	}
	return string(out), nil
}

// NormaliseReceiptCode upper-cases and maps the confusable letters back.
func NormaliseReceiptCode(s string) (string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	r := strings.NewReplacer("I", "1", "L", "1", "O", "0", "U", "V", "-", "", " ", "")
	s = r.Replace(s)
	if len(s) != ReceiptCodeLen {
		return "", fmt.Errorf("%w: must be %d characters", ErrInvalidReceiptCode, ReceiptCodeLen)
	}
	for _, c := range s {
		if !strings.ContainsRune(crockford, c) {
			return "", fmt.Errorf("%w: invalid character %q", ErrInvalidReceiptCode, c)
		}
	}
	return s, nil
}
