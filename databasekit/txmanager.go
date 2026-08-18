package databasekit

import (
	"context"
	"errors"
	"fmt"

	"github.com/mazzama/go-bootkit/core"
)

// Row is the result of a single-row query. The error is deferred: everything —
// readiness-gate failure, no rows, driver error — surfaces at Scan. It never
// returns nil and never returns an error at call time. Scan is callable exactly
// once.
type Row interface {
	Scan(dest ...any) error
}

// Rows is a multi-row result. It owns a connection until Close or exhaustion.
// Next/Scan/Err follow database/sql ordering: loop Next, Scan inside the loop,
// then check Err after the loop. Close is idempotent.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// Querier is the pgx-free query surface. Callers never import pgx or pgconn,
// and never mention pgx.Row, pgx.Rows, or pgconn.CommandTag.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (int64, error)
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
}

// Tx is a live transaction: a Querier that can commit, roll back, and open
// nested savepoints.
type Tx interface {
	TxProvider
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// TxProvider extends Querier with a transaction-opening entry point.
// Begin returns the framework Tx, not pgx.Tx.
type TxProvider interface {
	Querier
	Begin(ctx context.Context) (Tx, error)
}

// QuerierResolver resolves a Querier from context, allowing repositories to
// depend on a seam rather than the concrete TxManager.
type QuerierResolver interface {
	QuerierFromContext(ctx context.Context) Querier
}

type txKey struct{}

// errRow is a Row whose Scan returns a fixed error. It carries readiness-gate
// failures and nil-pool guards through the deferred QueryRow contract.
type errRow struct{ err error }

// readyQuerier gates a TxProvider on readiness before delegating, so a query
// issued before the pool connects blocks up to ctx's deadline rather than
// racing the connection.
type readyQuerier struct {
	provider TxProvider
}

// TxManager hands out a Querier from context (the active Tx, or the provider
// itself when no transaction is in flight) and runs transactional work via
// WithTx.
type TxManager struct {
	provider TxProvider
}

// ErrNoRows is the framework-native no-rows sentinel. It is the only error a
// repository must recognize by identity when a single-row query returns nothing.
// The pgx adapter rewrites pgx.ErrNoRows to it, so repositories never import pgx.
var ErrNoRows = errors.New("databasekit: no rows in result set")

// ErrTxClosed marks an operation on a committed or rolled-back transaction.
// The pgx adapter rewrites pgx.ErrTxClosed to it, so WithTx never names pgx.
var ErrTxClosed = errors.New("databasekit: transaction is closed")

func (e errRow) Scan(dest ...any) error {
	return e.err
}

func (r readyQuerier) wait(ctx context.Context) error {
	if rd, ok := r.provider.(core.Readyable); ok {
		if err := core.WaitReady(ctx, rd.Ready()); err != nil {
			return fmt.Errorf("pool not ready: %w", err)
		}
	}
	return nil
}

func (r readyQuerier) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	if err := r.wait(ctx); err != nil {
		return 0, err
	}
	return r.provider.Exec(ctx, sql, args...)
}

func (r readyQuerier) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	if err := r.wait(ctx); err != nil {
		return nil, err
	}
	return r.provider.Query(ctx, sql, args...)
}

func (r readyQuerier) QueryRow(ctx context.Context, sql string, args ...any) Row {
	if err := r.wait(ctx); err != nil {
		return errRow{err: err}
	}
	return r.provider.QueryRow(ctx, sql, args...)
}

// NewTxManager creates a TxManager over the given provider.
func NewTxManager(provider TxProvider) *TxManager {
	return &TxManager{provider: provider}
}

// TxFromContext returns the active framework Tx from the context, if any.
func TxFromContext(ctx context.Context) (Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(Tx)
	return tx, ok
}

// QuerierFromContext returns the active Tx if one is on the context, otherwise
// the configured TxProvider — readiness-gated when it is Readyable.
func (tm *TxManager) QuerierFromContext(ctx context.Context) Querier {
	if tx, ok := TxFromContext(ctx); ok {
		return tx
	}
	if _, ok := tm.provider.(core.Readyable); ok {
		return readyQuerier{provider: tm.provider}
	}
	return tm.provider
}

// WithTx executes fn within a transaction. If a transaction already exists on
// the context, a nested savepoint is created instead.
func (tm *TxManager) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	var tx Tx
	var err error

	existingTx, hasTx := TxFromContext(ctx)
	if hasTx {
		// Create a nested transaction (savepoint) using the existing transaction.
		tx, err = existingTx.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin nested transaction (savepoint): %w", err)
		}
	} else {
		if rd, ok := tm.provider.(core.Readyable); ok {
			if waitErr := core.WaitReady(ctx, rd.Ready()); waitErr != nil {
				return fmt.Errorf("pool not ready: %w", waitErr)
			}
		}
		tx, err = tm.provider.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
	}

	defer func() {
		// Rollback if there's a panic or if the transaction hasn't been committed
		// or rolled back yet. Rollback is a no-op on an already-closed transaction.
		_ = tx.Rollback(ctx)
	}()

	ctxWithTx := context.WithValue(ctx, txKey{}, tx)

	if fnErr := fn(ctxWithTx); fnErr != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, ErrTxClosed) {
			return fmt.Errorf("transaction failed: %w, rollback failed: %w", fnErr, rollbackErr)
		}
		return fnErr
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		return fmt.Errorf("failed to commit transaction: %w", commitErr)
	}

	return nil
}
