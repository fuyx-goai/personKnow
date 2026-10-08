package indexing

import (
	"context"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/google/uuid"

	librarydomain "knowledge-base/internal/library"
	platformvector "knowledge-base/internal/platform/vector"
	usage "knowledge-base/internal/usage"
)

type fixedSourceLoader struct {
	source IndexSource
}

func (loader fixedSourceLoader) Load(context.Context, Job) (IndexSource, error) {
	return loader.source, nil
}

type processorEmbedder struct{}

func (processorEmbedder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	vectors := make([][]float64, len(texts))
	for index := range texts {
		vectors[index] = []float64{float64(index + 1), 1}
	}
	return vectors, nil
}

type processorVectorRepository struct {
	chunks  []platformvector.Chunk
	deleted []uuid.UUID
}

func (repository *processorVectorRepository) Upsert(_ context.Context, chunks []platformvector.Chunk) error {
	repository.chunks = append(repository.chunks, chunks...)
	return nil
}

func (*processorVectorRepository) Search(context.Context, string, platformvector.SearchScope, int) ([]platformvector.Chunk, error) {
	return nil, nil
}

func (repository *processorVectorRepository) DeleteVersion(_ context.Context, versionID uuid.UUID) (int, error) {
	repository.deleted = append(repository.deleted, versionID)
	return 1, nil
}

func (*processorVectorRepository) Count(context.Context, platformvector.SearchScope) (int, error) {
	return 0, nil
}

type processorQuota struct {
	estimated int64
	actual    usage.Breakdown
}

func (quota *processorQuota) ReserveTokens(_ context.Context, userID uuid.UUID, estimated int64) (usage.Reservation, error) {
	quota.estimated = estimated
	return usage.Reservation{ID: uuid.New(), UserID: userID, Estimated: estimated, MonthStart: time.Now()}, nil
}

func (quota *processorQuota) SettleTokens(_ context.Context, _ usage.Reservation, actual usage.Breakdown) error {
	quota.actual = actual
	return nil
}

func TestProcessorBuildsVersionedChunksAndSettlesEmbeddingUsage(t *testing.T) {
	ownerID, libraryID, documentID, versionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	vectorRepository := &processorVectorRepository{}
	quota := &processorQuota{}
	processor := NewProcessor(ProcessorDependencies{
		Sources: fixedSourceLoader{source: IndexSource{
			OwnerUserID: ownerID, LibraryID: libraryID, DocumentID: documentID, ContentVersionID: versionID,
			Content: "abcdefghij", SourceName: "notes.md", Visibility: "private",
			Settings: librarydomain.RetrievalSettings{ChunkSize: 5, ChunkOverlap: 1},
		}},
		Embedder: processorEmbedder{}, Vectors: vectorRepository, Quota: quota,
	})
	var stages []string
	result, err := processor.Process(context.Background(), Job{ID: uuid.New(), DocumentID: documentID, Type: JobIndex}, func(_ context.Context, stage string, _ int) error {
		stages = append(stages, stage)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentVersionID != versionID || result.ChunkCount != 3 || len(vectorRepository.chunks) != 3 {
		t.Fatalf("unexpected process result: %+v chunks=%d", result, len(vectorRepository.chunks))
	}
	for index, chunk := range vectorRepository.chunks {
		if chunk.OwnerUserID != ownerID || chunk.LibraryID != libraryID || chunk.DocumentID != documentID || chunk.ContentVersionID != versionID {
			t.Fatalf("chunk metadata incomplete: %+v", chunk)
		}
		if chunk.ID != platformvector.StableChunkID(documentID, versionID, index) {
			t.Fatalf("unstable chunk id: %s", chunk.ID)
		}
	}
	if quota.estimated == 0 || quota.actual.Embedding != quota.estimated || result.IndexedTokens != quota.estimated {
		t.Fatalf("usage not settled: estimated=%d actual=%+v result=%+v", quota.estimated, quota.actual, result)
	}
	if len(stages) < 3 {
		t.Fatalf("processing stages not reported: %+v", stages)
	}
}
