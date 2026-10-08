package chat

import (
	"errors"
	"time"

	"github.com/google/uuid"

	usage "knowledge-base/internal/usage"
)

var (
	ErrSessionNotFound    = errors.New("问答会话不存在")
	ErrInvalidScope       = errors.New("问答范围无效")
	ErrHistoryUnavailable = errors.New("会话历史服务不可用")
)

type ScopeType string

const (
	ScopeSingleLibrary ScopeType = "single_library"
	ScopeGlobal        ScopeType = "global"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type MessageStatus string

const (
	MessageStreaming   MessageStatus = "streaming"
	MessageComplete    MessageStatus = "complete"
	MessageInterrupted MessageStatus = "interrupted"
	MessageFailed      MessageStatus = "failed"
)

type Session struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	ScopeType ScopeType  `json:"scope_type"`
	LibraryID *uuid.UUID `json:"library_id,omitempty"`
	Title     string     `json:"title"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type Message struct {
	ID           uuid.UUID     `json:"id"`
	SessionID    uuid.UUID     `json:"session_id"`
	Role         Role          `json:"role"`
	Content      string        `json:"content"`
	Status       MessageStatus `json:"status"`
	InputTokens  int64         `json:"input_tokens"`
	OutputTokens int64         `json:"output_tokens"`
	TotalTokens  int64         `json:"total_tokens"`
	RequestID    string        `json:"request_id,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	References   []Reference   `json:"references,omitempty"`
}

type Reference struct {
	DocumentID       uuid.UUID `json:"document_id"`
	ContentVersionID uuid.UUID `json:"content_version_id"`
	ChunkID          string    `json:"chunk_id"`
	SourceName       string    `json:"source_name"`
	LocationLabel    string    `json:"location_label"`
	Excerpt          string    `json:"excerpt"`
	Similarity       float64   `json:"similarity"`
	Rank             int       `json:"rank"`
}

type RetrievalSettings struct {
	TopK                int
	SimilarityThreshold float64
}

type SessionFilter struct {
	Before *time.Time
	Limit  int
}

type CreateSessionCommand struct {
	ScopeType ScopeType
	LibraryID *uuid.UUID
	Title     string
}

type EngineResult struct {
	References []Reference
	Usage      usage.Breakdown
}

type EventType string

const (
	EventDelta     EventType = "delta"
	EventReference EventType = "reference"
	EventUsage     EventType = "usage"
	EventError     EventType = "error"
	EventDone      EventType = "done"
)

type Event struct {
	Type EventType `json:"type"`
	Data any       `json:"data,omitempty"`
}
