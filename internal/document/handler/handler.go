package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	. "knowledge-base/internal/document/entity"
	. "knowledge-base/internal/document/service"
	librarydomain "knowledge-base/internal/library/entity"
	"knowledge-base/internal/platform/httpx"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) Upload(context *gin.Context) {
	principal, ok := documentPrincipal(context)
	if !ok {
		return
	}
	libraryID, err := uuid.Parse(context.PostForm("library_id"))
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_LIBRARY_ID", "请选择知识库"))
		return
	}
	header, err := context.FormFile("file")
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("FILE_REQUIRED", "请选择要上传的文件"))
		return
	}
	file, err := header.Open()
	if err != nil {
		writeDocumentError(context, err)
		return
	}
	defer file.Close()
	result, err := handler.service.Upload(context, principal.UserID, UploadCommand{
		LibraryID: libraryID, OriginalName: header.Filename, Reader: file, Tags: context.PostFormArray("tags"),
	})
	if err != nil {
		writeDocumentError(context, err)
		return
	}
	context.JSON(http.StatusAccepted, result)
}

func (handler *Handler) List(context *gin.Context) {
	principal, ok := documentPrincipal(context)
	if !ok {
		return
	}
	var libraryID *uuid.UUID
	if raw := context.Query("library_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(context, httpx.BadRequest("INVALID_LIBRARY_ID", "知识库 ID 无效"))
			return
		}
		libraryID = &parsed
	}
	limit, _ := strconv.Atoi(context.Query("limit"))
	documents, err := handler.service.List(context, principal.UserID, ListFilter{
		LibraryID: libraryID, Keyword: context.Query("keyword"), Status: Status(context.Query("status")), Limit: limit,
	})
	if err != nil {
		writeDocumentError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"documents": documents})
}

func (handler *Handler) Get(context *gin.Context) {
	principal, documentID, ok := documentContext(context)
	if !ok {
		return
	}
	document, err := handler.service.Get(context, principal.UserID, documentID)
	if err != nil {
		writeDocumentError(context, err)
		return
	}
	context.JSON(http.StatusOK, document)
}

func (handler *Handler) Update(context *gin.Context) {
	principal, documentID, ok := documentContext(context)
	if !ok {
		return
	}
	var request MetadataCommand
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_DOCUMENT", "文件信息格式无效"))
		return
	}
	if err := handler.service.UpdateMetadata(context, principal.UserID, documentID, request); err != nil {
		writeDocumentError(context, err)
		return
	}
	context.Status(http.StatusNoContent)
}

func (handler *Handler) Content(context *gin.Context) {
	principal, documentID, ok := documentContext(context)
	if !ok {
		return
	}
	version, text, err := handler.service.Content(context, principal.UserID, documentID)
	if err != nil {
		writeDocumentError(context, err)
		return
	}
	context.JSON(http.StatusOK, gin.H{"content_version": version, "text": text})
}

func (handler *Handler) EditContent(context *gin.Context) {
	principal, documentID, ok := documentContext(context)
	if !ok {
		return
	}
	var request struct {
		Text string `json:"text" binding:"required"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_CONTENT", "编辑内容不能为空"))
		return
	}
	result, err := handler.service.EditContent(context, principal.UserID, documentID, request.Text)
	if err != nil {
		writeDocumentError(context, err)
		return
	}
	context.JSON(http.StatusAccepted, result)
}

func (handler *Handler) Reindex(context *gin.Context) {
	principal, documentID, ok := documentContext(context)
	if !ok {
		return
	}
	jobID, err := handler.service.Reindex(context, principal.UserID, documentID)
	if err != nil {
		writeDocumentError(context, err)
		return
	}
	context.JSON(http.StatusAccepted, gin.H{"job_id": jobID})
}

func (handler *Handler) Delete(context *gin.Context) {
	principal, documentID, ok := documentContext(context)
	if !ok {
		return
	}
	jobID, err := handler.service.Delete(context, principal.UserID, documentID)
	if err != nil {
		writeDocumentError(context, err)
		return
	}
	context.JSON(http.StatusAccepted, gin.H{"job_id": jobID})
}

func documentContext(context *gin.Context) (httpx.Principal, uuid.UUID, bool) {
	principal, ok := documentPrincipal(context)
	if !ok {
		return httpx.Principal{}, uuid.Nil, false
	}
	documentID, err := uuid.Parse(context.Param("id"))
	if err != nil {
		httpx.WriteError(context, httpx.BadRequest("INVALID_DOCUMENT_ID", "文件 ID 无效"))
		return httpx.Principal{}, uuid.Nil, false
	}
	return principal, documentID, true
}

func documentPrincipal(context *gin.Context) (httpx.Principal, bool) {
	principal, ok := httpx.PrincipalFrom(context)
	if !ok {
		httpx.WriteError(context, &httpx.Error{Status: http.StatusUnauthorized, Code: "AUTH_REQUIRED", Message: "请先登录"})
	}
	return principal, ok
}

func writeDocumentError(context *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDocumentNotFound), errors.Is(err, librarydomain.ErrNotFound):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusNotFound, Code: "DOCUMENT_NOT_FOUND", Message: "文件不存在"})
	case errors.Is(err, ErrDocumentReadOnly), errors.Is(err, librarydomain.ErrForbidden):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusForbidden, Code: "DOCUMENT_READ_ONLY", Message: "公开知识库文件仅支持只读访问"})
	case errors.Is(err, ErrFileTooLarge):
		httpx.WriteError(context, &httpx.Error{Status: http.StatusRequestEntityTooLarge, Code: "FILE_TOO_LARGE", Message: "单文件不能超过 50 MB"})
	case errors.Is(err, ErrUnsupportedFormat), errors.Is(err, ErrInvalidFile):
		httpx.WriteError(context, httpx.BadRequest("INVALID_FILE_FORMAT", "文件格式不受支持或内容与扩展名不匹配"))
	default:
		httpx.WriteError(context, err)
	}
}
