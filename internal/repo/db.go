// Package repo provides a thin convenience wrapper around *sql.DB.
//
// It intentionally does NOT define per-entity repository interfaces —
// each workflow agent defines its own repository types in separate files
// to avoid merge conflicts on a shared file.
package repo

import (
	"context"
	"database/sql"
)

// DB wraps *sql.DB and re-exposes the common query helpers so that
// repository types can embed it.
type DB struct {
	*sql.DB
}

// New returns a DB wrapper around the given *sql.DB.
func New(db *sql.DB) *DB {
	return &DB{db}
}

// ExecContext is a convenience pass-through (the embedded *sql.DB already
// provides this, but declaring it here makes the wrapper self-documenting).
func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.DB.ExecContext(ctx, query, args...)
}

// QueryRowContext is a convenience pass-through.
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.DB.QueryRowContext(ctx, query, args...)
}
