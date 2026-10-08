package usage

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidAmount        = errors.New("用量必须大于零")
	ErrInvalidBreakdown     = errors.New("Token 用量不能为负数")
	ErrTokenQuotaExceeded   = errors.New("本月 Token 用量已达上限")
	ErrStorageQuotaExceeded = errors.New("存储空间已达上限")
	ErrStorageUnderflow     = errors.New("存储释放量超过当前用量")
	ErrReservationMismatch  = errors.New("Token 预留状态无效")
	ErrInvalidAuditMetadata = errors.New("审计字段包含不允许的内容")
)

type UsageType string

const (
	UsageStorage   UsageType = "storage"
	UsageEmbedding UsageType = "embedding"
	UsageInput     UsageType = "input"
	UsageOutput    UsageType = "output"
)

type Limits struct {
	StorageBytes  int64 `json:"storage_quota_bytes"`
	MonthlyTokens int64 `json:"monthly_token_quota"`
}

type MonthlyUsage struct {
	StorageBytes    int64 `json:"storage_bytes"`
	EmbeddingTokens int64 `json:"embedding_tokens"`
	InputTokens     int64 `json:"input_tokens"`
	OutputTokens    int64 `json:"output_tokens"`
	TotalTokens     int64 `json:"total_tokens"`
}

type Breakdown struct {
	Embedding int64 `json:"embedding_tokens"`
	Input     int64 `json:"input_tokens"`
	Output    int64 `json:"output_tokens"`
}

func (breakdown Breakdown) Total() int64 {
	return breakdown.Embedding + breakdown.Input + breakdown.Output
}

func (breakdown Breakdown) Validate() error {
	if breakdown.Embedding < 0 || breakdown.Input < 0 || breakdown.Output < 0 {
		return ErrInvalidBreakdown
	}
	return nil
}

type Reservation struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	Estimated  int64     `json:"estimated_tokens"`
	MonthStart time.Time `json:"month_start"`
	CreatedAt  time.Time `json:"created_at"`
}

type Summary struct {
	MonthStart       time.Time `json:"month_start"`
	StorageBytes     int64     `json:"storage_bytes"`
	StorageQuota     int64     `json:"storage_quota_bytes"`
	StorageRemaining int64     `json:"storage_remaining_bytes"`
	EmbeddingTokens  int64     `json:"embedding_tokens"`
	InputTokens      int64     `json:"input_tokens"`
	OutputTokens     int64     `json:"output_tokens"`
	TotalTokens      int64     `json:"total_tokens"`
	TokenQuota       int64     `json:"monthly_token_quota"`
	TokenRemaining   int64     `json:"token_remaining"`
}

type Record struct {
	ID           uuid.UUID      `json:"id"`
	UserID       uuid.UUID      `json:"user_id"`
	Type         UsageType      `json:"usage_type"`
	Quantity     int64          `json:"quantity"`
	Unit         string         `json:"unit"`
	ResourceType string         `json:"resource_type"`
	ResourceID   uuid.UUID      `json:"resource_id"`
	OccurredAt   time.Time      `json:"occurred_at"`
	Metadata     map[string]any `json:"metadata"`
}

type RecordFilter struct {
	Type   UsageType
	Before *time.Time
	Limit  int
}

type AuditResult string

const (
	AuditSuccess AuditResult = "success"
	AuditFailure AuditResult = "failure"
)

type AuditCommand struct {
	ActorUserID  *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	Result       AuditResult
	RequestID    string
	ClientType   string
	IPHash       string
	Metadata     map[string]any
}

type AuditEntry struct {
	ID           int64          `json:"id"`
	ActorUserID  *uuid.UUID     `json:"actor_user_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   *uuid.UUID     `json:"resource_id,omitempty"`
	Result       AuditResult    `json:"result"`
	RequestID    string         `json:"request_id,omitempty"`
	ClientType   string         `json:"client_type,omitempty"`
	IPHash       string         `json:"-"`
	Metadata     map[string]any `json:"metadata"`
	OccurredAt   time.Time      `json:"occurred_at"`
}

type AuditFilter struct {
	Action       string
	ResourceType string
	Before       *time.Time
	Limit        int
}
