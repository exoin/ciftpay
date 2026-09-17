package org

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/fiscal/mock"
	"github.com/exoin/ciftpay/internal/platform/crypto"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/dbtest"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
)

// noLookup is a fiscal.Provider without the PIN lookup capability.
type noLookup struct{ fiscal.Provider }

func (noLookup) Name() string { return "nolookup" }

// flakyLookup is a provider whose checker is down.
type flakyLookup struct{ fiscal.Provider }

func (flakyLookup) Name() string { return "flaky" }
func (flakyLookup) LookupPIN(context.Context, string) (fiscal.Taxpayer, error) {
	return fiscal.Taxpayer{}, &fiscal.TransientError{Code: "upstream_5xx", Message: "boom"}
}

func TestFiscalPINChecker(t *testing.T) {
	ctx := context.Background()
	m := mock.New(mock.FailNone)

	cases := []struct {
		name string
		p    fiscal.Provider
		pin  string
		want error
	}{
		{"known pin", m, "A012345678Z", nil},
		{"reserved unknown pin", m, mock.UnknownPIN, ErrPINUnknown},
		{"malformed pin", m, "X123", ErrBadPIN},
		{"adapter without lookup", noLookup{}, "A012345678Z", fiscal.ErrLookupUnavailable},
		{"checker down", flakyLookup{}, "A012345678Z", fiscal.ErrLookupUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewFiscalPINChecker(tc.p, time.Second).CheckPIN(ctx, tc.pin)
			if tc.want == nil && err != nil {
				t.Fatalf("CheckPIN = %v, want nil", err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("CheckPIN = %v, want %v", err, tc.want)
			}
		})
	}
}

func newTestService(t *testing.T, d *db.DB, pin PINChecker) *Service {
	t.Helper()
	keys, err := crypto.New(bytes.Repeat([]byte{7}, 32), "pepper")
	if err != nil {
		t.Fatal(err)
	}
	return New(d, keys, nil, pin, "test-secret", "mock", true, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)))
}

func createTestUser(t *testing.T, s *Service, msisdn string) gen.User {
	t.Helper()
	var u gen.User
	err := s.DB.Unscoped(context.Background(), func(ctx context.Context, tx db.Tx) error {
		enc, err := s.Keys.EncryptString(msisdn)
		if err != nil {
			return err
		}
		u, err = tx.CreateUser(ctx, gen.CreateUserParams{MsisdnEnc: enc, MsisdnHash: s.Keys.Hash(msisdn), Locale: "en"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestCreateOrg_PINLookupOutcomes(t *testing.T) {
	d := dbtest.Open(t)
	ctx := context.Background()

	t.Run("known pin is verified", func(t *testing.T) {
		s := newTestService(t, d, NewFiscalPINChecker(mock.New(mock.FailNone), time.Second))
		u := createTestUser(t, s, "254140994513")
		o, err := s.CreateOrg(ctx, u.ID, CreateOrgInput{Name: "Mama Njeri", KRAPin: "A012345678Z"})
		if err != nil {
			t.Fatal(err)
		}
		if o.KRAPinVerifiedAt == nil {
			t.Fatal("kra_pin_verified_at should be set for a known PIN")
		}
		ms, err := s.ListMemberships(ctx, u.ID)
		if err != nil || len(ms) != 1 || ms[0].Role != RoleOwner || !ms[0].IsDefault {
			t.Fatalf("memberships = %+v, %v; want one default owner", ms, err)
		}
	})

	t.Run("unknown pin is rejected", func(t *testing.T) {
		s := newTestService(t, d, NewFiscalPINChecker(mock.New(mock.FailNone), time.Second))
		u := createTestUser(t, s, "254140994514")
		_, err := s.CreateOrg(ctx, u.ID, CreateOrgInput{Name: "Ghost Ltd", KRAPin: mock.UnknownPIN})
		if !errors.Is(err, ErrPINUnknown) {
			t.Fatalf("CreateOrg = %v, want ErrPINUnknown", err)
		}
		ms, _ := s.ListMemberships(ctx, u.ID)
		if len(ms) != 0 {
			t.Fatalf("no org should have been created, got %+v", ms)
		}
	})

	t.Run("checker down creates unverified org", func(t *testing.T) {
		s := newTestService(t, d, NewFiscalPINChecker(flakyLookup{}, time.Second))
		u := createTestUser(t, s, "254140994515")
		o, err := s.CreateOrg(ctx, u.ID, CreateOrgInput{Name: "Offline Duka", KRAPin: "P012345678Q"})
		if err != nil {
			t.Fatal(err)
		}
		if o.KRAPinVerifiedAt != nil {
			t.Fatal("kra_pin_verified_at must stay null when the checker is unavailable")
		}
	})

	t.Run("duplicate pin conflicts", func(t *testing.T) {
		s := newTestService(t, d, NewFiscalPINChecker(mock.New(mock.FailNone), time.Second))
		u := createTestUser(t, s, "254140994516")
		_, err := s.CreateOrg(ctx, u.ID, CreateOrgInput{Name: "Second Duka", KRAPin: "A012345678Z"})
		if !errors.Is(err, ErrPINTaken) {
			t.Fatalf("CreateOrg = %v, want ErrPINTaken", err)
		}
	})
}
