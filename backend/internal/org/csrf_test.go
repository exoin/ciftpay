package org

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/fiscal/mock"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/dbtest"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
)

func TestCsrfEndpoint(t *testing.T) {
	d := dbtest.Open(t)
	ctx := context.Background()
	s := newTestService(t, d, NewFiscalPINChecker(mock.New(mock.FailNone), time.Second))
	h := &Handler{S: s, SecureCookie: false}

	r := chi.NewRouter()
	h.MountPublic(r)
	r.Group(func(pr chi.Router) {
		pr.Use(h.Authenticate)
		h.MountPrivate(pr)
	})

	// 1. Unauthenticated request to /csrf should succeed with 200 and empty token
	req := httptest.NewRequest(http.MethodGet, "/csrf", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unauthenticated /csrf, got %d: %s", w.Code, w.Body.String())
	}
	var unauthRes struct {
		CsrfToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &unauthRes); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if unauthRes.CsrfToken != "" {
		t.Fatalf("expected empty csrf token for unauthenticated caller, got %q", unauthRes.CsrfToken)
	}

	// 2. Create user & session
	u := createTestUser(t, s, "254700123456")
	token := "test-token-" + uuid.New().String()
	expectedCsrf := "csrf-" + uuid.New().String()
	expires := time.Now().Add(time.Hour)
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := tx.CreateSession(ctx, gen.CreateSessionParams{
			UserID:    u.ID,
			TokenHash: s.hashToken(token),
			CsrfToken: expectedCsrf,
			ExpiresAt: expires,
		})
		return err
	})
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// 3. Authenticated request to /csrf should return 200 with the active CSRF token
	reqAuth := httptest.NewRequest(http.MethodGet, "/csrf", nil)
	reqAuth.AddCookie(&http.Cookie{Name: SessionCookie, Value: token})
	wAuth := httptest.NewRecorder()
	r.ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated /csrf, got %d: %s", wAuth.Code, wAuth.Body.String())
	}

	var res struct {
		CsrfToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(wAuth.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if res.CsrfToken != expectedCsrf {
		t.Fatalf("expected csrf token %q, got %q", expectedCsrf, res.CsrfToken)
	}

	// 4. Authenticated request to /auth/csrf should also return 200
	reqAuth2 := httptest.NewRequest(http.MethodGet, "/auth/csrf", nil)
	reqAuth2.AddCookie(&http.Cookie{Name: SessionCookie, Value: token})
	wAuth2 := httptest.NewRecorder()
	r.ServeHTTP(wAuth2, reqAuth2)
	if wAuth2.Code != http.StatusOK {
		t.Fatalf("expected 200 for /auth/csrf, got %d: %s", wAuth2.Code, wAuth2.Body.String())
	}
}
