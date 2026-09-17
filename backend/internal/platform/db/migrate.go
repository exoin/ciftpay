package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	migrations "github.com/exoin/ciftpay/db/migrations"
)

// Migrate applies goose migrations from backend/db/migrations (embedded) and
// then River's own schema, in that order. Safe to run repeatedly.
func (d *DB) Migrate(ctx context.Context) error {
	sqlDB := stdlib.OpenDBFromPool(d.Pool)
	defer func() { _ = sqlDB.Close() }()

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		return fmt.Errorf("db: goose up: %w", err)
	}

	migrator, err := rivermigrate.New(riverpgxv5.New(d.Pool), nil)
	if err != nil {
		return fmt.Errorf("db: river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("db: river migrate: %w", err)
	}
	return nil
}

// Version returns the current goose version for /healthz and ciftctl.
func (d *DB) Version(ctx context.Context) (int64, error) {
	sqlDB := stdlib.OpenDBFromPool(d.Pool)
	defer func() { _ = sqlDB.Close() }()
	if err := goose.SetDialect("postgres"); err != nil {
		return 0, err
	}
	v, err := goose.GetDBVersionContext(ctx, sqlDB)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return v, nil
}
