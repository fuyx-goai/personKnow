package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadUsesRuntimeValuesFromYAML(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`http_addr: :9090
vector_store: mem
database:
  url: postgres://yaml-user:yaml-pass@localhost/knowledge
  max_conns: 6
auth:
  jwt_secret: 01234567890123456789012345678901
  identity_secret: abcdefghijklmnopqrstuvwxyz123456
  access_ttl: 20m
  refresh_ttl: 720h
  wechat_app_id: wx-yaml
  wechat_app_secret: yaml-secret
storage:
  data_dir: yaml-data
worker:
  lease: 8m
  poll: 2s
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", configPath)
	t.Setenv("DATABASE_URL", "postgres://environment-must-not-win/db")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.URL != "postgres://yaml-user:yaml-pass@localhost/knowledge" {
		t.Fatalf("database URL not loaded from YAML: %q", cfg.Database.URL)
	}
	if cfg.Auth.JWTSecret == "" || cfg.Auth.WeChatAppID != "wx-yaml" {
		t.Fatal("authentication values were not loaded from YAML")
	}
	if cfg.Auth.IdentitySecret != "abcdefghijklmnopqrstuvwxyz123456" {
		t.Fatal("identity secret was not loaded from YAML")
	}
	if cfg.Auth.AccessTTL != 20*time.Minute || cfg.Auth.RefreshTTL != 30*24*time.Hour {
		t.Fatalf("unexpected token TTLs: %s %s", cfg.Auth.AccessTTL, cfg.Auth.RefreshTTL)
	}
	if cfg.Worker.Lease != 8*time.Minute || cfg.Worker.Poll != 2*time.Second {
		t.Fatalf("unexpected worker durations: %s %s", cfg.Worker.Lease, cfg.Worker.Poll)
	}
	if cfg.Storage.MaxFileBytes != 50*1024*1024 {
		t.Fatalf("unexpected max file size: %d", cfg.Storage.MaxFileBytes)
	}
}

func TestValidateAllowsStandaloneModeWithoutDatabase(t *testing.T) {
	cfg := Config{VectorStore: StoreMem, LLM: LLMConfig{APIKey: "configured-in-yaml"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("standalone mode should not require PostgreSQL: %v", err)
	}
}

func TestValidateRejectsShortJWTSecret(t *testing.T) {
	cfg := Config{
		VectorStore: StoreMem,
		LLM:         LLMConfig{APIKey: "configured-in-yaml"},
		Database:    DatabaseConfig{URL: "postgres://localhost/db"},
		Auth: AuthConfig{
			JWTSecret: "short", IdentitySecret: "abcdefghijklmnopqrstuvwxyz123456",
			WeChatAppID: "wx", WeChatSecret: "secret",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected short JWT secret to be rejected")
	}
}
