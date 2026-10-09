package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	. "knowledge-base/internal/indexing/entity"
	"knowledge-base/internal/platform/httpx"
)

type jobManager struct {
	job     Job
	retried uuid.UUID
}

func (manager *jobManager) Get(context.Context, uuid.UUID, uuid.UUID) (Job, error) {
	return manager.job, nil
}

func (manager *jobManager) Retry(_ context.Context, _ uuid.UUID, jobID uuid.UUID) error {
	manager.retried = jobID
	return nil
}

type indexingVerifier struct {
	userID uuid.UUID
}

func (verifier indexingVerifier) VerifyAccess(string) (httpx.Principal, error) {
	return httpx.Principal{UserID: verifier.userID}, nil
}

func TestHandlerGetsAndRetriesOwnedJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID, jobID := uuid.New(), uuid.New()
	manager := &jobManager{job: Job{ID: jobID, Status: StatusFailed}}
	handler := NewHandler(manager)
	router := gin.New()
	router.Use(httpx.Auth(indexingVerifier{userID: userID}))
	router.GET("/jobs/:id", handler.Get)
	router.POST("/jobs/:id/retry", handler.Retry)

	for _, request := range []*http.Request{
		httpRequest(t, http.MethodGet, "/jobs/"+jobID.String()),
		httpRequest(t, http.MethodPost, "/jobs/"+jobID.String()+"/retry"),
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK && recorder.Code != http.StatusAccepted {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
	if manager.retried != jobID {
		t.Fatalf("retried=%s, want %s", manager.retried, jobID)
	}
}

func TestHandlerRejectsMalformedJobID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(&jobManager{})
	router := gin.New()
	router.Use(httpx.Auth(indexingVerifier{userID: uuid.New()}))
	router.GET("/jobs/:id", handler.Get)
	request := httpRequest(t, http.MethodGet, "/jobs/not-a-uuid")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "INVALID_JOB_ID") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func httpRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("Authorization", "Bearer token")
	return request
}
