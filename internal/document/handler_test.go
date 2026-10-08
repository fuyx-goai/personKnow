package document

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	librarydomain "knowledge-base/internal/library"
	"knowledge-base/internal/platform/httpx"
)

func TestHandlerUploadsMultipartDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID, libraryID := uuid.New(), uuid.New()
	repository := &fakeDocumentRepository{}
	service := NewService(Dependencies{
		Repository: repository,
		Libraries: fakeLibraryReader{library: librarydomain.Library{
			ID: libraryID, OwnerUserID: userID, Access: librarydomain.AccessOwner, Status: librarydomain.StatusActive,
		}},
		Files: NewLocalFileStore(t.TempDir()), Quota: &fakeStorageQuota{}, Jobs: &fakeJobScheduler{},
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
