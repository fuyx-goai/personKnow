package library

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound           = errors.New("知识库不存在")
	ErrForbidden          = errors.New("无权执行此操作")
	ErrInvalidLibrary     = errors.New("知识库参数无效")
	ErrInvalidSettings    = errors.New("检索设置无效")
	ErrReindexUnavailable = errors.New("重新索引服务暂不可用")
)

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
)

type Status string

const (
	StatusActive       Status = "active"
	StatusDeleting     Status = "deleting"
	StatusDeleteFailed Status = "delete_failed"
)

type Access string

const (
	AccessNone  Access = "none"
	AccessRead  Access = "read"
	AccessOwner Access = "owner"
)

type Library struct {
	ID            uuid.UUID  `json:"id"`
	OwnerUserID   uuid.UUID  `json:"owner_user_id"`
	OwnerNickname string     `json:"owner_nickname,omitempty"`
	Name          string     `json:"name"`
	Category      string     `json:"category"`
	Description   string     `json:"description"`
	Visibility    Visibility `json:"visibility"`
	Status        Status     `json:"status"`
	DocumentCount int        `json:"document_count"`
	ChunkCount    int64      `json:"chunk_count"`
	StorageBytes  int64      `json:"storage_bytes"`
	IndexedTokens int64      `json:"indexed_tokens"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	Access        Access     `json:"access"`
}

type RetrievalSettings struct {
	LibraryID           uuid.UUID `json:"library_id"`
	ChunkSize           int       `json:"chunk_size"`
	ChunkOverlap        int       `json:"chunk_overlap"`
	TopK                int       `json:"top_k"`
	SimilarityThreshold float64   `json:"similarity_threshold"`
	UpdatedBy           uuid.UUID `json:"-"`
}

func DefaultRetrievalSettings() RetrievalSettings {
	return RetrievalSettings{ChunkSize: 800, ChunkOverlap: 100, TopK: 5, SimilarityThreshold: 0.30}
}

func (settings RetrievalSettings) Validate() error {
	if settings.ChunkSize < 200 || settings.ChunkSize > 2000 {
		return fmt.Errorf("%w: chunk_size 必须在 200 到 2000 之间", ErrInvalidSettings)
	}
	if settings.ChunkOverlap < 0 || settings.ChunkOverlap > 500 || settings.ChunkOverlap >= settings.ChunkSize {
		return fmt.Errorf("%w: chunk_overlap 必须小于 chunk_size 且不超过 500", ErrInvalidSettings)
	}
	if settings.TopK < 1 || settings.TopK > 20 {
		return fmt.Errorf("%w: top_k 必须在 1 到 20 之间", ErrInvalidSettings)
	}
	if settings.SimilarityThreshold < 0 || settings.SimilarityThreshold > 1 {
		return fmt.Errorf("%w: similarity_threshold 必须在 0 到 1 之间", ErrInvalidSettings)
	}
	return nil
}

func (library Library) Validate() error {
	name := strings.TrimSpace(library.Name)
	if name == "" || len([]rune(name)) > 120 {
		return fmt.Errorf("%w: 名称不能为空且不能超过 120 个字符", ErrInvalidLibrary)
	}
	if library.Visibility != VisibilityPrivate && library.Visibility != VisibilityPublic {
		return fmt.Errorf("%w: visibility 无效", ErrInvalidLibrary)
	}
	return nil
}

type ListFilter struct {
	Keyword  string
	Category string
	Limit    int
	Cursor   string
}

type ListResult struct {
	Owned  []Library `json:"owned"`
	Public []Library `json:"public"`
}

type CreateCommand struct {
	Name        string
	Category    string
	Description string
	Visibility  Visibility
}

type UpdateCommand struct {
	Name        string
	Category    string
	Description string
	Visibility  Visibility
}

type SettingsUpdateResult struct {
	Settings      RetrievalSettings `json:"settings"`
	RequiresIndex bool              `json:"requires_reindex"`
}
