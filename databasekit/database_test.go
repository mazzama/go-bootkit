package databasekit

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/core/healthkit"
)

func TestBuildPoolConfigDefaults(t *testing.T) {
	db := &PostgresDB{connStr: "postgres://user:pass@localhost:5432/mydb"}

	cfg, err := db.buildPoolConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With no sizing options, pgxpool's own default MaxConns applies.
	if cfg.MaxConns <= 0 {
		t.Errorf("expected a positive default MaxConns, got %d", cfg.MaxConns)
	}
}

func TestBuildPoolConfigAppliesSizingOptions(t *testing.T) {
	db := &PostgresDB{connStr: "postgres://user:pass@localhost:5432/mydb"}
	WithMaxConns(25)(db)
	WithMinConns(5)(db)

	cfg, err := db.buildPoolConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.MaxConns != 25 {
		t.Errorf("expected MaxConns 25, got %d", cfg.MaxConns)
	}
	if cfg.MinConns != 5 {
		t.Errorf("expected MinConns 5, got %d", cfg.MinConns)
	}
}

func TestBuildPoolConfigPreservesConnStrPoolSettings(t *testing.T) {
	// Sizing carried in the connection string is preserved when no option overrides it.
	db := &PostgresDB{connStr: "postgres://user:pass@localhost:5432/mydb?pool_max_conns=17"}

	cfg, err := db.buildPoolConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.MaxConns != 17 {
		t.Errorf("expected MaxConns 17 from connstr, got %d", cfg.MaxConns)
	}
}

func TestBuildPoolConfigOptionOverridesConnStr(t *testing.T) {
	// An explicit option wins over the connstr-carried value.
	db := &PostgresDB{connStr: "postgres://user:pass@localhost:5432/mydb?pool_max_conns=17"}
	WithMaxConns(30)(db)

	cfg, err := db.buildPoolConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.MaxConns != 30 {
		t.Errorf("expected option MaxConns 30 to override connstr, got %d", cfg.MaxConns)
	}
}

func TestBuildPoolConfigInvalidConnStr(t *testing.T) {
	db := &PostgresDB{connStr: "://not-a-valid-dsn"}

	if _, err := db.buildPoolConfig(); err == nil {
		t.Error("expected error for invalid connection string, got nil")
	}
}

func TestWithDBName(t *testing.T) {
	db := &PostgresDB{name: "default"}
	WithDBName("custom-db")(db)
	if db.name != "custom-db" {
		t.Errorf("expected 'custom-db', got %q", db.name)
	}
}

func TestWithConnectRetry(t *testing.T) {
	db := &PostgresDB{}
	WithConnectRetry(5, 2*time.Second)(db)
	if db.retryAttempts != 5 {
		t.Errorf("expected 5 retry attempts, got %d", db.retryAttempts)
	}
	if db.retryBackoff != 2*time.Second {
		t.Errorf("expected 2s backoff, got %v", db.retryBackoff)
	}
}
func TestWithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &PostgresDB{}
	WithLogger(logger)(db)
	if db.logger != logger {
		t.Error("expected logger to be set")
	}
}

func TestPostgresDBMethodsWhenPoolNil(t *testing.T) {
	db := &PostgresDB{}
	ctx := t.Context()

	_, err := db.Exec(ctx, "SELECT 1")
	if err == nil || err.Error() != "database pool is not initialized" {
		t.Errorf("Exec unexpected error: %v", err)
	}

	_, err = db.Query(ctx, "SELECT 1")
	if err == nil || err.Error() != "database pool is not initialized" {
		t.Errorf("Query unexpected error: %v", err)
	}

	var dummy int
	err = db.QueryRow(ctx, "SELECT 1").Scan(&dummy)
	if err == nil || err.Error() != "database pool is not initialized" {
		t.Errorf("QueryRow unexpected error: %v", err)
	}

	_, err = db.Begin(ctx)
	if err == nil || err.Error() != "database pool is not initialized" {
		t.Errorf("Begin unexpected error: %v", err)
	}

	if db.TxProvider() != db {
		t.Error("TxProvider should return db")
	}
}

func TestPostgresDBConnectRetryFailure(t *testing.T) {
	db, err := NewPostgresDB("postgres://invalid:5432/db?sslmode=disable", WithConnectRetry(2, 1*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ctx := t.Context()
	err = db.Start(ctx)
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "failed to connect to database") && !strings.Contains(err.Error(), "failed to create connection pool") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPostgresDBConnectRetryContextCanceled(t *testing.T) {
	db, err := NewPostgresDB("postgres://invalid:5432/db?sslmode=disable", WithConnectRetry(5, 50*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = db.Start(ctx)
	if err == nil {
		t.Fatal("expected context error")
	}
	if !strings.Contains(err.Error(), "context canceled during retry backoff") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPostgresDBName(t *testing.T) {
	db := &PostgresDB{name: "test-db"}
	if db.Name() != "test-db" {
		t.Errorf("expected 'test-db', got %q", db.Name())
	}
}

func TestHealthChecksReturnsTwoChecks(t *testing.T) {
	db := &PostgresDB{
		name: "test-pg",
	}

	checks := db.HealthChecks()
	if len(checks) != 2 {
		t.Fatalf("expected 2 health checks, got %d", len(checks))
	}

	if checks[0].Name != "test-pg-liveness" {
		t.Errorf("expected 'test-pg-liveness', got %q", checks[0].Name)
	}
	if checks[0].Kind != healthkit.Liveness {
		t.Errorf("expected Liveness kind")
	}

	if checks[1].Name != "test-pg-readiness" {
		t.Errorf("expected 'test-pg-readiness', got %q", checks[1].Name)
	}
	if checks[1].Kind != healthkit.Readiness {
		t.Errorf("expected Readiness kind")
	}
}

func TestHealthCheckLivenessReturnsNil(t *testing.T) {
	db := &PostgresDB{
		name: "test-pg",
	}

	checks := db.HealthChecks()
	err := checks[0].Fn(t.Context())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestHealthCheckReadinessReturnsErrorWhenPoolNil(t *testing.T) {
	db := &PostgresDB{
		name: "test-pg",
	}

	checks := db.HealthChecks()
	err := checks[1].Fn(t.Context())
	if err == nil {
		t.Fatal("expected error when pool is nil")
	}
	if err.Error() != "db is not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewPostgresDBValidation(t *testing.T) {
	tests := []struct {
		name    string
		connStr string
		wantErr bool
	}{
		{
			name:    "valid connection string",
			connStr: "postgres://user:pass@localhost:5432/mydb",
			wantErr: false,
		},
		{
			name:    "invalid connection string",
			connStr: "://not-a-valid-dsn",
			wantErr: true,
		},
		{
			name:    "empty connection string",
			connStr: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPostgresDB(tt.connStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPostgresDB() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTxManagerBlocksUntilContextDeadline(t *testing.T) {
	db := &PostgresDB{}
	tm := NewTxManager(db.TxProvider())
	querier := tm.QuerierFromContext(context.Background())

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := querier.Exec(ctx, "SELECT 1")
	if err == nil {
		t.Fatal("expected error due to deadline, got nil")
	}
	if err.Error() != "pool not ready: context deadline exceeded" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTxManagerQueryRowReturnsError(t *testing.T) {
	db := &PostgresDB{}
	tm := NewTxManager(db.TxProvider())
	querier := tm.QuerierFromContext(context.Background())

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	row := querier.QueryRow(ctx, "SELECT 1")
	var val int
	err := row.Scan(&val)
	if err == nil {
		t.Fatal("expected error due to deadline, got nil")
	}
	if err.Error() != "pool not ready: context deadline exceeded" {
		t.Errorf("unexpected error from Scan: %v", err)
	}
}
