package mpesa

import (
	"net/http"

	"github.com/exoin/ciftpay/internal/platform/httpx"
)

// IPAllowlist is a thread-safe dynamic allowlist for Daraja webhooks.
type IPAllowlist = httpx.DynamicAllowlist

// NewIPAllowlist constructs a thread-safe allowlist initialized with cidrs.
func NewIPAllowlist(cidrs []string) *IPAllowlist {
	return httpx.NewDynamicAllowlist(cidrs)
}

// IPAllowlistMiddleware returns an HTTP middleware reading from a thread-safe allowlist.
func IPAllowlistMiddleware(al *IPAllowlist) func(http.Handler) http.Handler {
	if al == nil {
		return httpx.IPAllowlist(nil)
	}
	return al.Middleware()
}
