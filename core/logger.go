package core

import (
	"io"
	"log/slog"
	"os"
)

type LoggerConfig struct {
	Level       slog.Level
	Writer      io.Writer
	Handler     slog.Handler
	ServiceName string
	Version     string
	Environment string
}

type LoggerOption func(*LoggerConfig)

// WithLogLevel sets the minimum log level for the logger.
func WithLogLevel(level slog.Level) LoggerOption {
	return func(c *LoggerConfig) {
		c.Level = level
	}
}

// WithLogWriter sets the output writer for the logger.
func WithLogWriter(writer io.Writer) LoggerOption {
	return func(c *LoggerConfig) {
		c.Writer = writer
	}
}

func WithServiceName(name string) LoggerOption {
	return func(c *LoggerConfig) { c.ServiceName = name }
}

func WithVersion(v string) LoggerOption {
	return func(c *LoggerConfig) { c.Version = v }
}

func WithEnvironment(e string) LoggerOption {
	return func(c *LoggerConfig) { c.Environment = e }
}

func WithHandler(h slog.Handler) LoggerOption {
	return func(c *LoggerConfig) { c.Handler = h }
}

// NewLogger creates a new *slog.Logger configured with JSON output
// and trace context correlation.
func NewLogger(options ...LoggerOption) *slog.Logger {
	config := &LoggerConfig{
		Level:  slog.LevelInfo,
		Writer: os.Stdout,
	}

	for _, opt := range options {
		opt(config)
	}

	var handler = config.Handler
	if handler == nil {
		handler = slog.NewJSONHandler(config.Writer, &slog.HandlerOptions{
			Level: config.Level,
		})
	}

	var finalHandler slog.Handler = NewTraceHandler(handler)

	var attrs []slog.Attr
	if config.ServiceName != "" {
		attrs = append(attrs, slog.String("service.name", config.ServiceName))
	}
	if config.Version != "" {
		attrs = append(attrs, slog.String("service.version", config.Version))
	}
	if config.Environment != "" {
		attrs = append(attrs, slog.String("deployment.environment", config.Environment))
	}

	if len(attrs) > 0 {
		finalHandler = finalHandler.WithAttrs(attrs)
	}

	return slog.New(finalHandler)
}
