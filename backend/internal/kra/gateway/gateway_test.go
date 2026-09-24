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

	// Valid PIN (Company)
	ctx := context.Background()
	tp, err := c.CheckPIN(ctx, "A012345678X")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tp.PIN != "A012345678X" {
		t.Errorf("expected PIN A012345678X, got %s", tp.PIN)
	}
	if !tp.VATRegistered {
		t.Errorf("expected A-PIN to be VAT registered")
	}
	if tp.TaxpayerName == "" {
		t.Errorf("expected non-empty TaxpayerName")
	}

	// Valid PIN (Individual)
	tpIndiv, err := c.CheckPIN(ctx, "P051234567Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tpIndiv.VATRegistered {
		t.Errorf("expected P-PIN not to be VAT registered by default")
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
