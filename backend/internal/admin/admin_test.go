package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/org"
	"github.com/exoin/ciftpay/internal/platform/httpx"
)

func TestDarajaIPsEndpoint_RoleAdminProtection(t *testing.T) {
	al := httpx.NewDynamicAllowlist([]string{"196.201.214.0/24"})
	h := &Handler{
		DarajaAllowlist: al,
	}

	r := chi.NewRouter()
	r.With(org.RequireRole(org.RoleAdmin)).Group(h.Mount)

	// 1. Request with non-admin role -> 403 Forbidden
	req := httptest.NewRequest(http.MethodGet, "/admin/system/daraja-ips", nil)
	req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{Role: org.RoleStaff}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-admin, got %d", rec.Code)
	}

	// 2. Request with RoleAdmin -> 200 OK
	req = httptest.NewRequest(http.MethodGet, "/admin/system/daraja-ips", nil)
	req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{Role: org.RoleAdmin}))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin, got %d", rec.Code)
	}

	var resp struct {
		Data struct {
			CIDRs []string `json:"cidrs"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data.CIDRs) != 1 || resp.Data.CIDRs[0] != "196.201.214.0/24" {
		t.Fatalf("expected [196.201.214.0/24], got %v", resp.Data.CIDRs)
	}
}

func TestDarajaIPsEndpoint_UpdateAndValidation(t *testing.T) {
	al := httpx.NewDynamicAllowlist([]string{"196.201.214.0/24"})
	h := &Handler{
		DarajaAllowlist: al,
	}

	r := chi.NewRouter()
	r.With(org.RequireRole(org.RoleAdmin)).Group(h.Mount)

	// 1. Valid update via array format
	payload := `["196.201.214.0/24", "196.201.213.0/24"]`
	req := httptest.NewRequest(http.MethodPost, "/admin/system/daraja-ips", bytes.NewBufferString(payload))
	req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{Role: org.RoleAdmin}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(al.Get()) != 2 {
		t.Fatalf("expected 2 CIDRs in memory, got %d", len(al.Get()))
	}

	// 2. Valid update via object format {"cidrs": [...]}
	payloadObj := `{"cidrs": ["10.0.0.0/8"]}`
	req = httptest.NewRequest(http.MethodPost, "/admin/system/daraja-ips", bytes.NewBufferString(payloadObj))
	req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{Role: org.RoleAdmin}))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(al.Get()) != 1 || al.Get()[0] != "10.0.0.0/8" {
		t.Fatalf("expected [10.0.0.0/8], got %v", al.Get())
	}

	// 3. Invalid CIDR format -> 400 Bad Request and allowlist unchanged
	badPayload := `["not-a-cidr-network"]`
	req = httptest.NewRequest(http.MethodPost, "/admin/system/daraja-ips", bytes.NewBufferString(badPayload))
	req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{Role: org.RoleAdmin}))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
	}
	// Verify allowlist was NOT changed
	if len(al.Get()) != 1 || al.Get()[0] != "10.0.0.0/8" {
		t.Fatalf("allowlist should have remained untouched, got %v", al.Get())
	}
}

func TestOpsEndpoints_RoleAdminProtection(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	r.With(org.RequireRole(org.RoleAdmin)).Group(h.Mount)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/admin/orgs"},
		{http.MethodPatch, "/admin/orgs/" + uuid.NewString()},
		{http.MethodPost, "/admin/orgs/batch-safaricom-export"},
		{http.MethodPost, "/admin/orgs/" + uuid.NewString() + "/notify"},
		{http.MethodGet, "/admin/invoices/dlq"},
		{http.MethodPost, "/admin/invoices/" + uuid.NewString() + "/force-acked"},
		{http.MethodGet, "/admin/webhooks"},
		{http.MethodPost, "/admin/webhooks/" + uuid.NewString() + "/replay"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{Role: org.RoleStaff}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403 Forbidden for non-admin, got %d", ep.method, ep.path, rec.Code)
		}
	}
}

func TestOpsEndpoints_Validation(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	r.With(org.RequireRole(org.RoleAdmin)).Group(h.Mount)

	// 1. Force ACKED validation - missing params
	req := httptest.NewRequest(http.MethodPost, "/admin/invoices/"+uuid.NewString()+"/force-acked", bytes.NewBufferString(`{}`))
	req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{Role: org.RoleAdmin}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// Since h.S is nil, handler checks h.S == nil first returning 503
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable when service is nil, got %d", rec.Code)
	}
}
