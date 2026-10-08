package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDatabaseMaxConns  = 10
	defaultMaxFileBytes      = 50 * 1024 * 1024
	defaultWorkerConcurrency = 2
	defaultLogMaxSizeMB      = 100
	defaultLogRetainDays     = 30
)

type DatabaseConfig struct {
	URL      string `yaml:"url"`
	MaxConns int32  `yaml:"max_conns"`
}

type AuthConfig struct {
	JWTSecret    string        `yaml:"jwt_secret"`
	AccessTTL    time.Duration `yaml:"-"`
	RefreshTTL   time.Duration `yaml:"-"`
	WeChatAppID  string        `yaml:"wechat_app_id"`
	WeChatSecret string        `yaml:"wechat_app_secret"`
}

type StorageConfig struct {
	DataDir      string `yaml:"data_dir"`
	MaxFileBytes int64  `yaml:"max_file_bytes"`
}

type WorkerConfig struct {
	Concurrency int           `yaml:"concurrency"`
	Lease       time.Duration `yaml:"-"`
	Poll        time.Duration `yaml:"-"`
}

type LogConfig struct {
	Dir        string `yaml:"dir"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	RetainDays int    `yaml:"retain_days"`
}

func (c *Config) applyRuntimeDefaults() {
	c.Database.URL = envOr("DATABASE_URL", c.Database.URL)
	if c.Database.MaxConns <= 0 {
		c.Database.MaxConns = defaultDatabaseMaxConns
	}

	c.Auth.JWTSecret = envOr("JWT_SECRET", c.Auth.JWTSecret)
	c.Auth.WeChatAppID = envOr("WECHAT_APP_ID", c.Auth.WeChatAppID)
	c.Auth.WeChatSecret = envOr("WECHAT_APP_SECRET", c.Auth.WeChatSecret)
	c.Auth.AccessTTL = durationEnv("ACCESS_TOKEN_TTL", 15*time.Minute)
	c.Auth.RefreshTTL = durationEnv("REFRESH_TOKEN_TTL", 30*24*time.Hour)

	c.Storage.DataDir = envOr("DATA_DIR", pick(c.Storage.DataDir, "data"))
	if c.Storage.MaxFileBytes <= 0 {
		c.Storage.MaxFileBytes = defaultMaxFileBytes
	}

	if c.Worker.Concurrency <= 0 {
		c.Worker.Concurrency = defaultWorkerConcurrency
	}
	c.Worker.Lease = durationEnv("WORKER_LEASE", 5*time.Minute)
	c.Worker.Poll = durationEnv("WORKER_POLL", time.Second)

	c.Log.Dir = pick(c.Log.Dir, "logs")
	if c.Log.MaxSizeMB <= 0 {
		c.Log.MaxSizeMB = defaultLogMaxSizeMB
	}
	if c.Log.RetainDays <= 0 {
		c.Log.RetainDays = defaultLogRetainDays
	}

	c.LLM.APIKey = envOr("LLM_API_KEY", c.LLM.APIKey)
	c.Milvus.Password = envOr("MILVUS_PASSWORD", c.Milvus.Password)
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Database.URL) == "" {
		return fmt.Errorf("DATABASE_URL 不能为空")
	}
	if len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET 至少需要 32 个字符")
	}
	if c.Auth.WeChatAppID == "" || c.Auth.WeChatSecret == "" {
		return fmt.Errorf("微信 AppID 和密钥不能为空")
	}
	return nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func intEnv(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
