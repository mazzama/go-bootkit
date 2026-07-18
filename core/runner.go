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

type ApplicationRunner struct {
	logger          *slog.Logger
	services        []Component
	startDeadline   time.Duration
	shutdownTimeout time.Duration
	mu              sync.Mutex
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

func (r *ApplicationRunner) Run(ctx context.Context) error {
	if r.logger != nil {
		r.mu.Lock()
		for _, s := range r.services {
			if loggable, ok := s.(Loggable); ok {
				loggable.SetLogger(r.logger)
			}
		}
		r.mu.Unlock()
	}

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

	// Channel to collect shutdown errors
	errCh := make(chan error, len(r.services))
	for _, s := range r.services {
		svc := s
		go func() {
			defer wg.Done()
			if stopErr := svc.Stop(shCtx); stopErr != nil {
				errCh <- fmt.Errorf("%s: shutdown error: %w", svc.Name(), stopErr)
			}
		}()
	}

	// Wait for all shutdowns to complete
	wg.Wait()
	close(errCh)

	// Log any shutdown errors
	if r.logger != nil {
		for err := range errCh {
			r.logger.Error("Service shutdown error", "error", err)
		}
	}

	return err
}
