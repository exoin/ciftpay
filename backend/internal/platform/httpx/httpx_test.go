package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The PWA calls the api cross-origin (localhost:3000 → localhost:8080) with a
// cookie, a CSRF token and the tenant header. Every one of those must survive
// the preflight or the whole app silently sits in its loading state.
func TestCORS_PreflightAllowsPWAHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := CORS([]string{"http://localhost:3000/"})(next)

	req := httptest.NewRequest(http.MethodOptions, "/payments", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "x-org-id, x-csrf-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("Allow-Origin = %q", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("credentials not allowed")
	}
	allowed := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	for _, want := range []string{"content-type", "x-csrf-token", "x-org-id"} {
		if !strings.Contains(allowed, want) {
			t.Errorf("Allow-Headers %q missing %s", allowed, want)
		}
	}
}

func TestCORS_UnknownOriginGetsNoHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := CORS([]string{"http://localhost:3000"})(next)

	req := httptest.NewRequest(http.MethodGet, "/payments", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("foreign origin must not be allowed")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("request should still reach the handler, got %d", rec.Code)
	}
}
