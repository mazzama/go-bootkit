package databasekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mazzama/go-bootkit/core"
)

type txKey struct{}
type errRow struct{ err error }

func (e errRow) Scan(dest ...any) error {
	return e.err
}

type readyQuerier struct {
	provider TxProvider
}

func (r readyQuerier) wait(ctx context.Context) error {
	if rd, ok := r.provider.(core.Readyable); ok {
		select {
		case <-rd.Ready():
			return nil
		case <-ctx.Done():
			return fmt.Errorf("pool not ready: %w", ctx.Err())
		}
	}
	return nil
}

func (r readyQuerier) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	if err := r.wait(ctx); err != nil {
		return pgconn.CommandTag{}, err
	}
	return r.provider.Exec(ctx, sql, arguments...)
}

func (r readyQuerier) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if err := r.wait(ctx); err != nil {
		return nil, err
	}
	return r.provider.Query(ctx, sql, args...)
}

func (r readyQuerier) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if err := r.wait(ctx); err != nil {
		return errRow{err: err}
	}
	return r.provider.QueryRow(ctx, sql, args...)
}

// QuerierResolver resolves a Querier from context, allowing repositories to depend
// on a seam rather than the concrete TxManager.
type QuerierResolver interface {
	QuerierFromContext(ctx context.Context) Querier
}

// Querier is an interface that represents the common methods between pgxpool.Pool and pgx.Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type TxProvider interface {
	Querier
	Begin(ctx context.Context) (pgx.Tx, error)
}

type TxManager struct {
	provider TxProvider
}

func NewTxManager(provider TxProvider) *TxManager {
	return &TxManager{provider: provider}
}

// TxFromContext returns the pgx.Tx from the context if it exists.
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// QuerierFromContext returns the pgx.Tx from the context if it exists, otherwise it returns the configured TxProvider.
func (tm *TxManager) QuerierFromContext(ctx context.Context) Querier {
	if tx, ok := TxFromContext(ctx); ok {
		return tx
	}
	if _, ok := tm.provider.(core.Readyable); ok {
		return readyQuerier{provider: tm.provider}
	}
	return tm.provider
}

// WithTx executes the provided function within a transaction.
// If a transaction already exists in the context, a nested transaction (savepoint) is created.
func (tm *TxManager) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	var tx pgx.Tx
	var err error

	existingTx, hasTx := TxFromContext(ctx)
	if hasTx {
		// Create a nested transaction (savepoint) using the existing transaction
		tx, err = existingTx.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin nested transaction (savepoint): %w", err)
		}
	} else {
		if rd, ok := tm.provider.(core.Readyable); ok {
			select {
			case <-rd.Ready():
			case <-ctx.Done():
				return fmt.Errorf("pool not ready: %w", ctx.Err())
			}
		}
		// Begin a new transaction from the provider
		tx, err = tm.provider.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
	}

	defer func() {
		// Rollback if there's a panic or if the transaction hasn't been committed/rolled back yet.
		// pgx.Tx.Rollback is a no-op if the transaction is already closed.
		_ = tx.Rollback(ctx)
	}()

	// Put the new transaction in context
	ctxWithTx := context.WithValue(ctx, txKey{}, tx)

	if err := fn(ctxWithTx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && rollbackErr != pgx.ErrTxClosed {
			return fmt.Errorf("transaction failed: %v, rollback failed: %v", err, rollbackErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
