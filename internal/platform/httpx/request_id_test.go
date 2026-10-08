package httpx

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDMiddlewareCreatesAndExposesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	var contextID string
	router.GET("/", func(c *gin.Context) {
		contextID = RequestIDFrom(c)
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	headerID := recorder.Header().Get(RequestIDHeader)
	if !regexp.MustCompile(`^[a-f0-9-]{36}$`).MatchString(headerID) {
		t.Fatalf("unexpected request id %q", headerID)
	}
	if contextID != headerID {
		t.Fatalf("context id %q differs from header %q", contextID, headerID)
	}
}

func TestRequestIDMiddlewareKeepsSafeIncomingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "client-request-123")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get(RequestIDHeader); got != "client-request-123" {
		t.Fatalf("expected incoming request id, got %q", got)
	}
}
