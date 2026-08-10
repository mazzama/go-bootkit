package core

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/core/healthkit"
)

// mockComponent implements both Component and Readyable interfaces.
type mockComponent struct {
	readyCh chan struct{}
	startFn func(ctx context.Context) error
	stopFn  func(ctx context.Context) error
	name    string
	started atomic.Bool
	stopped atomic.Bool
}

func (m *mockComponent) Name() string { return m.name }

func (m *mockComponent) Start(ctx context.Context) error {
	m.started.Store(true)
	if m.startFn != nil {
		return m.startFn(ctx)
	}
	<-ctx.Done()
	return nil
}

func (m *mockComponent) Stop(ctx context.Context) error {
	m.stopped.Store(true)
	if m.stopFn != nil {
		return m.stopFn(ctx)
	}
	return nil
}

func (m *mockComponent) Ready() <-chan struct{} {
	return m.readyCh
}

var _ Readyable = (*mockComponent)(nil)

func TestNewApplicationRunnerDefaults(t *testing.T) {
	r := NewApplicationRunner()

	if r.shutdownTimeout != 15*time.Second {
		t.Errorf("expected shutdownTimeout=15s, got %s", r.shutdownTimeout)
	}
	if r.startDeadline != 0 {
		t.Errorf("expected startDeadline=0, got %s", r.startDeadline)
	}
	if len(r.services) != 0 {
		t.Errorf("expected no services, got %d", len(r.services))
	}
	if r.HealthAggregator() == nil {
		t.Error("expected default healthAggregator to be non-nil")
	}
}

func TestWithShutdownTimeoutIgnoresNonPositive(t *testing.T) {
	r := NewApplicationRunner(WithShutdownTimeout(-5 * time.Second))

	if r.shutdownTimeout != 15*time.Second {
		t.Errorf("expected shutdownTimeout=15s (default), got %s", r.shutdownTimeout)
	}
}

func TestWithStartDeadlineIgnoresNonPositive(t *testing.T) {
	r := NewApplicationRunner(WithStartDeadline(0))

	if r.startDeadline != 0 {
		t.Errorf("expected startDeadline=0 (default), got %s", r.startDeadline)
	}
}

func TestWithServices(t *testing.T) {
	svc1 := &mockComponent{name: "svc1", readyCh: make(chan struct{})}
	svc2 := &mockComponent{name: "svc2", readyCh: make(chan struct{})}

	r := NewApplicationRunner(WithServices(svc1, svc2))

	if len(r.services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(r.services))
	}
	if r.services[0].Name() != "svc1" {
		t.Errorf("expected first service name=svc1, got %s", r.services[0].Name())
	}
	if r.services[1].Name() != "svc2" {
		t.Errorf("expected second service name=svc2, got %s", r.services[1].Name())
	}
}

func TestRunStartsAndStopsServices(t *testing.T) {
	readyCh := make(chan struct{})
	svc := &mockComponent{
		name:    "test-svc",
		readyCh: readyCh,
		startFn: func(ctx context.Context) error {
			close(readyCh)
			<-ctx.Done()
			return nil
		},
	}

	r := NewApplicationRunner(WithServices(svc))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = r.Run(ctx)

	if !svc.started.Load() {
		t.Error("expected service to be started")
	}
	if !svc.stopped.Load() {
		t.Error("expected service to be stopped")
	}
}

func TestRunReturnsServiceStartError(t *testing.T) {
	readyCh := make(chan struct{})
	svc := &mockComponent{
		name:    "failing-svc",
		readyCh: readyCh,
		startFn: func(_ context.Context) error {
			return errors.New("start failed")
		},
	}

	r := NewApplicationRunner(WithServices(svc))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := r.Run(ctx)
	if err == nil {
		t.Fatal("expected an error from Run")
	}
	if !strings.Contains(err.Error(), "failing-svc: start failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunStartDeadlineExceeded(t *testing.T) {
	// readyCh is never closed, so the deadline will be exceeded.
	svc := &mockComponent{
		name:    "slow-svc",
		readyCh: make(chan struct{}),
	}

	r := NewApplicationRunner(
		WithServices(svc),
		WithStartDeadline(100*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := r.Run(ctx)
	if err == nil {
		t.Fatal("expected an error from Run")
	}
	expected := "slow-svc: start deadline exceeded (100ms)"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error to contain %q, got %q", expected, err.Error())
	}
}

func TestRunLogsShutdownErrors(t *testing.T) {
	readyCh := make(chan struct{})
	svc := &mockComponent{
		name:    "failing-shutdown",
		readyCh: readyCh,
		startFn: func(ctx context.Context) error {
			close(readyCh)
			<-ctx.Done()
			return nil
		},
		stopFn: func(_ context.Context) error {
			return errors.New("shutdown failed")
		},
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	r := NewApplicationRunner(
		WithServices(svc),
		WithLogger(logger),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = r.Run(ctx)

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "shutdown failed") {
		t.Errorf("expected log output to contain 'shutdown failed', got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "failing-shutdown") {
		t.Errorf("expected log output to contain 'failing-shutdown', got: %s", logOutput)
	}
}

type mockHealthComponent struct {
	mockComponent
	checks []healthkit.Check
}

func (m *mockHealthComponent) HealthChecks() []healthkit.Check {
	return m.checks
}

func TestRunPropagatesHealthChecks(t *testing.T) {
	svc := &mockHealthComponent{
		mockComponent: mockComponent{
			name:    "health-svc",
			readyCh: make(chan struct{}),
			startFn: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
		},
		checks: []healthkit.Check{
			{
				Name: "custom-check",
				Kind: healthkit.Liveness,
				Fn:   func(ctx context.Context) error { return nil },
			},
		},
	}

	r := NewApplicationRunner(
		WithServices(svc),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = r.Run(ctx)

	agg := r.HealthAggregator()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	agg.Handler(healthkit.Liveness).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK from health handler, got %d", rec.Code)
	}
}

func TestWithHealthCacheTTL(t *testing.T) {
	r := NewApplicationRunner(WithHealthCacheTTL(10 * time.Second))
	if r.HealthAggregator() == nil {
		t.Error("expected non-nil healthAggregator when setting cache TTL")
	}
}

func TestRunShutdownOrder(t *testing.T) {
	var stopOrder []string
	var mu sync.Mutex

	createSvc := func(name string) *mockComponent {
		readyCh := make(chan struct{})
		return &mockComponent{
			name:    name,
			readyCh: readyCh,
			startFn: func(ctx context.Context) error {
				close(readyCh)
				<-ctx.Done()
				return nil
			},
			stopFn: func(ctx context.Context) error {
				mu.Lock()
				stopOrder = append(stopOrder, name)
				mu.Unlock()
				return nil
			},
		}
	}

	svc1 := createSvc("svc1")
	svc2 := createSvc("svc2")
	svc3 := createSvc("svc3")

	r := NewApplicationRunner(WithServices(svc1, svc2, svc3))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = r.Run(ctx)

	expectedOrder := []string{"svc3", "svc2", "svc1"}
	if len(stopOrder) != 3 {
		t.Fatalf("expected 3 stopped services, got %d", len(stopOrder))
	}
	for i, name := range expectedOrder {
		if stopOrder[i] != name {
			t.Errorf("expected stopped service at index %d to be %s, got %s", i, name, stopOrder[i])
		}
	}
}

func TestRunReturnsShutdownErrors(t *testing.T) {
	readyCh1 := make(chan struct{})
	readyCh2 := make(chan struct{})

	svc1 := &mockComponent{
		name:    "svc1",
		readyCh: readyCh1,
		startFn: func(ctx context.Context) error {
			close(readyCh1)
			<-ctx.Done()
			return nil
		},
		stopFn: func(ctx context.Context) error {
			return errors.New("svc1 failed to stop")
		},
	}

	svc2 := &mockComponent{
		name:    "svc2",
		readyCh: readyCh2,
		startFn: func(ctx context.Context) error {
			close(readyCh2)
			<-ctx.Done()
			return nil
		},
		stopFn: func(ctx context.Context) error {
			return errors.New("svc2 failed to stop")
		},
	}

	r := NewApplicationRunner(WithServices(svc1, svc2))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := r.Run(ctx)
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	// Verify both errors are returned in the joined error
	if !strings.Contains(err.Error(), "svc1 failed to stop") {
		t.Errorf("expected error to contain svc1 failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), "svc2 failed to stop") {
		t.Errorf("expected error to contain svc2 failure, got: %v", err)
	}
}

type recordingHooks struct {
	mu           sync.Mutex
	starts       []string
	stops        []string
	healthChecks []healthkit.Kind
}

func (r *recordingHooks) OnComponentStart(name string, duration time.Duration, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append(r.starts, name)
}
func (r *recordingHooks) OnComponentStop(name string, duration time.Duration, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stops = append(r.stops, name)
}
func (r *recordingHooks) OnHealthEvaluated(kind healthkit.Kind, duration time.Duration, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.healthChecks = append(r.healthChecks, kind)
}

func TestRunHooksAreCalled(t *testing.T) {
	hooks := &recordingHooks{}
	readyCh := make(chan struct{})
	svc := &mockHealthComponent{
		mockComponent: mockComponent{
			name:    "hooked-svc",
			readyCh: readyCh,
			startFn: func(ctx context.Context) error {
				close(readyCh)
				<-ctx.Done()
				return nil
			},
		},
		checks: []healthkit.Check{
			{Name: "check", Kind: healthkit.Liveness, Fn: func(ctx context.Context) error { return nil }},
		},
	}

	r := NewApplicationRunner(
		WithServices(svc),
		WithHooks(hooks),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = r.Run(ctx)

	// Trigger health check to ensure hook fires
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.HealthAggregator().Handler(healthkit.Liveness).ServeHTTP(rec, req)

	hooks.mu.Lock()
	defer hooks.mu.Unlock()

	if len(hooks.starts) != 1 || hooks.starts[0] != "hooked-svc" {
		t.Errorf("expected 1 start hook for hooked-svc, got %v", hooks.starts)
	}
	if len(hooks.stops) != 1 || hooks.stops[0] != "hooked-svc" {
		t.Errorf("expected 1 stop hook for hooked-svc, got %v", hooks.stops)
	}
	if len(hooks.healthChecks) != 1 || hooks.healthChecks[0] != healthkit.Liveness {
		t.Errorf("expected 1 health hook for Liveness, got %v", hooks.healthChecks)
	}
}
