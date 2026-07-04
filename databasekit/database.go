// Package databasekit provides a PostgreSQL database component.
//
// The PostgresDB type wraps pgxpool to provide a core.Component implementation
// with health checks and lifecycle management. It supports connection pooling,
// query execution, and transaction management.
//
// # Features
//
//   - Connection pooling via pgxpool
//   - Kubernetes-style health checks (liveness/readiness)
//   - Thread-safe pool access
//   - Transaction support
//   - Query and execution methods
//
// # Health Checks
//
// The component provides health checks that verify connectivity and readiness:
//   - Liveness: Verifies the database connection pool is accessible
//   - Readiness: Verifies the component has completed startup and is ready
//
// # Connection String
//
// The connection string follows PostgreSQL URI format:
//
//	postgres://user:password@host:port/database?sslmode=disable
//
// Example:
//
//	db, err := databasekit.NewPostgresDB(ctx,
//	    databasekit.WithConnectionString("postgres://user:pass@localhost:5432/mydb"),
//	    databasekit.WithDBName("main-db"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Register health checks
//	health.Register(db.HealthChecks()...)
//
//	// Use with ApplicationRunner
//	runner := core.NewApplicationRunner(
//	    core.WithServices(db),
//	)
package databasekit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

// PostgresDB is a PostgreSQL database component that implements core.Component.
//
// The PostgresDB wraps a pgxpool.Pool to provide connection pooling, query
// execution, and transaction management. It implements the Component and
// Readyable interfaces for lifecycle management.
//
// # Lifecycle
//
// On creation (NewPostgresDB), the component validates the connection string
// and stores the parsed configuration. Connection is deferred to Start(),
// which establishes the pool and verifies connectivity before signaling
// readiness. The Stop method closes the connection pool.
//
// # Thread Safety
//
// All methods are thread-safe. The Pool() method uses an RWMutex to protect
// access to the underlying pool reference.
//
// Example:
//
//	db, err := databasekit.NewPostgresDB(ctx,
//	    databasekit.WithConnectionString("postgres://user:pass@localhost:5432/mydb"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Execute a query
//	rows, err := db.Query(ctx, "SELECT * FROM users WHERE id = $1", userID)
//
//	// Execute a command
//	rowsAffected, err := db.Exec(ctx, "UPDATE users SET name = $1 WHERE id = $2", name, userID)
//
//	// Run a transaction
//	tx, err := db.Begin(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// ... perform transaction operations
//	if err := tx.Commit(ctx); err != nil {
//	    tx.Rollback(ctx)
//	}
type PostgresDB struct {
	name      string
	connStr   string
	config    *pgxpool.Config
	pool      *pgxpool.Pool
	mu        sync.RWMutex
	readyChan chan struct{}
}

// PostgresOption is a functional option for configuring a PostgresDB.
//
// Options are applied in the order provided to NewPostgresDB. See the With*
// functions for available options.
type PostgresOption func(*PostgresDB)

// NewPostgresDB creates a new PostgreSQL database component.
//
// The component validates the connection string and stores the parsed
// configuration, but does not connect to the database. Connection is
// deferred to Start(), which is called by ApplicationRunner.
//
// Default connection string: postgres://postgres:postgres@localhost:5432/postgres
//
// Example:
//
//	db, err := databasekit.NewPostgresDB(ctx,
//	    databasekit.WithConnectionString("postgres://user:pass@localhost:5432/mydb"),
//	    databasekit.WithDBName("main-db"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewPostgresDB(ctx context.Context, options ...PostgresOption) (*PostgresDB, error) {
	db := &PostgresDB{
		name:      "postgres-db",
		connStr:   "postgres://postgres:postgres@localhost:5432/postgres",
		readyChan: make(chan struct{}),
	}

	for _, option := range options {
		option(db)
	}

	config, errParse := pgxpool.ParseConfig(db.connStr)
	if errParse != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", errParse)
	}

	db.config = config
	return db, nil
}

// WithDBName sets the database component name.
//
// The name is used for logging, health check names, and component identification.
//
// Example:
//
//	db, err := databasekit.NewPostgresDB(ctx,
//	    databasekit.WithDBName("analytics-db"),
//	)
func WithDBName(name string) PostgresOption {
	return func(db *PostgresDB) {
		db.name = name
	}
}

// WithConnectionString sets the PostgreSQL connection string.
//
// The connection string follows PostgreSQL URI format:
//
//	postgres://user:password@host:port/database?sslmode=disable
//
// Example:
//
//	db, err := databasekit.NewPostgresDB(ctx,
//	    databasekit.WithConnectionString("postgres://user:pass@localhost:5432/mydb"),
//	)
func WithConnectionString(connStr string) PostgresOption {
	return func(db *PostgresDB) {
		db.connStr = connStr
	}
}

// Name returns the database component's identifier.
func (db *PostgresDB) Name() string {
	return db.name
}

// Start begins the database component's operation.
//
// Start establishes the connection pool and verifies connectivity by pinging
// the database. Once the connection is confirmed, it signals readiness by
// closing the ready channel, then blocks until the context is canceled.
//
// This method is typically called by ApplicationRunner.
func (db *PostgresDB) Start(ctx context.Context) error {
	pool, err := pgxpool.NewWithConfig(ctx, db.config)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	if errPing := pool.Ping(ctx); errPing != nil {
		pool.Close()
		return fmt.Errorf("failed to connect to database: %w", errPing)
	}

	db.mu.Lock()
	db.pool = pool
	db.mu.Unlock()

	close(db.readyChan)
	<-ctx.Done()
	return nil
}

// Stop gracefully shuts down the database component.
//
// Stop closes the connection pool. In-flight queries and transactions
// may be interrupted. The context deadline determines how long to wait
// before forcefully closing connections.
//
// This method is typically called by ApplicationRunner during shutdown.
func (db *PostgresDB) Stop(ctx context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.pool != nil {
		db.pool.Close()
	}
	return nil
}

// Ready returns a channel that closes when the database is ready.
//
// The channel closes when Start is called, signaling that the component
// has completed initialization and is ready to serve requests.
func (db *PostgresDB) Ready() <-chan struct{} {
	return db.readyChan
}

// Pool returns the underlying pgxpool.Pool.
//
// Use this method to access the pool directly for operations not covered
// by the component's methods. Access is thread-safe.
//
// Example:
//
//	pool := db.Pool()
//	conn, err := pool.Acquire(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer conn.Release()
func (db *PostgresDB) Pool() *pgxpool.Pool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.pool
}

// Exec executes a SQL command that doesn't return rows.
//
// Exec is typically used for INSERT, UPDATE, DELETE, and DDL statements.
// It returns the number of rows affected by the command.
//
// Example:
//
//	rowsAffected, err := db.Exec(ctx,
//	    "UPDATE users SET last_login = $1 WHERE id = $2",
//	    time.Now(),
//	    userID,
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Updated %d rows\n", rowsAffected)
func (db *PostgresDB) Exec(ctx context.Context, sql string, args ...interface{}) (int64, error) {
	result, err := db.Pool().Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// Query executes a SQL query that returns multiple rows.
//
// Query is typically used for SELECT statements that return multiple rows.
// The returned rows must be closed when no longer needed.
//
// Example:
//
//	rows, err := db.Query(ctx, "SELECT id, name FROM users WHERE active = $1", true)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer rows.Close()
//
//	for rows.Next() {
//	    var id int
//	    var name string
//	    if err := rows.Scan(&id, &name); err != nil {
//	        log.Fatal(err)
//	    }
//	    fmt.Printf("User %d: %s\n", id, name)
//	}
func (db *PostgresDB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.Pool().Query(ctx, sql, args...)
}

// QueryRow executes a SQL query that returns a single row.
//
// QueryRow is typically used for SELECT statements that return exactly one row.
// Use pgx.Row.Scan to read the result.
//
// Example:
//
//	var name string
//	err := db.QueryRow(ctx, "SELECT name FROM users WHERE id = $1", userID).Scan(&name)
//	if err == pgx.ErrNoRows {
//	    fmt.Println("User not found")
//	} else if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("User name: %s\n", name)
func (db *PostgresDB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.Pool().QueryRow(ctx, sql, args...)
}

// Begin starts a new transaction.
//
// The returned pgx.Tx must be committed or rolled back when done.
// Use defer to ensure cleanup in case of errors.
//
// Example:
//
//	tx, err := db.Begin(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer tx.Rollback(ctx) // Safe to call even after Commit
//
//	// Perform transaction operations
//	_, err = tx.Exec(ctx, "INSERT INTO orders (user_id, total) VALUES ($1, $2)", userID, total)
//	if err != nil {
//	    return err // tx.Rollback(ctx) called via defer
//	}
//
//	if err := tx.Commit(ctx); err != nil {
//	    return err
//	}
func (db *PostgresDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return db.Pool().Begin(ctx)
}

// HealthChecks returns health checks for the database component.
//
// The returned checks include liveness and readiness probes:
//   - Liveness: Verifies the connection pool is accessible by pinging the database
//   - Readiness: Verifies the component has signaled ready via the Ready() channel
//
// These checks should be registered with a healthkit.Aggregator:
//
//	health.Register(db.HealthChecks()...)
//
// Example:
//
//	health := healthkit.NewAggregator(5 * time.Second)
//	health.Register(db.HealthChecks()...)
//	router.Get("/health/liveness", health.Handler(healthkit.Liveness))
func (db *PostgresDB) HealthChecks() []healthkit.Check {
	return []healthkit.Check{
		{
			Name:    db.name + "-liveness",
			Kind:    healthkit.Liveness,
			Timeout: 2 * time.Second,
			Fn: func(ctx context.Context) error {
				if db.pool == nil {
					return fmt.Errorf("db is not initialized")
				}
				return db.pool.Ping(ctx)
			},
		},
		{
			Name:    db.name + "-readiness",
			Kind:    healthkit.Readiness,
			Timeout: 2 * time.Second,
			Fn: func(ctx context.Context) error {
				if db.pool == nil {
					return fmt.Errorf("db is not initialized")
				}
				select {
				case <-db.Ready():
					return nil
				case <-ctx.Done():
					return ctx.Err()
				default:
					return fmt.Errorf("db is not ready")
				}
			},
		},
	}
}

var _ core.Component = (*PostgresDB)(nil)
var _ core.Readyable = (*PostgresDB)(nil)
