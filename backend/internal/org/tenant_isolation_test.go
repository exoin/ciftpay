package org

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal/mock"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/dbtest"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
	"github.com/google/uuid"
)

func createSessionForUser(t *testing.T, s *Service, userID uuid.UUID) string {
	t.Helper()
	token := "test-token-" + uuid.New().String()
	csrf := "csrf-" + uuid.New().String()
	expires := time.Now().Add(time.Hour)
	err := s.DB.Unscoped(context.Background(), func(ctx context.Context, tx db.Tx) error {
		_, err := tx.CreateSession(ctx, gen.CreateSessionParams{
			UserID:    userID,
			TokenHash: s.hashToken(token),
			CsrfToken: csrf,
			ExpiresAt: expires,
		})
		return err
	})
	if err != nil {
		t.Fatalf("createSession: %v", err)
	}
	return token
}

func TestTenantIsolation_ForbiddenOnForeignOrg(t *testing.T) {
	d := dbtest.Open(t)
	ctx := context.Background()
	s := newTestService(t, d, NewFiscalPINChecker(mock.New(mock.FailNone), time.Second))

	// User A with Org A
	userA := createTestUser(t, s, "254700000001")
	orgA, err := s.CreateOrg(ctx, userA.ID, CreateOrgInput{Name: "Business A", KRAPin: "P010000001A"})
	if err != nil {
		t.Fatalf("create orgA: %v", err)
	}

	// User B with Org B
	userB := createTestUser(t, s, "254700000002")
	orgB, err := s.CreateOrg(ctx, userB.ID, CreateOrgInput{Name: "Business B", KRAPin: "P010000002B"})
	if err != nil {
		t.Fatalf("create orgB: %v", err)
	}

	// Create sessions
	tokenA := createSessionForUser(t, s, userA.ID)
	tokenB := createSessionForUser(t, s, userB.ID)

	// 1. User A requesting their own org A -> Succeeds
	pA, err := s.Principal(ctx, tokenA, orgA.ID.String())
	if err != nil {
		t.Fatalf("Principal(userA, orgA) unexpected error: %v", err)
	}
	if pA.OrgID != orgA.ID {
		t.Fatalf("expected OrgID %s, got %s", orgA.ID, pA.OrgID)
	}

	// 2. User A requesting foreign org B -> MUST return ErrForbidden
	_, err = s.Principal(ctx, tokenA, orgB.ID.String())
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Principal(userA, orgB) want ErrForbidden, got %v", err)
	}

	// 3. User B requesting foreign org A -> MUST return ErrForbidden
	_, err = s.Principal(ctx, tokenB, orgA.ID.String())
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Principal(userB, orgA) want ErrForbidden, got %v", err)
	}

	// 4. User A requesting random non-existent org -> MUST return ErrForbidden
	randomOrgID := uuid.New().String()
	_, err = s.Principal(ctx, tokenA, randomOrgID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Principal(userA, randomOrg) want ErrForbidden, got %v", err)
	}
}
