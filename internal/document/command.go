package document

import (
	"io"

	"github.com/google/uuid"
)

type ListFilter struct {
	LibraryID *uuid.UUID
	Keyword   string
	Status    Status
	Limit     int
	Cursor    string
}

type UploadCommand struct {
	LibraryID    uuid.UUID
	OriginalName string
	Reader       io.Reader
	Tags         []string
}

type UploadResult struct {
	Document Document  `json:"document"`
	JobID    uuid.UUID `json:"job_id"`
}

type EditResult struct {
	Content ContentVersion `json:"content"`
	JobID   uuid.UUID      `json:"job_id"`
}

type MetadataCommand struct {
	DisplayName string
	Tags        []string
}

type JobType string

const (
	JobIndex   JobType = "index"
	JobReindex JobType = "reindex"
	JobDelete  JobType = "delete"
)

type JobRequest struct {
	ActorID          uuid.UUID
	LibraryID        uuid.UUID
	DocumentID       uuid.UUID
	ContentVersionID *uuid.UUID
	Type             JobType
}
