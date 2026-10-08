package migration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"knowledge-base/internal/document"
	platformvector "knowledge-base/internal/platform/vector"
)

type memoryRepository struct {
	libraryID uuid.UUID
	documents map[uuid.UUID]bool
}

func (repo *memoryRepository) EnsureDefaultLibrary(context.Context, uuid.UUID) (uuid.UUID, error) {
	return repo.libraryID, nil
}

func (repo *memoryRepository) PrepareDocument(_ context.Context, item ImportDocument) (DocumentState, error) {
	if repo.documents == nil {
		repo.documents = map[uuid.UUID]bool{}
	}
	ready, exists := repo.documents[item.DocumentID]
	if !exists {
		repo.documents[item.DocumentID] = false
	}
	return DocumentState{Created: !exists, Ready: ready}, nil
}

func (repo *memoryRepository) CompleteDocument(_ context.Context, documentID uuid.UUID, _ int) (bool, error) {
	if repo.documents[documentID] {
		return false, nil
	}
	repo.documents[documentID] = true
	return true, nil
}

func (repo *memoryRepository) RecordAudit(context.Context, AuditRecord) error { return nil }

type memoryVectors struct {
	chunks []platformvector.Chunk
	err    error
}

func (writer *memoryVectors) Upsert(_ context.Context, chunks []platformvector.Chunk) error {
	if writer.err != nil {
		return writer.err
	}
	writer.chunks = append(writer.chunks, chunks...)
	return nil
}

type memoryFiles struct{}

func (memoryFiles) WriteContent(_ document.Location, _ int, text string) (document.StoredFile, error) {
	return document.StoredFile{RelativePath: "content/v1.txt", Size: int64(len(text)), SHA256: contentHash(text)}, nil
}

func TestLegacyMigrationIsIdempotentAndGroupsBySource(t *testing.T) {
	targetUser := uuid.New()
	repository := &memoryRepository{libraryID: uuid.New()}
	vectors := &memoryVectors{}
	migrator := NewLegacyMigrator(Dependencies{
		Repository: repository, Vectors: vectors, Files: memoryFiles{},
		Now: func() time.Time { return time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC) },
	})
	source := writeLegacyFixture(t, []map[string]any{
		legacyFixtureEntry("第一段", []float64{0.1, 0.2}, "notes/a.md"),
		legacyFixtureEntry("第二段", []float64{0.3, 0.4}, "notes/a.md"),
		legacyFixtureEntry("另一份资料", []float64{0.5, 0.6}, "docs/b.txt"),
	})
	options := Options{TargetUser: targetUser, SourcePath: source, BackupDir: filepath.Join(t.TempDir(), "backups")}

	first, err := migrator.Migrate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if first.DocumentsAdded != 2 || first.ChunksAdded != 3 {
		t.Fatalf("first report = %+v, want 2 documents and 3 chunks", first)
	}
	if len(repository.documents) != 2 || len(vectors.chunks) != 3 {
		t.Fatalf("stored documents=%d chunks=%d", len(repository.documents), len(vectors.chunks))
	}
	assertBackupExists(t, options.BackupDir)

	second, err := migrator.Migrate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if second.DocumentsAdded != 0 || second.ChunksAdded != 0 {
		t.Fatalf("second report = %+v, want zero additions", second)
	}
}

func TestLegacyMigrationFailureReportDoesNotExposeContent(t *testing.T) {
	secret := "不应出现在迁移报告里的正文"
	source := writeLegacyFixture(t, []map[string]any{
		legacyFixtureEntry(secret, []float64{}, "broken.md"),
	})
	migrator := NewLegacyMigrator(Dependencies{
		Repository: &memoryRepository{libraryID: uuid.New()}, Vectors: &memoryVectors{}, Files: memoryFiles{},
	})

	report, err := migrator.Migrate(context.Background(), Options{
		TargetUser: uuid.New(), SourcePath: source, BackupDir: filepath.Join(t.TempDir(), "backups"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 || report.Failures[0].Source != "broken.md" || report.Failures[0].Code != "invalid_vector" {
		t.Fatalf("failures = %+v", report.Failures)
	}
	encoded, marshalErr := json.Marshal(report)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("failure report leaked content: %s", encoded)
	}
}

func TestLegacyMigrationReturnsSafeVectorWriteFailure(t *testing.T) {
	secret := "向量写入失败也不能泄露正文"
	source := writeLegacyFixture(t, []map[string]any{legacyFixtureEntry(secret, []float64{0.1}, "write-fail.md")})
	migrator := NewLegacyMigrator(Dependencies{
		Repository: &memoryRepository{libraryID: uuid.New()},
		Vectors:    &memoryVectors{err: errors.New("backend unavailable")},
		Files:      memoryFiles{},
	})

	report, err := migrator.Migrate(context.Background(), Options{
		TargetUser: uuid.New(), SourcePath: source, BackupDir: filepath.Join(t.TempDir(), "backups"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 || report.Failures[0].Code != "vector_write_failed" {
		t.Fatalf("failures = %+v", report.Failures)
	}
	if strings.Contains(report.Failures[0].Message, secret) {
		t.Fatalf("failure message leaked content: %q", report.Failures[0].Message)
	}
}

func writeLegacyFixture(t *testing.T, entries []map[string]any) string {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "knowledge.json")
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func legacyFixtureEntry(content string, vector []float64, source string) map[string]any {
	return map[string]any{
		"content": content,
		"vector":  vector,
		"meta": map[string]any{
			"_source":    source,
			"_file_name": filepath.Base(source),
		},
	}
}

func assertBackupExists(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected a legacy backup")
	}
}
