package databasekit

import (
	"testing"

	"github.com/mazzama/go-bootkit/core/healthkit"
)

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

func TestReadyChannel(t *testing.T) {
	db := &PostgresDB{readyChan: make(chan struct{})}
	ch := db.Ready()
	if ch == nil {
		t.Fatal("expected non-nil ready channel")
	}

	select {
	case <-ch:
		t.Fatal("expected channel to be open (not ready yet)")
	default:
		// correct
	}
}

func TestHealthChecksReturnsTwoChecks(t *testing.T) {
	db := &PostgresDB{
		name:      "test-pg",
		readyChan: make(chan struct{}),
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
	if checks[0].Timeout != 0 {
		t.Errorf("expected 0s timeout, got %v", checks[0].Timeout)
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
		name:      "test-pg",
		readyChan: make(chan struct{}),
	}

	checks := db.HealthChecks()
	err := checks[0].Fn(t.Context())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestHealthCheckReadinessReturnsErrorWhenPoolNil(t *testing.T) {
	db := &PostgresDB{
		name:      "test-pg",
		readyChan: make(chan struct{}),
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
