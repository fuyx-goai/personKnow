package document

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	platformpostgres "knowledge-base/internal/platform/postgres"
	"knowledge-base/pkg/config"
)

func TestPostgresRepositoryAllowsPublicReadButNotOwnership(t *testing.T) {
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
	ownerID, readerID, libraryID := uuid.New(), uuid.New(), uuid.New()
	for _, userID := range []uuid.UUID{ownerID, readerID} {
		if _, err := pool.Exec(ctx, `INSERT INTO users(id,nickname) VALUES($1,'用户')`, userID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO knowledge_bases
        (id,owner_user_id,name,visibility,status) VALUES($1,$2,'公开库','public','active')`, libraryID, ownerID); err != nil {
		t.Fatal(err)
	}
	document := Document{
		ID: uuid.New(), LibraryID: libraryID, UploadedBy: ownerID, OriginalName: "note.md", DisplayName: "note.md",
		Format: FormatMarkdown, MIMEType: "text/plain", OriginalBytes: 10, OriginalSHA256: strings64("a"),
		OriginalPath: "path", Status: StatusQueued, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repository := NewPostgresRepository(pool)
	if err := repository.Create(ctx, document); err != nil {
		t.Fatal(err)
	}
	got, err := repository.Get(ctx, readerID, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != document.ID {
		t.Fatalf("unexpected document: %#v", got)
	}
}

func strings64(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
