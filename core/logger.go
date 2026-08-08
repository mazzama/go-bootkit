package core

import (
	"io"
	"log/slog"
	"os"
)

type LoggerConfig struct {
	Level  slog.Level
	Writer io.Writer
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

	jsonHandler := slog.NewJSONHandler(config.Writer, &slog.HandlerOptions{
		Level: config.Level,
	})

	traceHandler := NewTraceHandler(jsonHandler)

	return slog.New(traceHandler)
}
