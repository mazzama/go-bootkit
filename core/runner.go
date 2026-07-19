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

	"github.com/mazzama/go-bootkit/core/healthkit"
	"golang.org/x/sync/errgroup"
)

type ApplicationRunner struct {
	logger           *slog.Logger
	services         []Component
	healthAggregator *healthkit.Aggregator
	startDeadline    time.Duration
	shutdownTimeout  time.Duration
	mu               sync.Mutex
}

type Option func(*ApplicationRunner)

func NewApplicationRunner(options ...Option) *ApplicationRunner {
	runner := &ApplicationRunner{
		shutdownTimeout: 15 * time.Second, // Default shutdown timeout
	}

	for _, option := range options {
		option(runner)
	}

	return runner
}

func WithLogger(l *slog.Logger) Option {
	return func(a *ApplicationRunner) {
		a.logger = l
	}
}

func WithShutdownTimeout(d time.Duration) Option {
	return func(a *ApplicationRunner) {
		if d > 0 {
			a.shutdownTimeout = d
		}
	}
}

func WithStartDeadline(d time.Duration) Option {
	return func(a *ApplicationRunner) {
		if d > 0 {
			a.startDeadline = d
		}
	}
}

func WithServices(services ...Component) Option {
	return func(a *ApplicationRunner) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.services = append(a.services, services...)
	}
}

func WithHealthAggregator(agg *healthkit.Aggregator) Option {
	return func(a *ApplicationRunner) {
		a.healthAggregator = agg
	}
}

func (r *ApplicationRunner) Run(ctx context.Context) error {
	r.mu.Lock()
	if r.logger != nil {
		// Nothing to propagate
	}
	if r.healthAggregator != nil {
		for _, s := range r.services {
			if provider, ok := s.(HealthCheckProvider); ok {
				r.healthAggregator.Register(provider.HealthChecks()...)
			}
		}
	}
	r.mu.Unlock()

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

	err := eg.Wait()

	shCtx, cancel := context.WithTimeout(context.Background(), r.shutdownTimeout)
	defer cancel()

	var shutdownErrs []error
	// Shutdown sequentially in reverse registration order
	for i := len(r.services) - 1; i >= 0; i-- {
		svc := r.services[i]
		if stopErr := svc.Stop(shCtx); stopErr != nil {
			wrappedErr := fmt.Errorf("%s: shutdown error: %w", svc.Name(), stopErr)
			if r.logger != nil {
				r.logger.Error("Service shutdown error", "error", wrappedErr)
			}
			shutdownErrs = append(shutdownErrs, wrappedErr)
		}
	}

	var allErrs []error
	if err != nil {
		allErrs = append(allErrs, err)
	}
	allErrs = append(allErrs, shutdownErrs...)

	return errors.Join(allErrs...)
}
