package entity

import (
	"context"
	"io"

	"github.com/google/uuid"

	librarydomain "knowledge-base/internal/library/entity"
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

type FileStore interface {
	SaveOriginal(context.Context, Location, string, io.Reader, int64) (StoredFile, error)
	WriteContent(Location, int, string) (StoredFile, error)
	Read(string) ([]byte, error)
	DeleteDocument(Location) error
	Absolute(string) string
	DetectFormat(string, string) (Format, string, error)
}
