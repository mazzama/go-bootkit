package core_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/mazzama/go-bootkit/core"
)

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := core.NewLogger(
		core.WithLogWriter(&buf),
		core.WithLogLevel(slog.LevelDebug),
	)

	if logger == nil {
		t.Fatal("expected logger, got nil")
	}

	logger.Debug("test debug")
	if !bytes.Contains(buf.Bytes(), []byte("test debug")) {
		t.Errorf("expected output to contain 'test debug', got %s", buf.String())
	}
}
