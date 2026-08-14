package databasekit

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// pgxProvider is the query surface shared by pgx.Tx and *pgxpool.Pool: the pgx
// query methods plus opening a transaction. Both framework adapters translate
// it to the pgx-free seam, so the translation lives in one place.
type pgxProvider interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// pgxProviderAdapter translates a pgxProvider to the framework Querier and
// Begin. pgxTxAdapter and poolAdapter embed it.
type pgxProviderAdapter struct {
	src pgxProvider
}

func (a pgxProviderAdapter) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := a.src.Exec(ctx, sql, args...)
	return tag.RowsAffected(), err
}

func (a pgxProviderAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := a.src.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgxRowsAdapter{rows: rows}, nil
}

func (a pgxProviderAdapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return pgxRowAdapter{row: a.src.QueryRow(ctx, sql, args...)}
}

// Begin opens a transaction and wraps it in pgxTxAdapter so savepoints stay
// invisible to callers.
func (a pgxProviderAdapter) Begin(ctx context.Context) (Tx, error) {
	tx, err := a.src.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return newPgxTxAdapter(tx), nil
}

// pgxTxAdapter adapts a pgx.Tx to the framework Tx. It embeds pgxProviderAdapter
// for the query surface and adds Commit/Rollback.
type pgxTxAdapter struct {
	pgxProviderAdapter
	tx pgx.Tx
}

func newPgxTxAdapter(tx pgx.Tx) pgxTxAdapter {
	return pgxTxAdapter{
		pgxProviderAdapter: pgxProviderAdapter{src: tx},
		tx:                 tx,
	}
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
	pgxProviderAdapter
}

// NewPoolProvider wraps a *pgxpool.Pool as a TxProvider. Use it at wiring time
// when you own a pool directly rather than going through PostgresDB.
func NewPoolProvider(pool *pgxpool.Pool) TxProvider {
	return poolAdapter{pgxProviderAdapter: pgxProviderAdapter{src: pool}}
}

var _ TxProvider = poolAdapter{}
var _ Tx = pgxTxAdapter{}
