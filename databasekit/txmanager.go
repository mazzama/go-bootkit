package databasekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txKey struct{}

// Querier is an interface that represents the common methods between pgxpool.Pool and pgx.Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type TxManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// TxFromContext returns the pgx.Tx from the context if it exists.
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// QuerierFromContext returns the pgx.Tx from the context if it exists, otherwise it returns the provided pool.
func QuerierFromContext(ctx context.Context, pool *pgxpool.Pool) Querier {
	if tx, ok := TxFromContext(ctx); ok {
		return tx
	}
	return pool
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
		// Begin a new transaction from the pool
		tx, err = tm.pool.Begin(ctx)
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
