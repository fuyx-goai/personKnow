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

// sensitiveKeys 同时覆盖精确字段名和 *_token、*_secret 等常见派生字段。
// 集中脱敏可以避免各业务模块遗漏敏感信息处理。
var sensitiveKeys = map[string]struct{}{
	"api_key": {}, "authorization": {}, "jwt": {}, "jwt_secret": {},
	"password": {}, "refresh_token": {}, "secret": {}, "token": {},
}

// New 创建统一的 JSON Logger，便于本地检索和线上日志平台采集。
func New(writer io.Writer, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redactAttribute,
	})
	return slog.New(handler)
}

// NewFile 创建带大小滚动、过期清理和压缩能力的应用日志文件。
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

// redactAttribute 在日志写出前替换敏感字段，避免令牌和密钥落盘。
func redactAttribute(_ []string, attribute slog.Attr) slog.Attr {
	key := strings.ToLower(attribute.Key)
	for sensitive := range sensitiveKeys {
		if key == sensitive || strings.HasSuffix(key, "_"+sensitive) {
			return slog.String(attribute.Key, redactedValue)
		}
	}
	return attribute
}
