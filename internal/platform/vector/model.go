package vector

import (
	"fmt"

	"github.com/google/uuid"
)

const (
	MetaOwnerUserID    = "owner_user_id"
	MetaLibraryID      = "library_id"
	MetaDocumentID     = "document_id"
	MetaContentVersion = "content_version_id"
	MetaChunkIndex     = "chunk_index"
	MetaSourceName     = "source_name"
	MetaLocationLabel  = "page_or_section"
	MetaVisibility     = "visibility"
)

type Chunk struct {
	ID               string
	Content          string
	Vector           []float64
	OwnerUserID      uuid.UUID
	LibraryID        uuid.UUID
	DocumentID       uuid.UUID
	ContentVersionID uuid.UUID
	ChunkIndex       int
	SourceName       string
	LocationLabel    string
	Visibility       string
	Similarity       float64
}

type SearchScope struct {
	LibraryIDs     []uuid.UUID
	ActiveVersions map[uuid.UUID]uuid.UUID
	MinSimilarity  float64
}

func StableChunkID(documentID, versionID uuid.UUID, index int) string {
	return fmt.Sprintf("%s:%s:%06d", documentID, versionID, index)
}
