package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	. "knowledge-base/internal/library/entity"
	libraryservice "knowledge-base/internal/library/service"
	"knowledge-base/internal/platform/httpx"
)

type principalMiddleware struct {
	principal httpx.Principal
}

func (middleware principalMiddleware) handle(context *gin.Context) {
	context.Set("principal", middleware.principal)
	context.Next()
}

func TestHandlerCreatesPrivateLibraryByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	repository := &handlerLibraryRepository{}
	handler := NewHandler(libraryservice.NewService(repository))
	router := gin.New()
	router.Use(httpx.RequestID(), principalMiddleware{principal: httpx.Principal{UserID: userID}}.handle)
	router.POST("/libraries", handler.Create)

	request := httptest.NewRequest(http.MethodPost, "/libraries", strings.NewReader(`{"name":"AI 架构","category":"技术工程"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	if repository.library.OwnerUserID != userID || repository.library.Visibility != VisibilityPrivate {
		t.Fatalf("unexpected library: %#v", repository.library)
	}
}

type handlerLibraryRepository struct {
	library  Library
	settings RetrievalSettings
}

func (*handlerLibraryRepository) ListOwned(context.Context, uuid.UUID, ListFilter) ([]Library, error) {
	return nil, nil
}

func (*handlerLibraryRepository) ListPublic(context.Context, uuid.UUID, ListFilter) ([]Library, error) {
	return nil, nil
}

func (repository *handlerLibraryRepository) Create(_ context.Context, library Library, settings RetrievalSettings) error {
	repository.library, repository.settings = library, settings
	return nil
}

func (repository *handlerLibraryRepository) Get(context.Context, uuid.UUID) (Library, error) {
	return repository.library, nil
}

func (*handlerLibraryRepository) Update(context.Context, Library) error { return nil }

func (*handlerLibraryRepository) MarkDeleting(context.Context, uuid.UUID) error { return nil }

func (repository *handlerLibraryRepository) GetSettings(context.Context, uuid.UUID) (RetrievalSettings, error) {
	return repository.settings, nil
}

func (*handlerLibraryRepository) UpdateSettings(context.Context, RetrievalSettings) error { return nil }
