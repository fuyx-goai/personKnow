package httpx

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(context *gin.Context) {
		startedAt := time.Now()
		context.Next()
		attributes := []any{
			"request_id", RequestIDFrom(context),
			"method", context.Request.Method,
			"route", context.FullPath(),
			"status", context.Writer.Status(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"legacy_api", strings.HasPrefix(context.Request.URL.Path, "/api/") && !strings.HasPrefix(context.Request.URL.Path, "/api/v1/"),
		}
		if principal, ok := PrincipalFrom(context); ok {
			attributes = append(attributes, "user_id", principal.UserID.String())
		}
		logger.InfoContext(context.Request.Context(), "http_request", attributes...)
	}
}
