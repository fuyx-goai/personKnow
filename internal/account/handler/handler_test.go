package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	. "knowledge-base/internal/account/entity"
	accountservice "knowledge-base/internal/account/service"
	"knowledge-base/internal/platform/httpx"
)

func TestHandlerWeChatLoginDoesNotEchoIdentitySecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newHandlerService(t)
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

type handlerAccountRepository struct {
	user User
}

func newHandlerService(t *testing.T) *accountservice.Service {
	t.Helper()
	repository := &handlerAccountRepository{user: User{ID: uuid.New(), Nickname: "测试用户", Status: UserActive}}
	tokens := accountservice.NewTokenManager([]byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	return accountservice.NewService(repository, handlerWeChatClient{}, tokens, time.Now)
}

type handlerWeChatClient struct{}

func (handlerWeChatClient) ExchangeCode(context.Context, string) (WeChatIdentity, error) {
	return WeChatIdentity{AppID: "wx-test", OpenID: "openid-test"}, nil
}

func (repository *handlerAccountRepository) FindOrCreateUser(context.Context, WeChatIdentity, Profile) (User, error) {
	return repository.user, nil
}

func (repository *handlerAccountRepository) GetUser(context.Context, uuid.UUID) (User, error) {
	return repository.user, nil
}

func (*handlerAccountRepository) CreateSession(context.Context, Session) error { return nil }

func (*handlerAccountRepository) RotateSession(context.Context, string, string, time.Time) (Session, error) {
	return Session{}, ErrSessionInvalid
}

func (*handlerAccountRepository) RevokeSession(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (*handlerAccountRepository) ListSessions(context.Context, uuid.UUID) ([]Session, error) {
	return nil, nil
}

func (*handlerAccountRepository) RevokeOtherSessions(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (*handlerAccountRepository) CreateWebTicket(context.Context, WebLoginTicket) error { return nil }

func (*handlerAccountRepository) ConfirmWebTicket(context.Context, uuid.UUID, string, uuid.UUID, time.Time) error {
	return nil
}

func (*handlerAccountRepository) ConsumeWebTicket(context.Context, uuid.UUID, string, Session, time.Time) (User, error) {
	return User{}, ErrTicketInvalid
}

func (*handlerAccountRepository) WebTicketStatus(context.Context, uuid.UUID, string, time.Time) (TicketStatus, error) {
	return TicketPending, nil
}
