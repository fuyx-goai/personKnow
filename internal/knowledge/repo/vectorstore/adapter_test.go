package vectorstore

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/google/uuid"

	knowledgemodel "knowledge-base/internal/knowledge/model"
	platformvector "knowledge-base/internal/platform/vector"
)

type adapterEmbedder struct{}

func (adapterEmbedder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	vectors := make([][]float64, len(texts))
	for index := range texts {
		vectors[index] = []float64{1, 0}
	}
	return vectors, nil
}

func TestLegacyRepositoryCannotReadOrDeleteScopedChunks(t *testing.T) {
	t.Chdir(t.TempDir())
	store, err := NewMemStore(context.Background(), adapterEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	legacy := NewRepository(store)
	scoped := NewScopedRepository(store)
	if _, err := legacy.Save(context.Background(), []knowledgemodel.Chunk{{Content: "legacy", Vector: []float64{1, 0}}}); err != nil {
		t.Fatal(err)
	}
	ownerID, libraryID, documentID, versionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := scoped.Upsert(context.Background(), []platformvector.Chunk{{
		ID: platformvector.StableChunkID(documentID, versionID, 0), Content: "private", Vector: []float64{1, 0},
		OwnerUserID: ownerID, LibraryID: libraryID, DocumentID: documentID, ContentVersionID: versionID,
	}}); err != nil {
		t.Fatal(err)
	}
	chunks, err := legacy.Search(context.Background(), "query", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0].Content != "legacy" {
		t.Fatalf("legacy search leaked scoped chunks: %+v", chunks)
	}
	if _, err := legacy.DeleteBySource(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	count, err := scoped.Count(context.Background(), platformvector.SearchScope{LibraryIDs: []uuid.UUID{libraryID}})
	if err != nil || count != 1 {
		t.Fatalf("legacy clear deleted scoped chunks: count=%d err=%v", count, err)
	}
}
