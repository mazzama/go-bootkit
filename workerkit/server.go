package workerkit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

// AsynqServer is a lifecycle-managed asynq worker server.
type AsynqServer struct {
	core.Lifecycle
	name   string
	server *asynq.Server
	mux    *asynq.ServeMux
	logger *slog.Logger
}

var _ core.Component = (*AsynqServer)(nil)

// NewAsynqServer creates an AsynqServer from a Redis config and server config.
func NewAsynqServer(name string, redisCfg RedisConfig, cfg ServerConfig) *AsynqServer {
	server := asynq.NewServer(redisCfg.toAsynqOpt(), cfg.toAsynqConfig())
	mux := asynq.NewServeMux()

	s := &AsynqServer{
		name:   name,
		server: server,
		mux:    mux,
		logger: slog.Default(),
	}

	s.Lifecycle = core.NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		if err := s.server.Start(s.mux); err != nil {
			return nil, fmt.Errorf("failed to start asynq server: %w", err)
		}

		return func(shutdownCtx context.Context) error {
			// server.Shutdown() blocks until workers finish, so we can wrap it in a goroutine
			// to respect the context timeout, but asynq.Server's Shutdown already has its own
			// internal timeouts based on StrictPriority etc. We'll simply call it in a goroutine
			// and select on context.
			done := make(chan struct{})
			go func() {
				s.server.Shutdown()
				close(done)
			}()

			select {
			case <-done:
				return nil
			case <-shutdownCtx.Done():
				// Stop forces shutdown
				s.server.Stop()
				return fmt.Errorf("asynq server shutdown timed out: %w", shutdownCtx.Err())
			}
		}, nil
	})

	return s
}

func (sc ServerConfig) toAsynqConfig() asynq.Config {
	cfg := asynq.Config{
		Concurrency:    sc.Concurrency,
		Queues:         sc.Queues,
		StrictPriority: sc.StrictPriority,
	}
	if sc.ShutdownTimeout > 0 {
		cfg.ShutdownTimeout = sc.ShutdownTimeout
	}
	if sc.RetryDelayFunc != nil {
		cfg.RetryDelayFunc = func(n int, err error, t *asynq.Task) time.Duration {
			return sc.RetryDelayFunc(n, err, Task{Type: t.Type(), Payload: t.Payload()})
		}
	}
	return cfg
}

// Mux returns the internal asynq serve mux used for handler registration.
func (s *AsynqServer) Mux() *asynq.ServeMux {
	return s.mux
}

// HandleFunc registers a framework-native Handler on the internal mux.
// The handler receives a workerkit.Task constructed from the asynq task's
// type and payload, so domain handlers never import asynq.
func (s *AsynqServer) HandleFunc(pattern string, handler Handler) {
	s.mux.HandleFunc(pattern, func(ctx context.Context, t *asynq.Task) error {
		return handler(ctx, Task{Type: t.Type(), Payload: t.Payload()})
	})
}

// Name returns the component name of the server.
func (s *AsynqServer) Name() string {
	return s.name
}

// HealthChecks returns the liveness/readiness checks for the server.
func (s *AsynqServer) HealthChecks() []healthkit.Check {
	return asynqHealthChecks(s.name, s.Ready())
}
