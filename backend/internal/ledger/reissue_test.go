package ledger

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestPINValidation(t *testing.T) {
	valid := []string{"A000123456B", "P051234567Z", "A123456789X"}
	for _, pin := range valid {
		if !PINRe.MatchString(pin) {
			t.Errorf("PIN %q should be valid", pin)
		}
	}

	invalid := []string{"", "123456789", "B000123456Z", "A00012345B", "A00012345678B", "a000123456b"}
	for _, pin := range invalid {
		if PINRe.MatchString(pin) {
			t.Errorf("PIN %q should be invalid", pin)
		}
	}
}

func TestReissueValidation(t *testing.T) {
	svc := &Service{}
	_, err := svc.ReissueInvoice(context.Background(), uuid.New(), uuid.New(), ReissueInput{
		BuyerPIN: "INVALID_PIN",
	})
	if err != ErrInvalidPIN {
		t.Errorf("got %v, want %v", err, ErrInvalidPIN)
	}

	_, err = svc.ClaimReceiptByCode(context.Background(), "INVALID_CODE_XYZ", "A000123456B", "Test Buyer")
	if err == nil {
		t.Errorf("expected error for invalid receipt code")
	}
}
