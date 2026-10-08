package document

import (
	"context"

	"github.com/google/uuid"

	librarydomain "knowledge-base/internal/library"
)

type Repository interface {
	Create(context.Context, Document) error
	List(context.Context, uuid.UUID, ListFilter) ([]Document, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (Document, error)
	UpdateMetadata(context.Context, Document) error
	NextContentVersion(context.Context, uuid.UUID) (int, error)
	CreateContentVersion(context.Context, ContentVersion) error
	GetContent(context.Context, uuid.UUID, uuid.UUID) (ContentVersion, error)
	MarkDeleting(context.Context, uuid.UUID) error
}

type LibraryReader interface {
	Get(context.Context, uuid.UUID, uuid.UUID) (librarydomain.Library, error)
}

type StorageQuota interface {
	ApplyStorage(context.Context, uuid.UUID, uuid.UUID, int64) error
}

type JobScheduler interface {
	ScheduleDocument(context.Context, JobRequest) (uuid.UUID, error)
}
