package org

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal"
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
