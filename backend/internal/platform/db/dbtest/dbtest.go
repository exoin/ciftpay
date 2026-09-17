// Package dbtest gives integration tests a throwaway, fully migrated CiftPay
// database owned by the non-superuser application role, so RLS behaves exactly
// as in production (ADR-0007).
//
// It needs TEST_ADMIN_DATABASE_URL: a superuser URL on the same server the
// compose stack runs (make test exports it from PG_PORT). Without it every
// DB-backed test is skipped, which keeps `go test ./...` runnable offline.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/exoin/ciftpay/internal/platform/db"
)

// EnvAdminURL names the superuser connection string used to create and drop
// the per-test databases.
const EnvAdminURL = "TEST_ADMIN_DATABASE_URL"

// AppRole is the non-superuser role the migrations are run as and the app
// connects with (tools/postgres/init.sql).
const AppRole, appPassword = "ciftpay", "ciftpay"

// Open creates a fresh database, migrates it and returns a pool connected as
// the application role. The database is dropped when the test finishes.
func Open(t testing.TB) *db.DB {
	t.Helper()
	admin := os.Getenv(EnvAdminURL)
	if admin == "" {
		t.Skipf("%s not set; skipping database test", EnvAdminURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminConn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skipf("cannot reach test postgres at %s: %v", EnvAdminURL, err)
	}
	name := "ciftpay_test_" + randomSuffix()
	if _, err := adminConn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s OWNER %s`, quoteIdent(name), AppRole)); err != nil {
		_ = adminConn.Close(ctx)
		t.Fatalf("dbtest: create database: %v", err)
	}
	_ = adminConn.Close(ctx)

	appURL, err := appURLFor(admin, name)
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	d, err := db.Open(ctx, appURL, 4)
	if err != nil {
		t.Fatalf("dbtest: open %s: %v", name, err)
	}
	if err := d.Migrate(ctx); err != nil {
		d.Close()
		t.Fatalf("dbtest: migrate: %v", err)
	}
	t.Cleanup(func() {
		d.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, admin)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(ctx) }()
		_, _ = conn.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, quoteIdent(name)))
	})
	return d
}

// appURLFor swaps the credentials and database of the admin URL for the app
// role and the freshly created database.
func appURLFor(admin, dbname string) (string, error) {
	u, err := url.Parse(admin)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", EnvAdminURL, err)
	}
	u.User = url.UserPassword(AppRole, appPassword)
	u.Path = "/" + dbname
	return u.String(), nil
}

// quoteIdent double-quotes a SQL identifier (pgx.Identifier's spelling of the
// method trips the misspell linter).
func quoteIdent(name string) string {
	return pgx.Identifier{name}.Sanitize() //nolint:misspell // pgx API name
}

func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
