package publicapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/admin"
	"github.com/exoin/ciftpay/internal/org"
	"github.com/exoin/ciftpay/internal/platform/httpx"
)

func TestPublicAPIAdminHandler_UpdateAndEnforce(t *testing.T) {
	al := httpx.NewDynamicAllowlist([]string{"196.201.214.0/24"})
	h := &AdminHandler{Allowlist: al, Admin: &admin.Handler{DarajaAllowlist: al}}

	r := chi.NewRouter()
	h.Mount(r)

	// Admin update
	payload := `["196.201.214.0/24", "196.201.213.0/24"]`
	req := httptest.NewRequest(http.MethodPost, "/admin/system/daraja-ips", bytes.NewBufferString(payload))
	req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{Role: org.RoleAdmin}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if len(al.Get()) != 2 {
		t.Fatalf("expected 2 CIDRs, got %d", len(al.Get()))
	}

	// Invalid CIDR
	badReq := httptest.NewRequest(http.MethodPost, "/admin/system/daraja-ips", bytes.NewBufferString(`["invalid-ip"]`))
	badReq = badReq.WithContext(httpx.WithPrincipal(badReq.Context(), httpx.Principal{Role: org.RoleAdmin}))
	badRec := httptest.NewRecorder()
	r.ServeHTTP(badRec, badReq)

	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", badRec.Code)
	}

	// Non-admin access to Ops endpoints mounted via AdminHandler should be 403 Forbidden
	forbiddenReq := httptest.NewRequest(http.MethodGet, "/admin/orgs", nil)
	forbiddenReq = forbiddenReq.WithContext(httpx.WithPrincipal(forbiddenReq.Context(), httpx.Principal{Role: org.RoleStaff}))
	forbiddenRec := httptest.NewRecorder()
	r.ServeHTTP(forbiddenRec, forbiddenReq)

	if forbiddenRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-admin, got %d", forbiddenRec.Code)
	}

	// Admin access to Ops endpoints should route through
	adminReq := httptest.NewRequest(http.MethodGet, "/admin/invoices/dlq", nil)
	adminReq = adminReq.WithContext(httpx.WithPrincipal(adminReq.Context(), httpx.Principal{Role: org.RoleAdmin}))
	adminRec := httptest.NewRecorder()
	r.ServeHTTP(adminRec, adminReq)

	// Since h.Admin.S is nil, returns 503 rather than 403 or 404
	if adminRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for nil service, got %d", adminRec.Code)
	}

	// Force Acked with non-admin should be 403
	forceReq := httptest.NewRequest(http.MethodPost, "/admin/invoices/"+uuid.NewString()+"/force-acked", nil)
	forceReq = forceReq.WithContext(httpx.WithPrincipal(forceReq.Context(), httpx.Principal{Role: org.RoleStaff}))
	forceRec := httptest.NewRecorder()
	r.ServeHTTP(forceRec, forceReq)

	if forceRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-admin on force-acked, got %d", forceRec.Code)
	}
}
