package repo

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	platformpostgres "knowledge-base/internal/platform/postgres"
	"knowledge-base/pkg/config"
)

func TestSeedIsIdempotent(t *testing.T) {
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
	userID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,nickname) VALUES($1,'演示数据测试用户')`, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupSeedUser(context.Background(), pool, userID) })

	repository := NewPostgresRepository(pool)
	for run := 0; run < 2; run++ {
		if _, err := repository.Seed(ctx, userID, t.TempDir()); err != nil {
			t.Fatalf("第 %d 次写入失败：%v", run+1, err)
		}
	}
	assertCount(t, ctx, pool, `SELECT count(*) FROM knowledge_bases WHERE owner_user_id=$1`, userID, 3)
	assertCount(t, ctx, pool, `SELECT count(*) FROM documents d JOIN knowledge_bases k ON k.id=d.library_id WHERE k.owner_user_id=$1`, userID, 15)
	assertCount(t, ctx, pool, `SELECT count(*) FROM index_jobs j JOIN documents d ON d.id=j.document_id JOIN knowledge_bases k ON k.id=d.library_id WHERE k.owner_user_id=$1`, userID, 15)
	assertCount(t, ctx, pool, `SELECT count(*) FROM chat_sessions WHERE user_id=$1`, userID, 3)
	assertCount(t, ctx, pool, `SELECT count(*) FROM usage_records WHERE user_id=$1`, userID, 4)
}

func assertCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, userID uuid.UUID, expected int) {
	t.Helper()
	var actual int
	if err := pool.QueryRow(ctx, query, userID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("数量不符合预期：got=%d want=%d", actual, expected)
	}
}

func cleanupSeedUser(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) {
	queries := []string{
		`DELETE FROM message_references WHERE message_id IN (SELECT m.id FROM chat_messages m JOIN chat_sessions s ON s.id=m.session_id WHERE s.user_id=$1)`,
		`DELETE FROM chat_messages WHERE session_id IN (SELECT id FROM chat_sessions WHERE user_id=$1)`,
		`DELETE FROM chat_sessions WHERE user_id=$1`, `DELETE FROM usage_records WHERE user_id=$1`,
		`DELETE FROM audit_logs WHERE actor_user_id=$1`,
		`DELETE FROM index_jobs WHERE document_id IN (SELECT d.id FROM documents d JOIN knowledge_bases k ON k.id=d.library_id WHERE k.owner_user_id=$1)`,
		`UPDATE documents SET active_content_version_id=NULL WHERE library_id IN (SELECT id FROM knowledge_bases WHERE owner_user_id=$1)`,
		`DELETE FROM document_contents WHERE document_id IN (SELECT d.id FROM documents d JOIN knowledge_bases k ON k.id=d.library_id WHERE k.owner_user_id=$1)`,
		`DELETE FROM documents WHERE library_id IN (SELECT id FROM knowledge_bases WHERE owner_user_id=$1)`,
		`DELETE FROM library_retrieval_settings WHERE library_id IN (SELECT id FROM knowledge_bases WHERE owner_user_id=$1)`,
		`DELETE FROM knowledge_bases WHERE owner_user_id=$1`, `DELETE FROM monthly_usage WHERE user_id=$1`,
		`DELETE FROM user_plans WHERE user_id=$1`, `DELETE FROM users WHERE id=$1`,
	}
	for _, query := range queries {
		_, _ = pool.Exec(ctx, query, userID)
	}
}
