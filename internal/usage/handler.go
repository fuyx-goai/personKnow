package usage

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"knowledge-base/internal/platform/httpx"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) Summary(context *gin.Context) {
	principal, ok := usagePrincipal(context)
	if !ok {
		return
	}
	summary, err := handler.service.Summary(context, principal.UserID, time.Now())
	if err != nil {
		writeUsageError(context, err)
		return
	}
	context.JSON(http.StatusOK, summary)
}

func (handler *Handler) Records(context *gin.Context) {
	principal, ok := usagePrincipal(context)
	if !ok {
		return
	}
	filter, err := recordFilter(context)
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_USAGE_FILTER", "用量筛选参数无效"))
		return
	}
	records, err := handler.service.ListRecords(context, principal.UserID, filter)
	if err != nil {
		writeUsageError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"records": records})
}

func (handler *Handler) AuditLogs(context *gin.Context) {
	principal, ok := usagePrincipal(context)
	if !ok {
		return
	}
	filter, err := auditFilter(context)
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_AUDIT_FILTER", "审计筛选参数无效"))
		return
	}
	entries, err := handler.service.ListAudit(context, principal.UserID, filter)
	if err != nil {
		writeUsageError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"audit_logs": entries})
}

func usagePrincipal(context *gin.Context) (httpx.Principal, bool) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		httpx.WriteError(context, &httpx.Error{Status: http.StatusUnauthorized, Code: "AUTH_REQUIRED", Message: "请先登录"})
	}
	return principal, ok
}

func recordFilter(context *gin.Context) (RecordFilter, error) {
	limit, _ := strconv.Atoi(context.Query("limit"))
	filter := RecordFilter{Type: UsageType(context.Query("type")), Limit: limit}
	if filter.Type != "" && filter.Type != UsageStorage && filter.Type != UsageEmbedding && filter.Type != UsageInput && filter.Type != UsageOutput {
		return RecordFilter{}, errors.New("invalid usage type")
	}
	before, err := optionalTime(context.Query("before"))
	filter.Before = before
	return filter, err
}

func auditFilter(context *gin.Context) (AuditFilter, error) {
	limit, _ := strconv.Atoi(context.Query("limit"))
	before, err := optionalTime(context.Query("before"))
	return AuditFilter{Action: context.Query("action"), ResourceType: context.Query("resource_type"), Before: before, Limit: limit}, err
}

func optionalTime(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func writeUsageError(context *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrTokenQuotaExceeded):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusPaymentRequired, Code: "TOKEN_QUOTA_EXCEEDED", Message: err.Error()})
	case errors.Is(err, ErrStorageQuotaExceeded):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusInsufficientStorage, Code: "STORAGE_QUOTA_EXCEEDED", Message: err.Error()})
	case errors.Is(err, ErrInvalidAmount), errors.Is(err, ErrInvalidBreakdown), errors.Is(err, ErrInvalidAuditMetadata):
		httpx.WriteError(context, httpx.BadRequest("INVALID_USAGE", err.Error()))
	default:
		httpx.WriteError(context, err)
	}
}
