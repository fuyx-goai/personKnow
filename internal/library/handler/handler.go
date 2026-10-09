package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	. "knowledge-base/internal/library/entity"
	. "knowledge-base/internal/library/service"
	"knowledge-base/internal/platform/httpx"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) List(context *gin.Context) {
	principal, ok := requirePrincipal(context)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(context.Query("limit"))
	result, err := handler.service.List(context, principal.UserID, ListFilter{
		Keyword: context.Query("keyword"), Category: context.Query("category"), Limit: limit, Cursor: context.Query("cursor"),
	})
	if err != nil {
		writeLibraryError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}

func (handler *Handler) Create(context *gin.Context) {
	principal, ok := requirePrincipal(context)
	if !ok {
		return
	}
	var request struct {
		Name        string     `json:"name" binding:"required"`
		Category    string     `json:"category"`
		Description string     `json:"description"`
		Visibility  Visibility `json:"visibility"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_LIBRARY", "知识库名称不能为空"))
		return
	}
	library, err := handler.service.Create(context, principal.UserID, CreateCommand{
		Name: request.Name, Category: request.Category, Description: request.Description, Visibility: request.Visibility,
	})
	if err != nil {
		writeLibraryError(context, err)
		return
	}
	context.JSON(http.StatusCreated, library)
}

func (handler *Handler) Get(context *gin.Context) {
	principal, libraryID, ok := handlerContext(context)
	if !ok {
		return
	}
	library, err := handler.service.Get(context, principal.UserID, libraryID)
	if err != nil {
		writeLibraryError(context, err)
		return
	}
	context.JSON(http.StatusOK, library)
}

func (handler *Handler) Update(context *gin.Context) {
	principal, libraryID, ok := handlerContext(context)
	if !ok {
		return
	}
	var request UpdateCommand
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_LIBRARY", "请求内容无效"))
		return
	}
	if err := handler.service.Update(context, principal.UserID, libraryID, request); err != nil {
		writeLibraryError(context, err)
		return
	}
	context.Status(http.StatusNoContent)
}

func (handler *Handler) Delete(context *gin.Context) {
	principal, libraryID, ok := handlerContext(context)
	if !ok {
		return
	}
	if err := handler.service.Delete(context, principal.UserID, libraryID); err != nil {
		writeLibraryError(context, err)
		return
	}
	context.Status(http.StatusAccepted)
}

func (handler *Handler) Settings(context *gin.Context) {
	principal, libraryID, ok := handlerContext(context)
	if !ok {
		return
	}
	settings, err := handler.service.Settings(context, principal.UserID, libraryID)
	if err != nil {
		writeLibraryError(context, err)
		return
	}
	context.JSON(http.StatusOK, settings)
}

func (handler *Handler) UpdateSettings(context *gin.Context) {
	principal, libraryID, ok := handlerContext(context)
	if !ok {
		return
	}
	var request RetrievalSettings
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_RETRIEVAL_SETTINGS", "检索参数格式无效"))
		return
	}
	result, err := handler.service.UpdateSettings(context, principal.UserID, libraryID, request)
	if err != nil {
		writeLibraryError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}

func (handler *Handler) Reindex(context *gin.Context) {
	principal, libraryID, ok := handlerContext(context)
	if !ok {
		return
	}
	jobs, err := handler.service.Reindex(context, principal.UserID, libraryID)
	if err != nil {
		writeLibraryError(context, err)
		return
	}
	context.JSON(http.StatusAccepted, gin.H{"job_ids": jobs})
}

func handlerContext(context *gin.Context) (httpx.Principal, uuid.UUID, bool) {
	principal, ok := requirePrincipal(context)
	if !ok {
		return httpx.Principal{}, uuid.Nil, false
	}
	libraryID, err := uuid.Parse(context.Param("id"))
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_LIBRARY_ID", "知识库 ID 无效"))
		return httpx.Principal{}, uuid.Nil, false
	}
	return principal, libraryID, true
}

func requirePrincipal(context *gin.Context) (httpx.Principal, bool) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		httpx.WriteError(context, &httpx.Error{Status: http.StatusUnauthorized, Code: "AUTH_REQUIRED", Message: "请先登录"})
	}
	return principal, ok
}

func writeLibraryError(context *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusNotFound, Code: "LIBRARY_NOT_FOUND", Message: "知识库不存在"})
	case errors.Is(err, ErrForbidden):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusForbidden, Code: "LIBRARY_READ_ONLY", Message: "公开知识库仅支持只读访问"})
	case errors.Is(err, ErrInvalidLibrary):
		httpx.WriteError(context, httpx.BadRequest("INVALID_LIBRARY", err.Error()))
	case errors.Is(err, ErrInvalidSettings):
		httpx.WriteError(context, httpx.BadRequest("INVALID_RETRIEVAL_SETTINGS", err.Error()))
	case errors.Is(err, ErrReindexUnavailable):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusServiceUnavailable, Code: "REINDEX_UNAVAILABLE", Message: err.Error()})
	default:
		httpx.WriteError(context, err)
	}
}
