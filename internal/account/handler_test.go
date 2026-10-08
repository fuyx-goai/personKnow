package account

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"knowledge-base/internal/platform/httpx"
)

func TestHandlerWeChatLoginDoesNotEchoIdentitySecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newAccountServiceForTest(t)
	handler := NewHandler(service)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.POST("/login", handler.WeChatLogin)

	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"code":"one","nickname":"用户"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if strings.Contains(body, "openid-one") || strings.Contains(body, `"code"`) {
		t.Fatalf("identity secret leaked in response: %s", body)
	}
	if !strings.Contains(body, "access_token") || !strings.Contains(body, "refresh_token") {
		t.Fatalf("tokens missing from response: %s", body)
	}
}
