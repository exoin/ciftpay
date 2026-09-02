// Package db owns the pgx pool and the tenant-scoping helpers that make RLS
// work (ADR-0007). Every tenant read or write goes through WithOrg; the two
// narrow exceptions are WithIngest (resolve a shortcode to its org) and
// WithReceipt (public /r/{code} page).
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
)

// ErrNotFound is returned by helpers that translate pgx.ErrNoRows.
var ErrNotFound = errors.New("db: not found")

// DB wraps the pool.
type DB struct {
	Pool *pgxpool.Pool
}

// Open connects and pings.
func Open(ctx context.Context, url string, maxConns int32) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("db: parse url: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// Close releases the pool.
func (d *DB) Close() { d.Pool.Close() }

// Ping reports connectivity for /healthz.
func (d *DB) Ping(ctx context.Context) error { return d.Pool.Ping(ctx) }

// Tx is what a scoped callback receives: the sqlc querier plus the raw tx for
// River's InsertTx and hand-written SQL.
type Tx struct {
	*gen.Queries
	Tx pgx.Tx
}

// WithOrg runs fn inside a transaction scoped to orgID.
func (d *DB) WithOrg(ctx context.Context, orgID uuid.UUID, fn func(ctx context.Context, tx Tx) error) error {
	return d.scoped(ctx, "app.org_id", orgID.String(), fn)
}

// WithIngest runs fn with the cross-tenant ingest scope. Only the webhook
// path uses it, only to SELECT mpesa_shortcodes and write webhook_events.
func (d *DB) WithIngest(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
	return d.scoped(ctx, "app.scope", "ingest", fn)
}

// WithReceipt runs fn with read access to exactly one invoice by receipt code.
func (d *DB) WithReceipt(ctx context.Context, code string, fn func(ctx context.Context, tx Tx) error) error {
	return d.scoped(ctx, "app.receipt_code", code, fn)
}

// Unscoped runs fn on global tables only (orgs, users, sessions, ...).
// Tenant tables return zero rows here by construction.
func (d *DB) Unscoped(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
	return d.scoped(ctx, "", "", fn)
}

func (d *DB) scoped(ctx context.Context, key, value string, fn func(context.Context, Tx) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if key != "" {
		// set_config with is_local=true is the parameterised form of SET LOCAL.
		if _, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", key, value); err != nil {
			return fmt.Errorf("db: set %s: %w", key, err)
		}
	}
	if err := fn(ctx, Tx{Queries: gen.New(tx), Tx: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// NotFound reports whether err is pgx.ErrNoRows or ErrNotFound.
func NotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrNotFound)
}

// Ptr is a small helper for nullable sqlc params.
func Ptr[T any](v T) *T { return &v }
