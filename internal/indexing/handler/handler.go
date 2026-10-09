package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	. "knowledge-base/internal/indexing/entity"
	"knowledge-base/internal/platform/httpx"
)

type JobManager interface {
	Get(context.Context, uuid.UUID, uuid.UUID) (Job, error)
	Retry(context.Context, uuid.UUID, uuid.UUID) error
}

type Handler struct {
	jobs JobManager
}

func NewHandler(jobs JobManager) *Handler {
	return &Handler{jobs: jobs}
}

func (handler *Handler) Get(context *gin.Context) {
	principal, jobID, ok := jobContext(context)
	if !ok {
		return
	}
	job, err := handler.jobs.Get(context, principal.UserID, jobID)
	if err != nil {
		writeJobError(context, err)
		return
	}
	context.JSON(http.StatusOK, job)
}

func (handler *Handler) Retry(context *gin.Context) {
	principal, jobID, ok := jobContext(context)
	if !ok {
		return
	}
	if err := handler.jobs.Retry(context, principal.UserID, jobID); err != nil {
		writeJobError(context, err)
		return
	}
	context.JSON(http.StatusAccepted, gin.H{"job_id": jobID, "status": StatusQueued})
}

func jobContext(context *gin.Context) (httpx.Principal, uuid.UUID, bool) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		httpx.WriteError(context, &httpx.Error{Status: http.StatusUnauthorized, Code: "AUTH_REQUIRED", Message: "请先登录"})
		return httpx.Principal{}, uuid.Nil, false
	}
	jobID, err := uuid.Parse(context.Param("id"))
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_JOB_ID", "任务 ID 无效"))
		return httpx.Principal{}, uuid.Nil, false
	}
	return principal, jobID, true
}

func writeJobError(context *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrJobNotFound):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusNotFound, Code: "INDEX_JOB_NOT_FOUND", Message: err.Error()})
	case errors.Is(err, ErrJobNotRetryable):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusConflict, Code: "INDEX_JOB_NOT_RETRYABLE", Message: err.Error()})
	default:
		httpx.WriteError(context, err)
	}
}
