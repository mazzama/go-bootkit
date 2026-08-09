package databasekit

import (
	"context"
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

func TestWithConnectionString(t *testing.T) {
	db := &PostgresDB{}
	WithConnectionString("postgres://user:pass@host:5432/mydb")(db)
	if db.connStr != "postgres://user:pass@host:5432/mydb" {
		t.Errorf("unexpected connStr: %q", db.connStr)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPostgresDB(WithConnectionString(tt.connStr))
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPostgresDB() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLazyProviderBlocksUntilContextDeadline(t *testing.T) {
	db := &PostgresDB{}
	provider := db.TxProvider()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := provider.Exec(ctx, "SELECT 1")
	if err == nil {
		t.Fatal("expected error due to deadline, got nil")
	}
	if err.Error() != "pool not ready: context deadline exceeded" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLazyProviderQueryRowReturnsError(t *testing.T) {
	db := &PostgresDB{}
	provider := db.TxProvider()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	row := provider.QueryRow(ctx, "SELECT 1")
	var val int
	err := row.Scan(&val)
	if err == nil {
		t.Fatal("expected error due to deadline, got nil")
	}
	if err.Error() != "pool not ready: context deadline exceeded" {
		t.Errorf("unexpected error from Scan: %v", err)
	}
}
