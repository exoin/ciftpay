package publicapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/exoin/ciftpay/internal/admin"
	"github.com/exoin/ciftpay/internal/org"
	"github.com/exoin/ciftpay/internal/platform/httpx"
)

// DynamicIPAllowlist aliases the platform dynamic allowlist.
type DynamicIPAllowlist = httpx.DynamicAllowlist

// AdminHandler handles administrative system and ops-core routes.
type AdminHandler struct {
	Allowlist *httpx.DynamicAllowlist
	Admin     *admin.Handler
}

// Mount registers protected system administration endpoints requiring RoleAdmin.
func (h *AdminHandler) Mount(r chi.Router) {
	r.With(org.RequireRole(org.RoleAdmin)).Group(func(r chi.Router) {
		r.Get("/admin/system/daraja-ips", h.GetDarajaIPs)
		r.Post("/admin/system/daraja-ips", h.UpdateDarajaIPs)
		if h.Admin != nil {
			h.Admin.Mount(r)
		}
	})
}

// GetDarajaIPs returns the current Safaricom CIDR allowlist.
func (h *AdminHandler) GetDarajaIPs(w http.ResponseWriter, r *http.Request) {
	if h.Allowlist == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Daraja dynamic allowlist is not configured")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"cidrs": h.Allowlist.Get(),
		},
	})
}

// UpdateDarajaIPs validates and atomically updates the CIDR allowlist.
func (h *AdminHandler) UpdateDarajaIPs(w http.ResponseWriter, r *http.Request) {
	if h.Allowlist == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Daraja dynamic allowlist is not configured")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Failed to read request body")
		return
	}

	var cidrs []string
	if err := json.Unmarshal(body, &cidrs); err != nil {
		var obj struct {
			CIDRs []string `json:"cidrs"`
			IPs   []string `json:"ips"`
		}
		if err2 := json.Unmarshal(body, &obj); err2 != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid_json", "Body must be a JSON array of CIDRs or {\"cidrs\": [...]}")
			return
		}
		if len(obj.CIDRs) > 0 {
			cidrs = obj.CIDRs
		} else {
			cidrs = obj.IPs
		}
	}

	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		cidr := c
		if !strings.Contains(cidr, "/") {
			cidr += "/32"
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid_cidr", fmt.Sprintf("Invalid CIDR format %q", c))
			return
		}
	}

	if err := h.Allowlist.Set(cidrs); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_cidr", err.Error())
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"cidrs": h.Allowlist.Get(),
		},
	})
}
