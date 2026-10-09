package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	. "knowledge-base/internal/document/entity"
	documentrepo "knowledge-base/internal/document/repo"
	documentservice "knowledge-base/internal/document/service"
	librarydomain "knowledge-base/internal/library/entity"
	"knowledge-base/internal/platform/httpx"
)

func TestHandlerUploadsMultipartDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID, libraryID := uuid.New(), uuid.New()
	repository := &handlerDocumentRepository{}
	service := documentservice.NewService(documentservice.Dependencies{
		Repository: repository,
		Libraries: handlerLibraryReader{library: librarydomain.Library{
			ID: libraryID, OwnerUserID: userID, Access: librarydomain.AccessOwner, Status: librarydomain.StatusActive,
		}},
		Files: documentrepo.NewLocalFileStore(t.TempDir()), Quota: handlerStorageQuota{}, Jobs: handlerJobScheduler{},
	})
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(context *gin.Context) {
		context.Set("principal", httpx.Principal{UserID: userID})
		context.Next()
	})
	router.POST("/documents", handler.Upload)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("library_id", libraryID.String())
	file, err := writer.CreateFormFile("file", "note.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("# 上传知识"))
	_ = writer.Close()

	request := httptest.NewRequest(http.MethodPost, "/documents", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
}

type handlerLibraryReader struct{ library librarydomain.Library }

func (reader handlerLibraryReader) Get(context.Context, uuid.UUID, uuid.UUID) (librarydomain.Library, error) {
	return reader.library, nil
}

type handlerDocumentRepository struct{ document Document }

func (repository *handlerDocumentRepository) Create(_ context.Context, document Document) error {
	repository.document = document
	return nil
}

func (*handlerDocumentRepository) List(context.Context, uuid.UUID, ListFilter) ([]Document, error) {
	return nil, nil
}

func (repository *handlerDocumentRepository) Get(context.Context, uuid.UUID, uuid.UUID) (Document, error) {
	return repository.document, nil
}

func (*handlerDocumentRepository) UpdateMetadata(context.Context, Document) error { return nil }

func (*handlerDocumentRepository) NextContentVersion(context.Context, uuid.UUID) (int, error) {
	return 1, nil
}

func (*handlerDocumentRepository) CreateContentVersion(context.Context, ContentVersion) error {
	return nil
}

func (*handlerDocumentRepository) GetContent(context.Context, uuid.UUID, uuid.UUID) (ContentVersion, error) {
	return ContentVersion{}, nil
}

func (*handlerDocumentRepository) MarkDeleting(context.Context, uuid.UUID) error { return nil }

type handlerStorageQuota struct{}

func (handlerStorageQuota) ApplyStorage(context.Context, uuid.UUID, uuid.UUID, int64) error {
	return nil
}

type handlerJobScheduler struct{}

func (handlerJobScheduler) ScheduleDocument(context.Context, JobRequest) (uuid.UUID, error) {
	return uuid.New(), nil
}
