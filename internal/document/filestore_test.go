package document

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestFileStoreNeverUsesClientNameInPath(t *testing.T) {
	store := NewLocalFileStore(t.TempDir())
	location := Location{UserID: uuid.New(), LibraryID: uuid.New(), DocumentID: uuid.New()}
	stored, err := store.SaveOriginal(context.Background(), location, "../../secret.txt", strings.NewReader("safe"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.RelativePath, "..") || strings.Contains(stored.RelativePath, "secret") {
		t.Fatalf("unsafe path %q", stored.RelativePath)
	}
	data, err := os.ReadFile(store.Absolute(stored.RelativePath))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "safe" || stored.Size != 4 || stored.SHA256 == "" {
		t.Fatalf("unexpected stored file: %#v %q", stored, data)
	}
}

func TestFileStoreRejectsOversizedUpload(t *testing.T) {
	store := NewLocalFileStore(t.TempDir())
	location := Location{UserID: uuid.New(), LibraryID: uuid.New(), DocumentID: uuid.New()}
	_, err := store.SaveOriginal(context.Background(), location, "large.txt", strings.NewReader("12345"), 4)
	if err == nil {
		t.Fatal("expected oversized upload to fail")
	}
}
