package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	. "knowledge-base/internal/document/entity"
	documentrepo "knowledge-base/internal/document/repo"
	librarydomain "knowledge-base/internal/library/entity"
)

type fakeLibraryReader struct {
	library librarydomain.Library
}

func (reader fakeLibraryReader) Get(context.Context, uuid.UUID, uuid.UUID) (librarydomain.Library, error) {
	return reader.library, nil
}

type fakeDocumentRepository struct {
	document Document
	content  ContentVersion
}

func (repo *fakeDocumentRepository) Create(_ context.Context, document Document) error {
	repo.document = document
	return nil
}

func (repo *fakeDocumentRepository) List(context.Context, uuid.UUID, ListFilter) ([]Document, error) {
	if repo.document.ID == uuid.Nil {
		return nil, nil
	}
	return []Document{repo.document}, nil
}

func (repo *fakeDocumentRepository) Get(context.Context, uuid.UUID, uuid.UUID) (Document, error) {
	if repo.document.ID == uuid.Nil {
		return Document{}, ErrDocumentNotFound
	}
	return repo.document, nil
}

func (repo *fakeDocumentRepository) UpdateMetadata(_ context.Context, document Document) error {
	repo.document = document
	return nil
}

func (repo *fakeDocumentRepository) NextContentVersion(context.Context, uuid.UUID) (int, error) {
	return repo.content.Version + 1, nil
}

func (repo *fakeDocumentRepository) CreateContentVersion(_ context.Context, content ContentVersion) error {
	repo.content = content
	return nil
}

func (repo *fakeDocumentRepository) GetContent(context.Context, uuid.UUID, uuid.UUID) (ContentVersion, error) {
	return repo.content, nil
}

func (repo *fakeDocumentRepository) MarkDeleting(context.Context, uuid.UUID) error { return nil }

type fakeStorageQuota struct {
	delta int64
	err   error
}

func (quota *fakeStorageQuota) ApplyStorage(_ context.Context, _ uuid.UUID, _ uuid.UUID, delta int64) error {
	if quota.err != nil {
		return quota.err
	}
	quota.delta += delta
	return nil
}

type fakeJobScheduler struct {
	requests []JobRequest
}

func (scheduler *fakeJobScheduler) ScheduleDocument(_ context.Context, request JobRequest) (uuid.UUID, error) {
	scheduler.requests = append(scheduler.requests, request)
	return uuid.New(), nil
}

func TestUploadStoresFileAccountsUsageAndSchedulesIndex(t *testing.T) {
	userID, libraryID := uuid.New(), uuid.New()
	repository := &fakeDocumentRepository{}
	quota := &fakeStorageQuota{}
	scheduler := &fakeJobScheduler{}
	service := NewService(Dependencies{
		Repository: repository,
		Libraries: fakeLibraryReader{library: librarydomain.Library{
			ID: libraryID, OwnerUserID: userID, Access: librarydomain.AccessOwner, Status: librarydomain.StatusActive,
		}},
		Files: documentrepo.NewLocalFileStore(t.TempDir()), Quota: quota, Jobs: scheduler,
	})

	result, err := service.Upload(context.Background(), userID, UploadCommand{
		LibraryID: libraryID, OriginalName: "note.md", Reader: strings.NewReader("# RAG知识"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Document.Status != StatusQueued || result.JobID == uuid.Nil {
		t.Fatalf("unexpected upload result: %#v", result)
	}
	if quota.delta != int64(len("# RAG知识")) || len(scheduler.requests) != 1 {
		t.Fatalf("usage or job missing: delta=%d jobs=%d", quota.delta, len(scheduler.requests))
	}
}

func TestPublicReaderCannotEditDocument(t *testing.T) {
	ownerID, readerID, libraryID := uuid.New(), uuid.New(), uuid.New()
	repository := &fakeDocumentRepository{document: Document{
		ID: uuid.New(), LibraryID: libraryID, UploadedBy: ownerID, DisplayName: "公开资料", Status: StatusReady,
	}}
	service := NewService(Dependencies{
		Repository: repository,
		Libraries: fakeLibraryReader{library: librarydomain.Library{
			ID: libraryID, OwnerUserID: ownerID, Access: librarydomain.AccessRead, Visibility: librarydomain.VisibilityPublic, Status: librarydomain.StatusActive,
		}},
		Files: documentrepo.NewLocalFileStore(t.TempDir()), Quota: &fakeStorageQuota{}, Jobs: &fakeJobScheduler{}, Now: time.Now,
	})
	_, err := service.EditContent(context.Background(), readerID, repository.document.ID, "越权内容")
	if !errors.Is(err, ErrDocumentReadOnly) {
		t.Fatalf("expected read-only error, got %v", err)
	}
}
