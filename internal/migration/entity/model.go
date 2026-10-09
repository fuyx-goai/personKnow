package entity

import (
	"context"
	"time"

	"github.com/google/uuid"

	document "knowledge-base/internal/document/entity"
	platformvector "knowledge-base/internal/platform/vector"
)

var LegacyNamespace = uuid.MustParse("c70563c5-8907-4e14-b398-49370e0cf88c")

type Repository interface {
	EnsureDefaultLibrary(context.Context, uuid.UUID) (uuid.UUID, error)
	PrepareDocument(context.Context, ImportDocument) (DocumentState, error)
	CompleteDocument(context.Context, uuid.UUID, int) (bool, error)
	RecordAudit(context.Context, AuditRecord) error
}

type VectorWriter interface {
	Upsert(context.Context, []platformvector.Chunk) error
}

type ContentWriter interface {
	WriteContent(document.Location, int, string) (document.StoredFile, error)
}

type Dependencies struct {
	Repository Repository
	Vectors    VectorWriter
	Files      ContentWriter
	Now        func() time.Time
}

type Options struct {
	TargetUser uuid.UUID
	SourcePath string
	BackupDir  string
}

type Report struct {
	BackupPath     string    `json:"backup_path"`
	DocumentsAdded int       `json:"documents_added"`
	ChunksAdded    int       `json:"chunks_added"`
	Failures       []Failure `json:"failures"`
}

type Failure struct {
	Source  string `json:"source"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ImportDocument struct {
	TargetUser       uuid.UUID
	LibraryID        uuid.UUID
	DocumentID       uuid.UUID
	ContentVersionID uuid.UUID
	SourceName       string
	DisplayName      string
	Extension        string
	OriginalHash     string
	OriginalBytes    int64
	ContentPath      string
	ContentHash      string
	ChunkCount       int
	CreatedAt        time.Time
}

type DocumentState struct {
	Created bool
	Ready   bool
}

type AuditRecord struct {
	TargetUser   uuid.UUID
	LibraryID    uuid.UUID
	Documents    int
	Chunks       int
	FailureCount int
	OccurredAt   time.Time
}
