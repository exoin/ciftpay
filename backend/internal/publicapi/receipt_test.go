package publicapi

import (
	"context"
	"errors"
	"testing"

	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/ledger"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
)

type mockClaimer struct {
	calledWithCode string
	calledWithPIN  string
	errToReturn    error
}

func (m *mockClaimer) ClaimReceiptByCode(ctx context.Context, code, buyerPIN, buyerName string) (gen.Invoice, error) {
	m.calledWithCode = code
	m.calledWithPIN = buyerPIN
	if m.errToReturn != nil {
		return gen.Invoice{}, m.errToReturn
	}
	return gen.Invoice{}, nil
}

func TestClaimValidation(t *testing.T) {
	claimer := &mockClaimer{}
	svc := New(nil, nil, claimer)

	// Test invalid receipt code
	_, err := svc.Claim(context.Background(), "SHORT", "A000123456B", "Test")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}

	// Test invalid PIN from claimer
	claimer.errToReturn = ledger.ErrInvalidPIN
	validCode, _ := fiscal.NewReceiptCode()
	_, err = svc.Claim(context.Background(), validCode, "BAD_PIN", "Test")
	if !errors.Is(err, ledger.ErrInvalidPIN) {
		t.Errorf("got %v, want ErrInvalidPIN", err)
	}
}
