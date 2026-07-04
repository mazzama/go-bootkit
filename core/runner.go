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
)

// ApplicationRunner orchestrates the lifecycle of multiple components.
//
// The ApplicationRunner manages concurrent component startup using an errgroup,
// enforces optional start deadlines via Readyable channels, and handles graceful
// shutdown on SIGINT/SIGTERM signals.
//
// # Startup Process
//
//  1. All components are started concurrently via their Start() methods.
//  2. If a start deadline is configured and a component implements Readyable,
//     the runner waits for the component's Ready() channel to close.
//  3. If any component returns an error (other than context.Canceled), all
//     components are shut down.
//
// # Shutdown Process
//
//  1. The ApplicationRunner's Run() method returns when a component fails or
//     a termination signal is received.
//  2. All components' Stop() methods are called concurrently with a timeout.
//  3. The runner waits for all components to stop or the timeout to expire.
//
// Example:
//
//	runner := NewApplicationRunner(
//	    WithServices(server, database, cache),
//	    WithShutdownTimeout(30*time.Second),
//	    WithStartDeadline(10*time.Second),
//	)
//	if err := runner.Run(ctx); err != nil {
//	    log.Fatal(err)
//	}
type ApplicationRunner struct {
	logger          *slog.Logger
	mu              sync.Mutex
	startDeadline   time.Duration
	shutdownTimeout time.Duration
	services        []Component
}

// Option is a functional option for configuring an ApplicationRunner.
//
// Options are applied in the order they are provided to NewApplicationRunner.
// See the With* functions for available options.
type Option func(*ApplicationRunner)

// NewApplicationRunner creates a new ApplicationRunner with the provided options.
//
// The ApplicationRunner is configured using functional options. The default
// shutdown timeout is 15 seconds if not specified via WithShutdownTimeout.
//
// Example:
//
//	runner := NewApplicationRunner(
//	    WithLogger(logger),
//	    WithShutdownTimeout(30*time.Second),
//	    WithStartDeadline(10*time.Second),
//	)
func NewApplicationRunner(options ...Option) *ApplicationRunner {
	runner := &ApplicationRunner{
		shutdownTimeout: 15 * time.Second, // Default shutdown timeout
	}

	for _, option := range options {
		option(runner)
	}

	return runner
}

// WithLogger sets the logger for the ApplicationRunner.
//
// The logger is used to log information about component startup, shutdown,
// and errors. If not provided, no logging will occur.
//
// Example:
//
//	logger := slog.Default()
//	runner := NewApplicationRunner(WithLogger(logger))
func WithLogger(l *slog.Logger) Option {
	return func(a *ApplicationRunner) {
		a.logger = l
	}
}

// WithShutdownTimeout sets the maximum time to wait for components to stop.
//
// During shutdown, each component's Stop() method is called concurrently.
// If the timeout expires before all components have stopped, the runner
// returns without waiting for the remaining components.
//
// If not provided, the default timeout is 15 seconds.
//
// Example:
//
//	runner := NewApplicationRunner(
//	    WithShutdownTimeout(30*time.Second),
//	)
func WithShutdownTimeout(d time.Duration) Option {
	return func(a *ApplicationRunner) {
		if d > 0 {
			a.shutdownTimeout = d
		}
	}
}

// WithStartDeadline sets the maximum time to wait for components to be ready.
//
// If a component implements the Readyable interface, the runner waits for
// its Ready() channel to close. If the deadline expires before a component
// is ready, that component's context is canceled and the runner shuts down
// with an error.
//
// This is useful for ensuring that components don't hang indefinitely during
// startup, particularly in production environments where health checks may
// fail the application if it doesn't start within a certain time.
//
// Example:
//
//	runner := NewApplicationRunner(
//	    WithStartDeadline(10*time.Second),
//	)
func WithStartDeadline(d time.Duration) Option {
	return func(a *ApplicationRunner) {
		if d > 0 {
			a.startDeadline = d
		}
	}
}

// WithServices adds components to the ApplicationRunner.
//
// Components are started in the order they are added, but concurrently.
// Multiple WithServices options can be provided, and each can contain
// multiple components.
//
// Example:
//
//	runner := NewApplicationRunner(
//	    WithServices(server, database),
//	    WithServices(cache),
//	)
func WithServices(services ...Component) Option {
	return func(a *ApplicationRunner) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.services = append(a.services, services...)
	}
}

// Run starts all components and waits for termination or error.
//
// Run blocks until one of the following occurs:
//   - A component returns a non-nil error (other than context.Canceled)
//   - A termination signal is received (SIGINT or SIGTERM)
//   - A start deadline is exceeded (if configured)
//
// Before returning, Run initiates graceful shutdown by calling Stop() on all
// components concurrently, respecting the configured shutdown timeout.
//
// The provided context can be used to manually trigger shutdown by canceling it.
// If the context is canceled, all components will be shut down gracefully.
//
// Returns an error if:
//   - A component failed during startup (wraps the component error)
//   - A component exceeded its start deadline
//   - The context was already canceled at invocation
//
// Example:
//
//	ctx := context.Background()
//	err := runner.Run(ctx)
//	if err != nil {
//	    log.Printf("Application shut down: %v", err)
//	}
func (r *ApplicationRunner) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	eg, ctx := errgroup.WithContext(ctx)

	for _, s := range r.services {
		svc := s
		svcCtx, cancelSvc := context.WithCancel(ctx)

		// Start loop
		eg.Go(func() error {
			defer cancelSvc()
			if err := svc.Start(svcCtx); err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("%s: %w", svc.Name(), err)
			}
			return nil
		})

		if r.startDeadline > 0 {
			if rr, ok := svc.(Readyable); ok && rr.Ready() != nil {
				readyCh := rr.Ready()
				eg.Go(func() error {
					select {
					case <-readyCh:
						return nil
					case <-time.After(r.startDeadline):
						cancelSvc()
						return fmt.Errorf("%s: start deadline exceeded (%s)", svc.Name(), r.startDeadline)
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			}
		}
	}

	err := eg.Wait()

	shCtx, cancel := context.WithTimeout(context.Background(), r.shutdownTimeout)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(len(r.services))
	for _, s := range r.services {
		svc := s
		go func() {
			defer wg.Done()
			if err := svc.Stop(shCtx); err != nil && r.logger != nil {
				r.logger.Error("component stop failed", "name", svc.Name(), "error", err)
			}
		}()
	}
	wg.Wait()
	return err
}
