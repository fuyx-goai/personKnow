package vector_test

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/google/uuid"

	"knowledge-base/internal/knowledge/repo/vectorstore"
	platformvector "knowledge-base/internal/platform/vector"
)

type fixedEmbedder struct{}

func (fixedEmbedder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	vectors := make([][]float64, len(texts))
	for index := range texts {
		vectors[index] = []float64{1, 0}
	}
	return vectors, nil
}

func TestScopedRepositoryContract(t *testing.T) {
	t.Chdir(t.TempDir())
	store, err := vectorstore.NewMemStore(context.Background(), fixedEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	repository := vectorstore.NewScopedRepository(store)
	ownerID, libraryID, otherLibraryID := uuid.New(), uuid.New(), uuid.New()
	documentID, activeVersionID, oldVersionID := uuid.New(), uuid.New(), uuid.New()
	chunks := []platformvector.Chunk{
		newChunk(ownerID, libraryID, documentID, activeVersionID, 0, "active", []float64{1, 0}),
		newChunk(ownerID, libraryID, documentID, oldVersionID, 0, "old", []float64{1, 0}),
		newChunk(ownerID, otherLibraryID, uuid.New(), uuid.New(), 0, "other library", []float64{1, 0}),
	}
	if err := repository.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}
	scope := platformvector.SearchScope{
		LibraryIDs: []uuid.UUID{libraryID}, ActiveVersions: map[uuid.UUID]uuid.UUID{documentID: activeVersionID}, MinSimilarity: 0.8,
	}
	hits, err := repository.Search(context.Background(), "query", scope, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Content != "active" {
		t.Fatalf("scope returned unexpected hits: %+v", hits)
	}
	deleted, err := repository.DeleteVersion(context.Background(), oldVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}
	count, err := repository.Count(context.Background(), platformvector.SearchScope{LibraryIDs: []uuid.UUID{libraryID}})
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v, want 1", count, err)
	}
}

func TestStableChunkIDUsesVersionAndIndex(t *testing.T) {
	documentID, versionID := uuid.New(), uuid.New()
	first := platformvector.StableChunkID(documentID, versionID, 3)
	if first != platformvector.StableChunkID(documentID, versionID, 3) {
		t.Fatal("same document version and index produced different IDs")
	}
	if first == platformvector.StableChunkID(documentID, versionID, 4) || first == platformvector.StableChunkID(documentID, uuid.New(), 3) {
		t.Fatal("chunk ID did not change with version or index")
	}
}

func newChunk(ownerID, libraryID, documentID, versionID uuid.UUID, index int, content string, vector []float64) platformvector.Chunk {
	return platformvector.Chunk{
		ID: platformvector.StableChunkID(documentID, versionID, index), Content: content, Vector: vector,
		OwnerUserID: ownerID, LibraryID: libraryID, DocumentID: documentID, ContentVersionID: versionID,
		ChunkIndex: index, SourceName: "notes.md", LocationLabel: "section 1", Visibility: "private",
	}
}
