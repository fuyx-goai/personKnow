package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"knowledge-base/internal/chat"
	"knowledge-base/internal/document"
	"knowledge-base/internal/indexing"
	"knowledge-base/internal/library"
	platformpostgres "knowledge-base/internal/platform/postgres"
	platformvector "knowledge-base/internal/platform/vector"
	"knowledge-base/internal/usage"
	"knowledge-base/pkg/config"
)

func TestTwoUserVisibilityPublicQuestionAndReadOnlyWrite(t *testing.T) {
	ctx, pool := integrationDatabase(t)
	ownerID := seedUserWithPlan(t, ctx, pool, 1<<30, 100000)
	readerID := seedUserWithPlan(t, ctx, pool, 1<<30, 100000)
	usageService := usage.NewService(usage.NewPostgresRepository(pool), time.Now)
	libraryService := library.NewService(library.NewPostgresRepository(pool)).UseAuditor(usageService)
	created, err := libraryService.Create(ctx, ownerID, library.CreateCommand{Name: "集成测试私有库"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := libraryService.Get(ctx, readerID, created.ID); !errors.Is(err, library.ErrNotFound) {
		t.Fatalf("private library error = %v, want ErrNotFound", err)
	}
	if err := libraryService.Update(ctx, ownerID, created.ID, library.UpdateCommand{Visibility: library.VisibilityPublic}); err != nil {
		t.Fatal(err)
	}
	visible, err := libraryService.Get(ctx, readerID, created.ID)
	if err != nil || visible.Access != library.AccessRead {
		t.Fatalf("public read = %+v, err=%v", visible, err)
	}
	if err := libraryService.Update(ctx, readerID, created.ID, library.UpdateCommand{Name: "越权改名"}); !errors.Is(err, library.ErrForbidden) {
		t.Fatalf("public write error = %v, want ErrForbidden", err)
	}
	assertPublicDocumentWriteRejected(t, ctx, pool, libraryService, usageService, readerID, created.ID)
	assertPublicLibraryCanAnswer(t, ctx, pool, usageService, readerID, created.ID)
}

func TestQuotaRejectsBeforePersistenceEmbeddingAndModel(t *testing.T) {
	ctx, pool := integrationDatabase(t)
	userID := seedUserWithPlan(t, ctx, pool, 0, 0)
	usageService := usage.NewService(usage.NewPostgresRepository(pool), time.Now)
	libraryService := library.NewService(library.NewPostgresRepository(pool))
	created, err := libraryService.Create(ctx, userID, library.CreateCommand{Name: "零配额知识库"})
	if err != nil {
		t.Fatal(err)
	}
	jobs := &jobRecorder{}
	documentService := document.NewService(document.Dependencies{
		Repository: document.NewPostgresRepository(pool), Libraries: libraryService,
		Files: document.NewLocalFileStore(t.TempDir()), Quota: usageService, Jobs: jobs,
	})
	_, err = documentService.Upload(ctx, userID, document.UploadCommand{
		LibraryID: created.ID, OriginalName: "quota.md", Reader: strings.NewReader("# quota"),
	})
	if !errors.Is(err, usage.ErrStorageQuotaExceeded) || jobs.calls != 0 {
		t.Fatalf("upload err=%v jobs=%d", err, jobs.calls)
	}
	assertNoDocuments(t, ctx, pool, created.ID)
	assertIndexQuotaStopsEmbedding(t, ctx, usageService, userID, created.ID)
	assertChatQuotaStopsModel(t, ctx, pool, usageService, userID, created.ID)
}

func assertPublicDocumentWriteRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool, libraries *library.Service, quota *usage.Service, actorID, libraryID uuid.UUID) {
	t.Helper()
	service := document.NewService(document.Dependencies{
		Repository: document.NewPostgresRepository(pool), Libraries: libraries,
		Files: document.NewLocalFileStore(t.TempDir()), Quota: quota, Jobs: &jobRecorder{},
	})
	_, err := service.Upload(ctx, actorID, document.UploadCommand{
		LibraryID: libraryID, OriginalName: "forbidden.md", Reader: strings.NewReader("private"),
	})
	if !errors.Is(err, document.ErrDocumentReadOnly) {
		t.Fatalf("public upload error = %v, want ErrDocumentReadOnly", err)
	}
}

func assertPublicLibraryCanAnswer(t *testing.T, ctx context.Context, pool *pgxpool.Pool, quota *usage.Service, userID, libraryID uuid.UUID) {
	t.Helper()
	engine := &chatEngine{}
	service := chat.NewService(chat.ServiceDependencies{
		Repository: chat.NewPostgresRepository(pool), Engine: engine, Quota: quota, Now: time.Now,
	})
	session, err := service.CreateSession(ctx, userID, chat.CreateSessionCommand{
		ScopeType: chat.ScopeSingleLibrary, LibraryID: &libraryID, Title: "公开库问答",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Stream(ctx, userID, session.ID, "可以读取吗", func(chat.Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if engine.calls != 1 || engine.libraryID != libraryID {
		t.Fatalf("engine calls=%d library=%s", engine.calls, engine.libraryID)
	}
}

func assertIndexQuotaStopsEmbedding(t *testing.T, ctx context.Context, quota *usage.Service, userID, libraryID uuid.UUID) {
	t.Helper()
	embedder := &embedderRecorder{}
	vectors := &vectorRecorder{}
	processor := indexing.NewProcessor(indexing.ProcessorDependencies{
		Sources: sourceLoader{source: indexing.IndexSource{
			OwnerUserID: userID, LibraryID: libraryID, DocumentID: uuid.New(), ContentVersionID: uuid.New(),
			Content: "需要向量化的内容", SourceName: "quota.md", Visibility: "private",
			Settings: library.RetrievalSettings{ChunkSize: 800, ChunkOverlap: 100},
		}},
		Embedder: embedder, Vectors: vectors, Quota: quota,
	})
	_, err := processor.Process(ctx, indexing.Job{ID: uuid.New(), Type: indexing.JobIndex}, func(context.Context, string, int) error { return nil })
	if !errors.Is(err, usage.ErrTokenQuotaExceeded) || embedder.calls != 0 || vectors.upserts != 0 {
		t.Fatalf("index err=%v embedder=%d vectors=%d", err, embedder.calls, vectors.upserts)
	}
}

func assertChatQuotaStopsModel(t *testing.T, ctx context.Context, pool *pgxpool.Pool, quota *usage.Service, userID, libraryID uuid.UUID) {
	t.Helper()
	engine := &chatEngine{}
	service := chat.NewService(chat.ServiceDependencies{
		Repository: chat.NewPostgresRepository(pool), Engine: engine, Quota: quota, Now: time.Now,
	})
	session, err := service.CreateSession(ctx, userID, chat.CreateSessionCommand{
		ScopeType: chat.ScopeSingleLibrary, LibraryID: &libraryID, Title: "配额问答",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = service.Stream(ctx, userID, session.ID, "触发配额", func(chat.Event) error { return nil })
	if !errors.Is(err, usage.ErrTokenQuotaExceeded) || engine.calls != 0 {
		t.Fatalf("chat err=%v engine calls=%d", err, engine.calls)
	}
	var messages int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chat_messages WHERE session_id=$1`, session.ID).Scan(&messages); err != nil || messages != 0 {
		t.Fatalf("messages=%d err=%v", messages, err)
	}
}

func integrationDatabase(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := platformpostgres.Open(ctx, config.DatabaseConfig{URL: databaseURL, MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := platformpostgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return ctx, pool
}

func seedUserWithPlan(t *testing.T, ctx context.Context, pool *pgxpool.Pool, storageBytes, monthlyTokens int64) uuid.UUID {
	t.Helper()
	userID, planID := uuid.New(), uuid.New()
	code := "it-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,nickname) VALUES($1,'集成测试用户')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO plans(id,code,name,storage_quota_bytes,monthly_token_quota)
        VALUES($1,$2,'Integration',$3,$4)`, planID, code, storageBytes, monthlyTokens); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_plans(id,user_id,plan_id) VALUES($1,$2,$3)`, uuid.New(), userID, planID); err != nil {
		t.Fatal(err)
	}
	return userID
}

func assertNoDocuments(t *testing.T, ctx context.Context, pool *pgxpool.Pool, libraryID uuid.UUID) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM documents WHERE library_id=$1`, libraryID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("document count=%d err=%v", count, err)
	}
}

type jobRecorder struct{ calls int }

func (recorder *jobRecorder) ScheduleDocument(context.Context, document.JobRequest) (uuid.UUID, error) {
	recorder.calls++
	return uuid.New(), nil
}

type sourceLoader struct{ source indexing.IndexSource }

func (loader sourceLoader) Load(context.Context, indexing.Job) (indexing.IndexSource, error) {
	return loader.source, nil
}

type embedderRecorder struct{ calls int }

func (recorder *embedderRecorder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	recorder.calls++
	return make([][]float64, len(texts)), nil
}

type vectorRecorder struct{ upserts int }

func (recorder *vectorRecorder) Upsert(_ context.Context, chunks []platformvector.Chunk) error {
	recorder.upserts += len(chunks)
	return nil
}

func (*vectorRecorder) Search(context.Context, string, platformvector.SearchScope, int) ([]platformvector.Chunk, error) {
	return nil, nil
}

func (*vectorRecorder) DeleteVersion(context.Context, uuid.UUID) (int, error) { return 0, nil }

func (*vectorRecorder) Count(context.Context, platformvector.SearchScope) (int, error) { return 0, nil }

type chatEngine struct {
	calls     int
	libraryID uuid.UUID
}

func (engine *chatEngine) Stream(_ context.Context, _ string, scope platformvector.SearchScope, _ chat.RetrievalSettings, onDelta func(string) error) (chat.EngineResult, error) {
	engine.calls++
	if len(scope.LibraryIDs) > 0 {
		engine.libraryID = scope.LibraryIDs[0]
	}
	if err := onDelta("公开资料回答"); err != nil {
		return chat.EngineResult{}, err
	}
	return chat.EngineResult{Usage: usage.Breakdown{Input: 2, Output: 2}}, nil
}
