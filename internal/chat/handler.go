package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"knowledge-base/internal/platform/httpx"
	usage "knowledge-base/internal/usage"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) CreateSession(context *gin.Context) {
	principal, ok := chatPrincipal(context)
	if !ok {
		return
	}
	var request CreateSessionCommand
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_CHAT_SCOPE", "问答范围无效"))
		return
	}
	session, err := handler.service.CreateSession(context, principal.UserID, request)
	if err != nil {
		writeChatError(context, err)
		return
	}
	context.JSON(http.StatusCreated, session)
}

func (handler *Handler) ListSessions(context *gin.Context) {
	principal, ok := chatPrincipal(context)
	if !ok {
		return
	}
	filter, err := sessionFilter(context)
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_CURSOR", "分页游标无效"))
		return
	}
	sessions, err := handler.service.ListSessions(context, principal.UserID, filter)
	if err != nil {
		writeChatError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func (handler *Handler) History(context *gin.Context) {
	principal, sessionID, ok := chatContext(context)
	if !ok {
		return
	}
	filter, err := sessionFilter(context)
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_CURSOR", "分页游标无效"))
		return
	}
	session, messages, err := handler.service.History(context, principal.UserID, sessionID, filter)
	if err != nil {
		writeChatError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"session": session, "messages": messages})
}

func (handler *Handler) DeleteSession(context *gin.Context) {
	principal, sessionID, ok := chatContext(context)
	if !ok {
		return
	}
	if err := handler.service.DeleteSession(context, principal.UserID, sessionID); err != nil {
		writeChatError(context, err)
		return
	}
	context.Status(http.StatusNoContent)
}

func (handler *Handler) Stream(context *gin.Context) {
	principal, sessionID, ok := chatContext(context)
	if !ok {
		return
	}
	var request struct {
		Question string `json:"question" binding:"required"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(context, httpx.BadRequest("QUESTION_REQUIRED", "问题不能为空"))
		return
	}
	started := false
	emit := func(event Event) error {
		if !started {
			startSSE(context)
			started = true
		}
		if event.Type == EventError {
			if data, ok := event.Data.(map[string]any); ok {
				data["request_id"] = httpx.RequestIDFrom(context)
			}
		}
		return writeSSE(context, event)
	}
	err := handler.service.Stream(context.Request.Context(), principal.UserID, sessionID, request.Question, emit)
	if err != nil && !started {
		writeChatError(context, err)
	}
}

func startSSE(context *gin.Context) {
	context.Header("Content-Type", "text/event-stream")
	context.Header("Cache-Control", "no-cache")
	context.Header("Connection", "keep-alive")
	context.Status(http.StatusOK)
}

func writeSSE(context *gin.Context, event Event) error {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(context.Writer, "event: %s\ndata: %s\n\n", event.Type, data); err != nil {
		return err
	}
	context.Writer.Flush()
	return nil
}

func chatPrincipal(context *gin.Context) (httpx.Principal, bool) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		httpx.WriteError(context, &httpx.Error{Status: http.StatusUnauthorized, Code: "AUTH_REQUIRED", Message: "请先登录"})
	}
	return principal, ok
}

func chatContext(context *gin.Context) (httpx.Principal, uuid.UUID, bool) {
	principal, ok := chatPrincipal(context)
	if !ok {
		return httpx.Principal{}, uuid.Nil, false
	}
	sessionID, err := uuid.Parse(context.Param("id"))
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_SESSION_ID", "会话 ID 无效"))
		return httpx.Principal{}, uuid.Nil, false
	}
	return principal, sessionID, true
}

func sessionFilter(context *gin.Context) (SessionFilter, error) {
	limit, _ := strconv.Atoi(context.Query("limit"))
	filter := SessionFilter{Limit: limit}
	if raw := context.Query("before"); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return SessionFilter{}, err
		}
		filter.Before = &parsed
	}
	return filter, nil
}

func writeChatError(context *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrSessionNotFound):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusNotFound, Code: "CHAT_SESSION_NOT_FOUND", Message: err.Error()})
	case errors.Is(err, ErrInvalidScope):
		httpx.WriteError(context, httpx.BadRequest("INVALID_CHAT_SCOPE", err.Error()))
	case errors.Is(err, usage.ErrTokenQuotaExceeded):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusPaymentRequired, Code: "TOKEN_QUOTA_EXCEEDED", Message: err.Error()})
	default:
		httpx.WriteError(context, err)
	}
}
