package httpx

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The PWA calls the api cross-origin (localhost:3000 -> localhost:8080) with a
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

func TestCORS_LocalNetworkIPAllowed(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := CORS([]string{"http://localhost:3000"})(next)

	for _, origin := range []string{
		"http://192.168.89.243:3000",
		"http://10.0.0.5:3000",
		"http://172.20.10.4:3000",
		"http://127.0.0.1:3000",
	} {
		req := httptest.NewRequest(http.MethodOptions, "/payments", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("origin %s preflight status = %d, want 204", origin, rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("origin %s got Allow-Origin = %q", origin, got)
		}
		if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Fatalf("origin %s credentials not allowed", origin)
		}
	}
}

func TestCORS_WildcardConfig(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := CORS([]string{"*"})(next)

	req := httptest.NewRequest(http.MethodOptions, "/payments", nil)
	req.Header.Set("Origin", "https://anything.local:8080")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://anything.local:8080" {
		t.Fatalf("wildcard got Allow-Origin = %q", got)
	}
}

func TestDynamicAllowlist_Validation(t *testing.T) {
	al := NewDynamicAllowlist(nil)
	if !al.IsEmpty() {
		t.Fatal("expected empty allowlist")
	}

	err := al.Set([]string{"192.168.1.0/24", "10.0.0.1"})
	if err != nil {
		t.Fatalf("valid CIDRs should pass: %v", err)
	}
	if len(al.Get()) != 2 {
		t.Fatalf("expected 2 CIDRs, got %d", len(al.Get()))
	}

	err = al.Set([]string{"not-a-cidr"})
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
	// Verify state wasn't corrupted
	if len(al.Get()) != 2 {
		t.Fatalf("allowlist should have remained unchanged after failure, got %d", len(al.Get()))
	}
}

func TestDynamicAllowlist_DynamicUpdateMiddleware(t *testing.T) {
	al := NewDynamicAllowlist([]string{"196.201.214.0/24"})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := IPAllowlist(al)(next)

	// IP in initial allowlist -> OK
	req1 := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	req1.RemoteAddr = "196.201.214.50:12345"
	rec1 := httptest.NewRecorder()
	mw.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec1.Code)
	}

	// IP not in initial allowlist -> 403 Forbidden
	req2 := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	req2.RemoteAddr = "196.201.215.10:12345"
	rec2 := httptest.NewRecorder()
	mw.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec2.Code)
	}

	// Dynamically update allowlist without restarting
	err := al.Set([]string{"196.201.215.0/24"})
	if err != nil {
		t.Fatalf("failed to update allowlist: %v", err)
	}

	// Previously allowed IP should now be blocked
	rec3 := httptest.NewRecorder()
	mw.ServeHTTP(rec3, req1)
	if rec3.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for old IP, got %d", rec3.Code)
	}

	// Newly added IP should now be allowed
	rec4 := httptest.NewRecorder()
	mw.ServeHTTP(rec4, req2)
	if rec4.Code != http.StatusOK {
		t.Fatalf("expected status 200 for new IP, got %d", rec4.Code)
	}
}

func TestDynamicAllowlist_EmptyAllowsAll(t *testing.T) {
	al := NewDynamicAllowlist(nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := IPAllowlist(al)(next)

	req := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty allowlist should permit all, got %d", rec.Code)
	}
}

func TestDynamicAllowlist_ThreadSafety(t *testing.T) {
	al := NewDynamicAllowlist([]string{"10.0.0.0/8"})
	var wg sync.WaitGroup

	// Concurrently read and check IPs
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = al.Get()
				_ = al.Allows(nil)
			}
		}(i)
	}

	// Concurrently update allowlist
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = al.Set([]string{fmt.Sprintf("10.%d.0.0/16", id)})
			}
		}(i)
	}

	wg.Wait()
}
