package entity

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	ListOwned(context.Context, uuid.UUID, ListFilter) ([]Library, error)
	ListPublic(context.Context, uuid.UUID, ListFilter) ([]Library, error)
	Create(context.Context, Library, RetrievalSettings) error
	Get(context.Context, uuid.UUID) (Library, error)
	Update(context.Context, Library) error
	MarkDeleting(context.Context, uuid.UUID) error
	GetSettings(context.Context, uuid.UUID) (RetrievalSettings, error)
	UpdateSettings(context.Context, RetrievalSettings) error
}

type ReindexScheduler interface {
	ScheduleLibrary(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error)
}
