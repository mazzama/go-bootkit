package databasekit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mazzama/go-bootkit/databasekit"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(ctx context.Context, t *testing.T) (*databasekit.PostgresDB, func()) {
	t.Helper()
	
	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(15*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	db := databasekit.NewPostgresDB(
		databasekit.WithConnectionString(connStr),
	)
	
	errCh := make(chan error, 1)
	go func() {
		errCh <- db.Start(ctx)
	}()
	
	select {
	case <-db.Ready():
	case err := <-errCh:
		t.Fatalf("failed to start db: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for db to start")
	}
	
	// Create table
	_, err = db.Pool().Exec(ctx, "CREATE TABLE IF NOT EXISTS items (id SERIAL PRIMARY KEY, name TEXT UNIQUE)")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	cleanup := func() {
		db.Stop(context.Background())
		if err := pgContainer.Terminate(context.Background()); err != nil {
			t.Fatalf("failed to terminate container: %v", err)
		}
	}
	
	return db, cleanup
}

func countItems(ctx context.Context, t *testing.T, querier databasekit.Querier) int {
	t.Helper()
	var count int
	err := querier.QueryRow(ctx, "SELECT COUNT(*) FROM items").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count items: %v", err)
	}
	return count
}

func TestTxManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Canceling the context will stop the db
	
	db, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	tm := databasekit.NewTxManager(db.Pool())

	t.Run("QuerierFromContext without tx", func(t *testing.T) {
		querier := databasekit.QuerierFromContext(ctx, db.Pool())
		if _, ok := querier.(pgx.Tx); ok {
			t.Fatal("expected querier to be a pool, got tx")
		}
	})

	t.Run("Basic commit", func(t *testing.T) {
		err := tm.WithTx(ctx, func(ctxTx context.Context) error {
			querier := databasekit.QuerierFromContext(ctxTx, db.Pool())
			if _, ok := querier.(pgx.Tx); !ok {
				t.Fatal("expected querier to be a tx")
			}
			_, err := querier.Exec(ctxTx, "INSERT INTO items (name) VALUES ('item1')")
			return err
		})
		
		if err != nil {
			t.Fatalf("WithTx failed: %v", err)
		}
		
		if count := countItems(ctx, t, db.Pool()); count != 1 {
			t.Errorf("expected 1 item, got %d", count)
		}
	})

	t.Run("Rollback on error", func(t *testing.T) {
		err := tm.WithTx(ctx, func(ctxTx context.Context) error {
			querier := databasekit.QuerierFromContext(ctxTx, db.Pool())
			_, err := querier.Exec(ctxTx, "INSERT INTO items (name) VALUES ('item2')")
			if err != nil {
				return err
			}
			return fmt.Errorf("intentional rollback")
		})
		
		if err == nil || err.Error() != "intentional rollback" {
			t.Fatalf("expected 'intentional rollback', got %v", err)
		}
		
		if count := countItems(ctx, t, db.Pool()); count != 1 {
			t.Errorf("expected 1 item (item1), got %d", count)
		}
	})

	t.Run("Nested savepoint commit", func(t *testing.T) {
		err := tm.WithTx(ctx, func(ctxTx1 context.Context) error {
			q1 := databasekit.QuerierFromContext(ctxTx1, db.Pool())
			_, err := q1.Exec(ctxTx1, "INSERT INTO items (name) VALUES ('item3')")
			if err != nil {
				return err
			}
			
			// Nested tx
			err = tm.WithTx(ctxTx1, func(ctxTx2 context.Context) error {
				q2 := databasekit.QuerierFromContext(ctxTx2, db.Pool())
				_, err := q2.Exec(ctxTx2, "INSERT INTO items (name) VALUES ('item4')")
				return err
			})
			if err != nil {
				return err
			}
			
			return nil
		})
		
		if err != nil {
			t.Fatalf("nested WithTx failed: %v", err)
		}
		
		if count := countItems(ctx, t, db.Pool()); count != 3 {
			t.Errorf("expected 3 items (item1, item3, item4), got %d", count)
		}
	})

	t.Run("Nested savepoint rollback", func(t *testing.T) {
		err := tm.WithTx(ctx, func(ctxTx1 context.Context) error {
			q1 := databasekit.QuerierFromContext(ctxTx1, db.Pool())
			_, err := q1.Exec(ctxTx1, "INSERT INTO items (name) VALUES ('item5')")
			if err != nil {
				return err
			}
			
			// Nested tx that fails
			err = tm.WithTx(ctxTx1, func(ctxTx2 context.Context) error {
				q2 := databasekit.QuerierFromContext(ctxTx2, db.Pool())
				_, err := q2.Exec(ctxTx2, "INSERT INTO items (name) VALUES ('item6')")
				if err != nil {
					return err
				}
				return fmt.Errorf("nested intentional rollback")
			})
			if err == nil || err.Error() != "nested intentional rollback" {
				return fmt.Errorf("expected 'nested intentional rollback', got %v", err)
			}
			
			// Swallow the error from inner so outer can commit
			return nil
		})
		
		if err != nil {
			t.Fatalf("nested WithTx failed: %v", err)
		}
		
		if count := countItems(ctx, t, db.Pool()); count != 4 {
			t.Errorf("expected 4 items (item1, item3, item4, item5), got %d", count)
		}
	})
}
