package vector

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Upsert(context.Context, []Chunk) error
	Search(context.Context, string, SearchScope, int) ([]Chunk, error)
	DeleteVersion(context.Context, uuid.UUID) (int, error)
	Count(context.Context, SearchScope) (int, error)
}
