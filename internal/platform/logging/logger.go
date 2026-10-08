package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"

	"knowledge-base/pkg/config"
)

const redactedValue = "[REDACTED]"

var sensitiveKeys = map[string]struct{}{
	"api_key": {}, "authorization": {}, "jwt": {}, "jwt_secret": {},
	"password": {}, "refresh_token": {}, "secret": {}, "token": {},
}

func New(writer io.Writer, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redactAttribute,
	})
	return slog.New(handler)
}

func NewFile(cfg config.LogConfig) (*slog.Logger, error) {
	if err := os.MkdirAll(cfg.Dir, 0o750); err != nil {
		return nil, err
	}
	writer := &lumberjack.Logger{
		Filename:  filepath.Join(cfg.Dir, "app.log"),
		MaxSize:   cfg.MaxSizeMB,
		MaxAge:    cfg.RetainDays,
		Compress:  true,
		LocalTime: false,
	}
	return New(writer, slog.LevelInfo), nil
}

func redactAttribute(_ []string, attribute slog.Attr) slog.Attr {
	key := strings.ToLower(attribute.Key)
	for sensitive := range sensitiveKeys {
		if key == sensitive || strings.HasSuffix(key, "_"+sensitive) {
			return slog.String(attribute.Key, redactedValue)
		}
	}
	return attribute
}
