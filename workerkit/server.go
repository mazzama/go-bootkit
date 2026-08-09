package workerkit

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

type ServerConfig struct {
	asynq.Config
}

type ServerOption func(*ServerConfig)

func WithConcurrency(n int) ServerOption {
	return func(c *ServerConfig) {
		c.Concurrency = n
	}
}

func WithAsynqConfig(cfg asynq.Config) ServerOption {
	return func(c *ServerConfig) {
		c.Config = cfg
	}
}

type AsynqServer struct {
	core.Lifecycle
	name   string
	server *asynq.Server
	mux    *asynq.ServeMux
	logger *slog.Logger
}

func NewAsynqServer(name string, redisOpt asynq.RedisConnOpt, opts ...ServerOption) *AsynqServer {
	cfg := &ServerConfig{}
	cfg.Concurrency = 10 // default
	for _, opt := range opts {
		opt(cfg)
	}

	server := asynq.NewServer(redisOpt, cfg.Config)
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

func (s *AsynqServer) Mux() *asynq.ServeMux {
	return s.mux
}

func (s *AsynqServer) Name() string {
	return s.name
}

func (s *AsynqServer) HealthChecks() []healthkit.Check {
	return asynqHealthChecks(s.name, s.Ready())
}

var _ core.Component = (*AsynqServer)(nil)
