package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"knowledge-base/pkg/config"
)

func TestLoggerRedactsSensitiveFields(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output, slog.LevelDebug)
	logger.Info("request",
		"request_id", "req-1",
		"authorization", "Bearer super-secret",
		"refresh_token", "refresh-secret",
	)

	logged := output.String()
	if strings.Contains(logged, "super-secret") || strings.Contains(logged, "refresh-secret") {
		t.Fatalf("sensitive value leaked: %s", logged)
	}
	if !strings.Contains(logged, "[REDACTED]") || !strings.Contains(logged, "req-1") {
		t.Fatalf("redaction or safe field missing: %s", logged)
	}
}

func TestNewFileCreatesApplicationLog(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "logs")
	logger, err := NewFile(config.LogConfig{Dir: directory, MaxSizeMB: 1, RetainDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("started", "request_id", "req-file")

	data, err := os.ReadFile(filepath.Join(directory, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "req-file") {
		t.Fatalf("log record was not written: %s", data)
	}
}
