package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	. "knowledge-base/internal/account/entity"
	. "knowledge-base/internal/account/service"
	"knowledge-base/internal/platform/httpx"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) WeChatLogin(context *gin.Context) {
	var request struct {
		Code        string `json:"code" binding:"required"`
		Nickname    string `json:"nickname"`
		AvatarURL   string `json:"avatar_url"`
		DeviceLabel string `json:"device_label"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_REQUEST", "请提供微信登录 code"))
		return
	}
	result, err := handler.service.WeChatLogin(context, LoginCommand{
		Code: request.Code, Profile: Profile{Nickname: request.Nickname, AvatarURL: request.AvatarURL},
		ClientType: ClientMiniProgram, DeviceLabel: request.DeviceLabel,
	})
	if err != nil {
		writeAccountError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}

func (handler *Handler) Refresh(context *gin.Context) {
	var request struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_REQUEST", "refresh_token 不能为空"))
		return
	}
	result, err := handler.service.Refresh(context, request.RefreshToken)
	if err != nil {
		writeAccountError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}

func (handler *Handler) Logout(context *gin.Context) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		writeAccountError(context, ErrSessionInvalid)
		return
	}
	if err := handler.service.RevokeSession(context, principal.UserID, principal.SessionID); err != nil {
		writeAccountError(context, err)
		return
	}
	context.Status(http.StatusNoContent)
}

func (handler *Handler) CreateWebTicket(context *gin.Context) {
	result, err := handler.service.CreateWebTicket(context)
	if err != nil {
		writeAccountError(context, err)
		return
	}
	context.JSON(http.StatusCreated, result)
}

func (handler *Handler) PollWebTicket(context *gin.Context) {
	ticketID, err := uuid.Parse(context.Param("id"))
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_TICKET", "登录票据格式无效"))
		return
	}
	secret := context.GetHeader("X-Login-Ticket-Secret")
	result, err := handler.service.PollWebTicket(context, ticketID, secret, context.GetHeader("X-Device-Label"))
	if err != nil {
		writeAccountError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}

func (handler *Handler) ConfirmWebTicket(context *gin.Context) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		writeAccountError(context, ErrSessionInvalid)
		return
	}
	ticketID, err := uuid.Parse(context.Param("id"))
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_TICKET", "登录票据格式无效"))
		return
	}
	var request struct {
		Secret string `json:"secret" binding:"required"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_TICKET", "登录票据密钥不能为空"))
		return
	}
	if err := handler.service.ConfirmWebTicket(context, principal.UserID, ticketID, request.Secret); err != nil {
		writeAccountError(context, err)
		return
	}
	context.Status(http.StatusNoContent)
}

func (handler *Handler) Me(context *gin.Context) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		writeAccountError(context, ErrSessionInvalid)
		return
	}
	user, err := handler.service.CurrentUser(context, principal.UserID)
	if err != nil {
		writeAccountError(context, err)
		return
	}
	context.JSON(http.StatusOK, user)
}

func (handler *Handler) Sessions(context *gin.Context) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		writeAccountError(context, ErrSessionInvalid)
		return
	}
	sessions, err := handler.service.ListSessions(context, principal.UserID)
	if err != nil {
		writeAccountError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func (handler *Handler) RevokeSession(context *gin.Context) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		writeAccountError(context, ErrSessionInvalid)
		return
	}
	sessionID, err := uuid.Parse(context.Param("id"))
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_SESSION", "会话 ID 无效"))
		return
	}
	if err := handler.service.RevokeSession(context, principal.UserID, sessionID); err != nil {
		writeAccountError(context, err)
		return
	}
	context.Status(http.StatusNoContent)
}

func (handler *Handler) RevokeOtherSessions(context *gin.Context) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		writeAccountError(context, ErrSessionInvalid)
		return
	}
	if err := handler.service.RevokeOtherSessions(context, principal.UserID, principal.SessionID); err != nil {
		writeAccountError(context, err)
		return
	}
	context.Status(http.StatusNoContent)
}

func writeAccountError(context *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrWeChatLogin):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusUnauthorized, Code: "WECHAT_LOGIN_FAILED", Message: "微信登录失败，请重试"})
	case errors.Is(err, ErrSessionInvalid):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusUnauthorized, Code: "SESSION_INVALID", Message: "登录已失效，请重新登录"})
	case errors.Is(err, ErrTicketInvalid):
		httpx.WriteError(context, httpx.BadRequest("TICKET_INVALID", "二维码已过期或已使用"))
	case errors.Is(err, ErrUserDisabled):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusForbidden, Code: "USER_DISABLED", Message: "账号已停用"})
	default:
		httpx.WriteError(context, err)
	}
}
