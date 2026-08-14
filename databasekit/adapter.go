package databasekit

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This file holds the pgx adapters: the only place in databasekit that names
// pgx result types. Everything a repository uses crosses the pgx-free seam
// (Querier/TxProvider/Tx); the translation back to pgx happens here.

// pgxRowAdapter adapts a pgx.Row to the framework Row, rewriting pgx.ErrNoRows
// to databasekit.ErrNoRows so repositories never import pgx.
type pgxRowAdapter struct {
	row pgx.Row
}

func (r pgxRowAdapter) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoRows
	}
	return err
}

// pgxRowsAdapter adapts pgx.Rows to the framework Rows. It is a shallow
// pass-through; its value is keeping pgx.Rows out of repository imports.
type pgxRowsAdapter struct {
	rows pgx.Rows
}

func (r pgxRowsAdapter) Next() bool             { return r.rows.Next() }
func (r pgxRowsAdapter) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r pgxRowsAdapter) Err() error             { return r.rows.Err() }
func (r pgxRowsAdapter) Close()                 { r.rows.Close() }

// pgxTxAdapter adapts a pgx.Tx to the framework Tx. Nested Begin re-wraps the
// inner pgx.Tx so savepoints stay invisible to callers.
type pgxTxAdapter struct {
	tx pgx.Tx
}

func (t pgxTxAdapter) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := t.tx.Exec(ctx, sql, args...)
	return tag.RowsAffected(), err
}

func (t pgxTxAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := t.tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgxRowsAdapter{rows: rows}, nil
}

func (t pgxTxAdapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return pgxRowAdapter{row: t.tx.QueryRow(ctx, sql, args...)}
}

func (t pgxTxAdapter) Begin(ctx context.Context) (Tx, error) {
	tx, err := t.tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return pgxTxAdapter{tx: tx}, nil
}

func (t pgxTxAdapter) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

func (t pgxTxAdapter) Rollback(ctx context.Context) error {
	err := t.tx.Rollback(ctx)
	if errors.Is(err, pgx.ErrTxClosed) {
		return ErrTxClosed
	}
	return err
}

// poolAdapter adapts a *pgxpool.Pool to TxProvider. It is the wiring-side entry
// point for callers who own a pool directly (tests, non-Lifecycle deployments).
// Repositories never see it.
type poolAdapter struct {
	pool *pgxpool.Pool
}

func (a poolAdapter) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := a.pool.Exec(ctx, sql, args...)
	return tag.RowsAffected(), err
}

func (a poolAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := a.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgxRowsAdapter{rows: rows}, nil
}

func (a poolAdapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return pgxRowAdapter{row: a.pool.QueryRow(ctx, sql, args...)}
}

func (a poolAdapter) Begin(ctx context.Context) (Tx, error) {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return pgxTxAdapter{tx: tx}, nil
}

// NewPoolProvider wraps a *pgxpool.Pool as a TxProvider. Use it at wiring time
// when you own a pool directly rather than going through PostgresDB.
func NewPoolProvider(pool *pgxpool.Pool) TxProvider {
	return poolAdapter{pool: pool}
}

var _ TxProvider = poolAdapter{}
var _ Tx = pgxTxAdapter{}
