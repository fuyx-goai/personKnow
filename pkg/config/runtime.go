package config

import (
	"fmt"
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

// DatabaseConfig 定义 PostgreSQL 连接串和连接池上限。
type DatabaseConfig struct {
	URL      string `yaml:"url"`
	MaxConns int32  `yaml:"max_conns"`
}

// AuthConfig 集中保存账号认证参数；生产环境应把私有 YAML 权限限制为仅服务账号可读。
type AuthConfig struct {
	JWTSecret      string        `yaml:"jwt_secret"`
	IdentitySecret string        `yaml:"identity_secret"`
	AccessTTL      time.Duration `yaml:"-"`
	RefreshTTL     time.Duration `yaml:"-"`
	AccessTTLText  string        `yaml:"access_ttl"`
	RefreshTTLText string        `yaml:"refresh_ttl"`
	WeChatAppID    string        `yaml:"wechat_app_id"`
	WeChatSecret   string        `yaml:"wechat_app_secret"`
}

// StorageConfig 定义原文件落盘目录和单文件大小上限。
type StorageConfig struct {
	DataDir      string `yaml:"data_dir"`
	MaxFileBytes int64  `yaml:"max_file_bytes"`
}

// WorkerConfig 控制异步索引任务的并发、租约和轮询周期。
type WorkerConfig struct {
	Concurrency int           `yaml:"concurrency"`
	Lease       time.Duration `yaml:"-"`
	Poll        time.Duration `yaml:"-"`
	LeaseText   string        `yaml:"lease"`
	PollText    string        `yaml:"poll"`
}

// LogConfig 定义结构化日志的目录、滚动阈值和保留周期。
type LogConfig struct {
	Dir        string `yaml:"dir"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	RetainDays int    `yaml:"retain_days"`
}

// applyRuntimeDefaults 将 YAML 字段转换成运行时类型，并补齐非敏感默认值。
// 除 CONFIG_FILE 仅负责选择文件外，业务配置不再被环境变量隐式覆盖。
func (c *Config) applyRuntimeDefaults() {
	c.Database.URL = strings.TrimSpace(c.Database.URL)
	if c.Database.MaxConns <= 0 {
		c.Database.MaxConns = defaultDatabaseMaxConns
	}

	c.Auth.JWTSecret = strings.TrimSpace(c.Auth.JWTSecret)
	c.Auth.IdentitySecret = strings.TrimSpace(c.Auth.IdentitySecret)
	c.Auth.WeChatAppID = strings.TrimSpace(c.Auth.WeChatAppID)
	c.Auth.WeChatSecret = strings.TrimSpace(c.Auth.WeChatSecret)
	c.Auth.AccessTTL = parseDuration(c.Auth.AccessTTLText, 15*time.Minute)
	c.Auth.RefreshTTL = parseDuration(c.Auth.RefreshTTLText, 30*24*time.Hour)

	c.Storage.DataDir = pick(c.Storage.DataDir, "data")
	if c.Storage.MaxFileBytes <= 0 {
		c.Storage.MaxFileBytes = defaultMaxFileBytes
	}

	if c.Worker.Concurrency <= 0 {
		c.Worker.Concurrency = defaultWorkerConcurrency
	}
	c.Worker.Lease = parseDuration(c.Worker.LeaseText, 5*time.Minute)
	c.Worker.Poll = parseDuration(c.Worker.PollText, time.Second)

	c.Log.Dir = pick(c.Log.Dir, "logs")
	if c.Log.MaxSizeMB <= 0 {
		c.Log.MaxSizeMB = defaultLogMaxSizeMB
	}
	if c.Log.RetainDays <= 0 {
		c.Log.RetainDays = defaultLogRetainDays
	}

}

// Validate 在创建外部连接前检查启动所需的关键配置，尽早返回可读错误。
func (c Config) Validate() error {
	if c.VectorStore != StoreMem && c.VectorStore != StoreMilvus {
		return fmt.Errorf("vector_store 仅支持 %q 或 %q", StoreMem, StoreMilvus)
	}
	if strings.TrimSpace(c.LLM.APIKey) == "" {
		return fmt.Errorf("llm.api_key 不能为空")
	}
	if !c.DatabaseEnabled() {
		return nil
	}
	if len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("auth.jwt_secret 至少需要 32 个字符")
	}
	if len(c.Auth.IdentitySecret) < 32 {
		return fmt.Errorf("auth.identity_secret 至少需要 32 个字符")
	}
	if c.Auth.WeChatAppID == "" || c.Auth.WeChatSecret == "" {
		return fmt.Errorf("auth.wechat_app_id 和 auth.wechat_app_secret 不能为空")
	}
	return nil
}

// DatabaseEnabled 表示是否启用依赖 PostgreSQL 的账号、多知识库和审计能力。
func (c Config) DatabaseEnabled() bool {
	return strings.TrimSpace(c.Database.URL) != ""
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
