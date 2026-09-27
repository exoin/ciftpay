package gateway

import (
	"context"
	"testing"
)

func TestMockGateway(t *testing.T) {
	cfg := Config{
		UseMockGateway: true,
		IsProduction:   false,
	}
	c := NewClient(cfg, nil)

	// Valid PIN (Company - P)
	ctx := context.Background()
	tp, err := c.CheckPIN(ctx, "P051234567X")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tp.PIN != "P051234567X" {
		t.Errorf("expected PIN P051234567X, got %s", tp.PIN)
	}
	if !tp.VATRegistered {
		t.Errorf("expected P-PIN (Company) to be VAT registered")
	}
	if tp.TaxpayerName == "" {
		t.Errorf("expected non-empty TaxpayerName")
	}

	// Valid PIN (Individual - A)
	tpIndiv, err := c.CheckPIN(ctx, "A012345678Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tpIndiv.VATRegistered {
		t.Errorf("expected A-PIN (Individual) not to be VAT registered by default")
	}

	// Reserved Unknown PIN
	_, err = c.CheckPIN(ctx, UnknownPIN)
	if err != ErrPINNotFound {
		t.Errorf("expected ErrPINNotFound for UnknownPIN, got %v", err)
	}

	// Invalid PIN format
	_, err = c.CheckPIN(ctx, "INVALID")
	if err != ErrPINInvalid {
		t.Errorf("expected ErrPINInvalid, got %v", err)
	}

	// Obligations
	obs, err := c.FetchObligations(ctx, "A012345678X")
	if err != nil {
		t.Fatalf("unexpected error fetching obligations: %v", err)
	}
	if len(obs) == 0 {
		t.Errorf("expected at least 1 obligation")
	}

	// Obligations for Unknown PIN
	_, err = c.FetchObligations(ctx, UnknownPIN)
	if err != ErrPINNotFound {
		t.Errorf("expected ErrPINNotFound for UnknownPIN obligations, got %v", err)
	}
}

func TestMockGatewayRejectedInProduction(t *testing.T) {
	cfg := Config{
		UseMockGateway: true,
		IsProduction:   true, // Prod flag prevents mock usage
	}
	c := NewClient(cfg, nil)

	ctx := context.Background()
	// Should fail with ErrNotConfigured or network failure, NOT return mock data
	_, err := c.CheckPIN(ctx, "A012345678X")
	if err == nil {
		t.Fatalf("expected error in production when mock is bypassed without config")
	}
}
