package migration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	platformpostgres "knowledge-base/internal/platform/postgres"
	"knowledge-base/pkg/config"
)

func TestPostgresRepositoryMigratesDocumentIdempotently(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := platformpostgres.Open(ctx, config.DatabaseConfig{URL: databaseURL, MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := platformpostgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	userID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,nickname) VALUES($1,'迁移测试用户')`, userID); err != nil {
		t.Fatal(err)
	}
	repository := NewPostgresRepository(pool)
	libraryID, err := repository.EnsureDefaultLibrary(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	documentID, versionID := uuid.New(), uuid.New()
	item := ImportDocument{
		TargetUser: userID, LibraryID: libraryID, DocumentID: documentID, ContentVersionID: versionID,
		SourceName: "notes.md", DisplayName: "notes.md", Extension: "md",
		OriginalHash: contentHash("legacy"), OriginalBytes: 6, ContentPath: "content/v1.txt",
		ContentHash: contentHash("legacy"), ChunkCount: 2, CreatedAt: time.Now().UTC(),
	}

	first, err := repository.PrepareDocument(ctx, item)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.PrepareDocument(ctx, item)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || second.Created || first.Ready || second.Ready {
		t.Fatalf("states = first %+v second %+v", first, second)
	}
	completed, err := repository.CompleteDocument(ctx, documentID, 2)
	if err != nil {
		t.Fatal(err)
	}
	completedAgain, err := repository.CompleteDocument(ctx, documentID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !completed || completedAgain {
		t.Fatalf("completed=%v completedAgain=%v", completed, completedAgain)
	}
	ready, err := repository.PrepareDocument(ctx, item)
	if err != nil {
		t.Fatal(err)
	}
	if !ready.Ready || ready.Created {
		t.Fatalf("ready state = %+v", ready)
	}
}
