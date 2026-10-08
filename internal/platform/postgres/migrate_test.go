package postgres

import (
	"strings"
	"testing"
)

func TestMigrationsContainRequiredTables(t *testing.T) {
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 || migrations[0].Version != 1 {
		t.Fatalf("unexpected migrations: %#v", migrations)
	}
	required := []string{
		"schema_migrations", "users", "wechat_identities", "plans", "user_plans",
		"auth_sessions", "web_login_tickets", "knowledge_bases", "library_retrieval_settings",
		"documents", "document_contents", "index_jobs", "chat_sessions", "chat_messages",
		"message_references", "usage_records", "monthly_usage", "audit_logs", "idempotency_records",
	}
	for _, table := range required {
		if !strings.Contains(migrations[0].SQL, "CREATE TABLE "+table) &&
			!strings.Contains(migrations[0].SQL, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("migration is missing table %s", table)
		}
	}
}

func TestMigrationContainsTaskClaimAndActiveJobIndexes(t *testing.T) {
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	sql := migrations[0].SQL
	for _, fragment := range []string{"idx_jobs_claim", "uk_jobs_active_document", "idx_libraries_public"} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("migration is missing %s", fragment)
		}
	}
}
