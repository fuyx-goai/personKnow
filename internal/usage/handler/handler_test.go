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

	"knowledge-base/internal/platform/httpx"
	. "knowledge-base/internal/usage/entity"
	usageservice "knowledge-base/internal/usage/service"
)

type fixedVerifier struct {
	principal httpx.Principal
}

func (verifier fixedVerifier) VerifyAccess(string) (httpx.Principal, error) {
	return verifier.principal, nil
}

func TestHandlerReturnsAuthenticatedUsersUsageOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	repository := newHandlerRepository(Limits{StorageBytes: 100, MonthlyTokens: 200})
	service := usageservice.NewService(repository, nil)
	if err := service.ApplyStorage(t.Context(), userID, uuid.New(), 25); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.Auth(fixedVerifier{principal: httpx.Principal{UserID: userID}}))
	router.GET("/usage/summary", handler.Summary)

	request := httptest.NewRequest(http.MethodGet, "/usage/summary", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"storage_bytes":25`) || !strings.Contains(body, `"storage_quota_bytes":100`) {
		t.Fatalf("unexpected summary response: %s", body)
	}
}

func TestHandlerRejectsUnauthenticatedAuditQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(usageservice.NewService(newHandlerRepository(Limits{}), nil))
	router := gin.New()
	router.Use(httpx.RequestID())
	router.GET("/audit-logs", handler.AuditLogs)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/audit-logs", nil))

	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), `"code":"AUTH_REQUIRED"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

type handlerRepository struct {
	limits Limits
	usage  MonthlyUsage
}

func newHandlerRepository(limits Limits) *handlerRepository {
	return &handlerRepository{limits: limits}
}

func (repository *handlerRepository) UpdateMonthly(_ context.Context, _ uuid.UUID, _ time.Time, mutate MonthlyMutation) error {
	next, _, err := mutate(repository.usage, repository.limits)
	repository.usage = next
	return err
}

func (repository *handlerRepository) Summary(context.Context, uuid.UUID, time.Time) (MonthlyUsage, Limits, error) {
	return repository.usage, repository.limits, nil
}

func (*handlerRepository) ListRecords(context.Context, uuid.UUID, RecordFilter) ([]Record, error) {
	return nil, nil
}

func (*handlerRepository) AppendAudit(context.Context, AuditEntry) error { return nil }

func (*handlerRepository) ListAudit(context.Context, uuid.UUID, AuditFilter) ([]AuditEntry, error) {
	return nil, nil
}
