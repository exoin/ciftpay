package org

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal"
	kragw "github.com/exoin/ciftpay/internal/kra/gateway"
)

// FiscalPINChecker asks the fiscal provider whether KRA knows a PIN. When the
// configured adapter has no PIN lookup the check reports "unavailable" so the
// org is created unverified rather than rejected.
type FiscalPINChecker struct {
	Provider fiscal.Provider
	Timeout  time.Duration
}

// NewFiscalPINChecker wraps a provider; a nil provider is never contacted.
func NewFiscalPINChecker(p fiscal.Provider, timeout time.Duration) *FiscalPINChecker {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &FiscalPINChecker{Provider: p, Timeout: timeout}
}

// CheckPIN implements PINChecker.
func (c *FiscalPINChecker) CheckPIN(ctx context.Context, pin string) error {
	if !PINRe.MatchString(pin) {
		return ErrBadPIN
	}
	lookup, ok := c.Provider.(fiscal.PINLookup)
	if !ok || c.Provider == nil {
		return fmt.Errorf("%w: adapter %s has no PIN lookup", fiscal.ErrLookupUnavailable, adapterName(c.Provider))
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	_, err := lookup.LookupPIN(ctx, pin)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fiscal.ErrPINUnknown):
		return ErrPINUnknown
	default:
		// Validation errors from the adapter cannot happen for a PIN that
		// already passed PINRe; anything else is "could not ask".
		return fmt.Errorf("%w: %w", fiscal.ErrLookupUnavailable, err)
	}
}

func adapterName(p fiscal.Provider) string {
	if p == nil {
		return "none"
	}
	return p.Name()
}

// GatewayPINChecker asks the KRA developer/gateway client whether KRA knows a PIN.
// If the gateway reports ErrPINNotFound, ErrPINUnknown is returned.
// Any upstream or network failure returns ErrLookupUnavailable.
type GatewayPINChecker struct {
	Client *kragw.Client
}

// NewGatewayPINChecker wraps a KRA Gateway client.
func NewGatewayPINChecker(client *kragw.Client) *GatewayPINChecker {
	return &GatewayPINChecker{Client: client}
}

// CheckPIN implements PINChecker.
func (c *GatewayPINChecker) CheckPIN(ctx context.Context, pin string) error {
	if !PINRe.MatchString(pin) {
		return ErrBadPIN
	}
	if c.Client == nil {
		return fmt.Errorf("%w: gateway client is nil", fiscal.ErrLookupUnavailable)
	}
	_, err := c.Client.CheckPIN(ctx, pin)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, kragw.ErrPINNotFound):
		return ErrPINUnknown
	case errors.Is(err, kragw.ErrPINInvalid):
		return ErrBadPIN
	default:
		return fmt.Errorf("%w: %w", fiscal.ErrLookupUnavailable, err)
	}
}
