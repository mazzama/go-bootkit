package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/mazzama/go-bootkit/core/healthkit"
)

// ApplicationRunner orchestrates the component lifecycle: it starts all
// registered services (with an optional per-service readiness deadline),
// wires health checks into the aggregator, and shuts services down in
// reverse registration order on context cancellation or signal.
type ApplicationRunner struct {
	logger           *slog.Logger
	services         []Component
	healthAggregator *healthkit.Aggregator
	startDeadline    time.Duration
	shutdownTimeout  time.Duration
	hooks            Hooks
	mu               sync.Mutex
}

// Option configures an ApplicationRunner at construction time.
type Option func(*ApplicationRunner)

// NewApplicationRunner creates an ApplicationRunner with default shutdown
// timeout (15s) and health aggregator (1s cache floor). Pass Option values
// to override defaults.
func NewApplicationRunner(options ...Option) *ApplicationRunner {
	runner := &ApplicationRunner{
		shutdownTimeout:  15 * time.Second,           // Default shutdown timeout
		healthAggregator: healthkit.NewAggregator(0), // 0 applies 1s floor
		hooks:            NoOpHooks{},
	}

	for _, option := range options {
		option(runner)
	}

	return runner
}

// WithLogger sets the runner's logger, used for shutdown error reporting.
func WithLogger(l *slog.Logger) Option {
	return func(a *ApplicationRunner) {
		a.logger = l
	}
}

// WithShutdownTimeout sets the total budget for shutting down all services.
// Non-positive values are ignored (the default 15s applies).
func WithShutdownTimeout(d time.Duration) Option {
	return func(a *ApplicationRunner) {
		if d > 0 {
			a.shutdownTimeout = d
		}
	}
}

// WithStartDeadline sets how long each Readyable service may take to become
// ready before startup is aborted. Non-positive values disable the deadline.
func WithStartDeadline(d time.Duration) Option {
	return func(a *ApplicationRunner) {
		if d > 0 {
			a.startDeadline = d
		}
	}
}

// WithServices registers the components the runner starts and stops.
func WithServices(services ...Component) Option {
	return func(a *ApplicationRunner) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.services = append(a.services, services...)
	}
}

// WithHooks installs an observability hook receiving lifecycle and health
// evaluation events. Nil hooks are ignored.
func WithHooks(h Hooks) Option {
	return func(a *ApplicationRunner) {
		if h != nil {
			a.hooks = h
		}
	}
}

// HealthAggregator returns the runner's health check aggregator. Use it to wire
// health endpoints into your HTTP router before calling Run.
func (r *ApplicationRunner) HealthAggregator() *healthkit.Aggregator {
	return r.healthAggregator
}

// WithHealthCacheTTL sets the cache TTL for the health check aggregator.
// A value of 0 applies a safe 1s floor; a negative value disables caching.
func WithHealthCacheTTL(d time.Duration) Option {
	return func(a *ApplicationRunner) {
		a.healthAggregator = healthkit.NewAggregator(d)
	}
}

// WithHealthAggregator overrides the runner's internal health aggregator.
//
// Deprecated: prefer WithHealthCacheTTL to configure the default aggregator's
// cache duration.
func WithHealthAggregator(agg *healthkit.Aggregator) Option {
	return func(a *ApplicationRunner) {
		a.healthAggregator = agg
	}
}

// Run starts all services, blocks until ctx is cancelled or a signal is
// received, then shuts everything down and joins any start/shutdown errors.
func (r *ApplicationRunner) Run(ctx context.Context) error {
	r.healthWiring()

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startErr := r.startSupervisor(ctx)
	shutdownErrs := r.shutdownOrchestrator(ctx)

	var allErrs []error
	if startErr != nil {
		allErrs = append(allErrs, startErr)
	}
	allErrs = append(allErrs, shutdownErrs...)

	return errors.Join(allErrs...)
}

func (r *ApplicationRunner) healthWiring() {
	r.healthAggregator.SetHook(func(kind healthkit.Kind, duration time.Duration, err error) {
		r.hooks.OnHealthEvaluated(kind.String(), duration, err)
	})

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.services {
		if provider, ok := s.(HealthCheckProvider); ok {
			r.healthAggregator.Register(provider.HealthChecks()...)
		}
	}
}

func (r *ApplicationRunner) startSupervisor(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	for _, s := range r.services {
		svc := s
		svcCtx, cancelSvc := context.WithCancel(ctx)

		// Start loop
		eg.Go(func() error {
			defer cancelSvc()
			start := time.Now()
			err := svc.Start(svcCtx)
			duration := time.Since(start)
			r.hooks.OnComponentStart(svc.Name(), duration, err)

			if err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("%s: %w", svc.Name(), err)
			}
			return nil
		})

		if r.startDeadline > 0 {
			if rr, ok := svc.(Readyable); ok && rr.Ready() != nil {
				readyCh := rr.Ready()
				eg.Go(func() error {
					timer := time.NewTimer(r.startDeadline)
					defer timer.Stop()
					select {
					case <-readyCh:
						return nil
					case <-timer.C:
						cancelSvc()
						return fmt.Errorf("%s: start deadline exceeded (%s)", svc.Name(), r.startDeadline)
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			}
		}
	}

	return eg.Wait()
}

func (r *ApplicationRunner) shutdownOrchestrator(ctx context.Context) []error {
	var shutdownErrs []error
	remainingTimeout := r.shutdownTimeout

	// Shutdown sequentially in reverse registration order
	for i := len(r.services) - 1; i >= 0; i-- {
		svc := r.services[i]

		budget := remainingTimeout / time.Duration(i+1)
		start := time.Now()
		shCtx, cancel := context.WithTimeout(ctx, budget)

		stopErr := svc.Stop(shCtx)
		stopDuration := time.Since(start)
		r.hooks.OnComponentStop(svc.Name(), stopDuration, stopErr)

		if stopErr != nil {
			wrappedErr := fmt.Errorf("%s: shutdown error: %w", svc.Name(), stopErr)
			if r.logger != nil {
				r.logger.Error("Service shutdown error", "error", wrappedErr)
			}
			shutdownErrs = append(shutdownErrs, wrappedErr)
		}

		cancel()

		elapsed := time.Since(start)
		remainingTimeout -= elapsed
		if remainingTimeout < 0 {
			remainingTimeout = 0
		}
	}
	return shutdownErrs
}
