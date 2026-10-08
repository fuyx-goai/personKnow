package library

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"knowledge-base/internal/platform/httpx"
)

type principalMiddleware struct {
	principal httpx.Principal
}

func (middleware principalMiddleware) handle(context *gin.Context) {
	context.Set("principal", middleware.principal)
	context.Next()
}

func TestHandlerCreatesPrivateLibraryByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	repository := &fakeRepository{}
	handler := NewHandler(NewService(repository))
	router := gin.New()
	router.Use(httpx.RequestID(), principalMiddleware{principal: httpx.Principal{UserID: userID}}.handle)
	router.POST("/libraries", handler.Create)

	request := httptest.NewRequest(http.MethodPost, "/libraries", strings.NewReader(`{"name":"AI 架构","category":"技术工程"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	if repository.library.OwnerUserID != userID || repository.library.Visibility != VisibilityPrivate {
		t.Fatalf("unexpected library: %#v", repository.library)
	}
}
