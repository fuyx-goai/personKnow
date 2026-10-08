package document

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrFileTooLarge      = errors.New("文件超过大小限制")
	ErrUnsupportedFormat = errors.New("不支持的文件格式")
	ErrInvalidFile       = errors.New("文件内容与格式不匹配")
	ErrDocumentNotFound  = errors.New("文件不存在")
	ErrDocumentReadOnly  = errors.New("公开知识库文件仅支持只读访问")
)

type Format string

const (
	FormatPDF      Format = "pdf"
	FormatDOCX     Format = "docx"
	FormatMarkdown Format = "md"
	FormatText     Format = "txt"
	FormatHTML     Format = "html"
	FormatCSV      Format = "csv"
	FormatPPTX     Format = "pptx"
)

type Status string

const (
	StatusQueued       Status = "queued"
	StatusProcessing   Status = "processing"
	StatusReady        Status = "ready"
	StatusFailed       Status = "failed"
	StatusDeleting     Status = "deleting"
	StatusDeleteFailed Status = "delete_failed"
)

type Location struct {
	UserID     uuid.UUID
	LibraryID  uuid.UUID
	DocumentID uuid.UUID
}

type StoredFile struct {
	RelativePath string
	Size         int64
	SHA256       string
	Extension    string
}

type ParsedContent struct {
	Text         string         `json:"text"`
	StructureMap map[string]any `json:"structure_map"`
}

type Document struct {
	ID                     uuid.UUID  `json:"id"`
	LibraryID              uuid.UUID  `json:"library_id"`
	UploadedBy             uuid.UUID  `json:"uploaded_by"`
	OriginalName           string     `json:"original_name"`
	DisplayName            string     `json:"display_name"`
	Format                 Format     `json:"format"`
	MIMEType               string     `json:"mime_type"`
	OriginalBytes          int64      `json:"original_bytes"`
	OriginalSHA256         string     `json:"-"`
	OriginalPath           string     `json:"-"`
	Status                 Status     `json:"status"`
	Summary                string     `json:"summary"`
	Tags                   []string   `json:"tags"`
	ActiveContentVersionID *uuid.UUID `json:"active_content_version_id,omitempty"`
	ChunkCount             int        `json:"chunk_count"`
	IndexedTokens          int64      `json:"indexed_tokens"`
	LastIndexedAt          *time.Time `json:"last_indexed_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type ContentVersion struct {
	ID            uuid.UUID      `json:"id"`
	DocumentID    uuid.UUID      `json:"document_id"`
	Version       int            `json:"version"`
	SourceType    string         `json:"source_type"`
	ContentPath   string         `json:"-"`
	ContentHash   string         `json:"content_hash"`
	TextBytes     int64          `json:"text_bytes"`
	TokenEstimate int64          `json:"token_estimate"`
	StructureMap  map[string]any `json:"structure_map"`
	CreatedBy     uuid.UUID      `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
}
