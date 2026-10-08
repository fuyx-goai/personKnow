package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type stubVerifier struct {
	principal Principal
	err       error
}

func (verifier stubVerifier) VerifyAccess(string) (Principal, error) {
	return verifier.principal, verifier.err
}

func TestAuthMiddlewareExposesPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	want := Principal{UserID: uuid.New(), SessionID: uuid.New()}
	router := gin.New()
	router.Use(RequestID(), Auth(stubVerifier{principal: want}))
	router.GET("/", func(context *gin.Context) {
		if got, ok := PrincipalFrom(context); !ok || got != want {
			t.Fatalf("unexpected principal: %#v %v", got, ok)
		}
		context.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer valid")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
}

func TestAuthMiddlewareRejectsInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), Auth(stubVerifier{err: errors.New("invalid")}))
	router.GET("/", func(context *gin.Context) { context.Status(http.StatusNoContent) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
}
