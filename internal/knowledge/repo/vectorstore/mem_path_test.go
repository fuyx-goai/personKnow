package vectorstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestMemStoreAtUsesExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy", "knowledge.json")
	store, err := NewMemStoreAt(context.Background(), nil, path)
	if err != nil {
		t.Fatal(err)
	}
	document := (&schema.Document{ID: "chunk-1", Content: "content"}).WithDenseVector([]float64{0.1, 0.2})
	if _, err := store.Store(context.Background(), []*schema.Document{document}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("explicit vector file was not created: %v", err)
	}
	reloaded, err := NewMemStoreAt(context.Background(), nil, path)
	if err != nil {
		t.Fatal(err)
	}
	count, err := reloaded.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}
