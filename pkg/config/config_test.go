package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadUsesEnvironmentSecrets(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("http_addr: :9090\nvector_store: mem\n")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", configPath)
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/knowledge")
	t.Setenv("JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("WECHAT_APP_ID", "wx-test")
	t.Setenv("WECHAT_APP_SECRET", "wechat-secret")
	t.Setenv("IDENTITY_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("DATA_DIR", filepath.Join(t.TempDir(), "data"))

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.URL != "postgres://user:pass@localhost/knowledge" {
		t.Fatalf("database URL not loaded from environment: %q", cfg.Database.URL)
	}
	if cfg.Auth.JWTSecret == "" || cfg.Auth.WeChatAppID != "wx-test" {
		t.Fatal("authentication secrets were not loaded from environment")
	}
	if cfg.Auth.IdentitySecret != "abcdefghijklmnopqrstuvwxyz123456" {
		t.Fatal("identity secret was not loaded from environment")
	}
	if cfg.Auth.AccessTTL != 15*time.Minute || cfg.Auth.RefreshTTL != 30*24*time.Hour {
		t.Fatalf("unexpected token TTLs: %s %s", cfg.Auth.AccessTTL, cfg.Auth.RefreshTTL)
	}
	if cfg.Storage.MaxFileBytes != 50*1024*1024 {
		t.Fatalf("unexpected max file size: %d", cfg.Storage.MaxFileBytes)
	}
}

func TestValidateRejectsShortJWTSecret(t *testing.T) {
	cfg := Config{
		Database: DatabaseConfig{URL: "postgres://localhost/db"},
		Auth:     AuthConfig{JWTSecret: "short", WeChatAppID: "wx", WeChatSecret: "secret"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected short JWT secret to be rejected")
	}
}
