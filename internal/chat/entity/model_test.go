package entity

import (
	"testing"

	"github.com/google/uuid"
)

func TestDeduplicateReferencesKeepsHighestSimilarityPerDocument(t *testing.T) {
	documentID := uuid.New()
	otherDocumentID := uuid.New()
	references := []Reference{
		{DocumentID: documentID, ChunkID: "chunk-low", SourceName: "same.md", Similarity: 0.56, Rank: 2},
		{DocumentID: documentID, ChunkID: "chunk-high", SourceName: "same.md", Similarity: 0.62, Rank: 1},
		{DocumentID: otherDocumentID, ChunkID: "chunk-other", SourceName: "other.md", Similarity: 0.51, Rank: 3},
	}

	actual := DeduplicateReferences(references)
	if len(actual) != 2 {
		t.Fatalf("expected two unique documents, got %d", len(actual))
	}
	if actual[0].DocumentID != documentID || actual[0].ChunkID != "chunk-high" || actual[0].Similarity != 0.62 || actual[0].Rank != 1 {
		t.Fatalf("highest-similarity reference was not retained: %+v", actual[0])
	}
	if actual[1].DocumentID != otherDocumentID || actual[1].Rank != 2 {
		t.Fatalf("reference ranks were not normalized: %+v", actual)
	}
}
