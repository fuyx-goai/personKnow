package library

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	platformpostgres "knowledge-base/internal/platform/postgres"
	"knowledge-base/pkg/config"
)

func TestPostgresRepositoryListsPublicLibrariesSeparately(t *testing.T) {
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
	ownerID, readerID := uuid.New(), uuid.New()
	for _, userID := range []uuid.UUID{ownerID, readerID} {
		if _, err := pool.Exec(ctx, `INSERT INTO users(id,nickname) VALUES($1,$2)`, userID, "测试用户"); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewPostgresRepository(pool)
	library := Library{
		ID: uuid.New(), OwnerUserID: ownerID, Name: "公开技术库", Category: "技术工程",
		Visibility: VisibilityPublic, Status: StatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	settings := DefaultRetrievalSettings()
	settings.LibraryID, settings.UpdatedBy = library.ID, ownerID
	if err := repository.Create(ctx, library, settings); err != nil {
		t.Fatal(err)
	}
	publicLibraries, err := repository.ListPublic(ctx, readerID, ListFilter{Keyword: "技术", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(publicLibraries) != 1 || publicLibraries[0].ID != library.ID {
		t.Fatalf("unexpected public libraries: %#v", publicLibraries)
	}
	owned, err := repository.ListOwned(ctx, readerID, ListFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 0 {
		t.Fatalf("reader should not own public library: %#v", owned)
	}
}
