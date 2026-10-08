package httpx

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	platformlogging "knowledge-base/internal/platform/logging"
)

func TestAccessLogRecordsMetadataWithoutSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	logger := platformlogging.New(&output, slog.LevelInfo)
	router := gin.New()
	router.Use(RequestID(), AccessLog(logger))
	router.POST("/api/v1/chat", func(context *gin.Context) { context.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodPost, "/api/v1/chat?token=query-secret", strings.NewReader("body-secret"))
	request.Header.Set("Authorization", "Bearer header-secret")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	logLine := output.String()
	for _, secret := range []string{"query-secret", "body-secret", "header-secret"} {
		if strings.Contains(logLine, secret) {
			t.Fatalf("access log leaked %q: %s", secret, logLine)
		}
	}
	for _, field := range []string{`"request_id"`, `"route":"/api/v1/chat"`, `"status":204`} {
		if !strings.Contains(logLine, field) {
			t.Fatalf("access log missing %s: %s", field, logLine)
		}
	}
}
