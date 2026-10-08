package migration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"knowledge-base/internal/document"
	platformvector "knowledge-base/internal/platform/vector"
)

var legacyNamespace = uuid.MustParse("c70563c5-8907-4e14-b398-49370e0cf88c")

type Repository interface {
	EnsureDefaultLibrary(context.Context, uuid.UUID) (uuid.UUID, error)
	PrepareDocument(context.Context, ImportDocument) (DocumentState, error)
	CompleteDocument(context.Context, uuid.UUID, int) (bool, error)
	RecordAudit(context.Context, AuditRecord) error
}

type VectorWriter interface {
	Upsert(context.Context, []platformvector.Chunk) error
}

type ContentWriter interface {
	WriteContent(document.Location, int, string) (document.StoredFile, error)
}

type Dependencies struct {
	Repository Repository
	Vectors    VectorWriter
	Files      ContentWriter
	Now        func() time.Time
}

type Options struct {
	TargetUser uuid.UUID
	SourcePath string
	BackupDir  string
}

type Report struct {
	BackupPath     string    `json:"backup_path"`
	DocumentsAdded int       `json:"documents_added"`
	ChunksAdded    int       `json:"chunks_added"`
	Failures       []Failure `json:"failures"`
}

type Failure struct {
	Source  string `json:"source"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ImportDocument struct {
	TargetUser       uuid.UUID
	LibraryID        uuid.UUID
	DocumentID       uuid.UUID
	ContentVersionID uuid.UUID
	SourceName       string
	DisplayName      string
	Extension        string
	OriginalHash     string
	OriginalBytes    int64
	ContentPath      string
	ContentHash      string
	ChunkCount       int
	CreatedAt        time.Time
}

type DocumentState struct {
	Created bool
	Ready   bool
}

type AuditRecord struct {
	TargetUser   uuid.UUID
	LibraryID    uuid.UUID
	Documents    int
	Chunks       int
	FailureCount int
	OccurredAt   time.Time
}

type LegacyMigrator struct {
	repository Repository
	vectors    VectorWriter
	files      ContentWriter
	now        func() time.Time
}

type legacyEntry struct {
	Content string         `json:"content"`
	Vector  []float64      `json:"vector"`
	Meta    map[string]any `json:"meta"`
}

func NewLegacyMigrator(dependencies Dependencies) *LegacyMigrator {
	now := dependencies.Now
	if now == nil {
		now = time.Now
	}
	return &LegacyMigrator{
		repository: dependencies.Repository, vectors: dependencies.Vectors,
		files: dependencies.Files, now: now,
	}
}

func (migrator *LegacyMigrator) Migrate(ctx context.Context, options Options) (Report, error) {
	if err := migrator.validate(options); err != nil {
		return Report{}, err
	}
	raw, backupPath, err := backupSource(options.SourcePath, options.BackupDir, migrator.now())
	if err != nil {
		return Report{}, err
	}
	report := Report{BackupPath: backupPath, Failures: []Failure{}}
	groups, failures, err := decodeLegacy(raw)
	if err != nil {
		return report, err
	}
	report.Failures = append(report.Failures, failures...)
	libraryID, err := migrator.repository.EnsureDefaultLibrary(ctx, options.TargetUser)
	if err != nil {
		return report, fmt.Errorf("创建默认知识库失败: %w", err)
	}
	for _, source := range sortedSources(groups) {
		migrator.importSource(ctx, options.TargetUser, libraryID, source, groups[source], &report)
	}
	audit := AuditRecord{TargetUser: options.TargetUser, LibraryID: libraryID,
		Documents: report.DocumentsAdded, Chunks: report.ChunksAdded,
		FailureCount: len(report.Failures), OccurredAt: migrator.now()}
	if err := migrator.repository.RecordAudit(ctx, audit); err != nil {
		return report, fmt.Errorf("记录迁移审计日志失败: %w", err)
	}
	return report, nil
}

func (migrator *LegacyMigrator) importSource(ctx context.Context, userID, libraryID uuid.UUID, source string, entries []legacyEntry, report *Report) {
	item, content, chunks := buildImport(userID, libraryID, source, entries, migrator.now())
	stored, err := migrator.files.WriteContent(document.Location{
		UserID: userID, LibraryID: libraryID, DocumentID: item.DocumentID,
	}, 1, content)
	if err != nil {
		report.addFailure(source, "content_write_failed", "写入内容版本失败")
		return
	}
	item.ContentPath, item.ContentHash, item.OriginalBytes = stored.RelativePath, stored.SHA256, stored.Size
	state, err := migrator.repository.PrepareDocument(ctx, item)
	if err != nil {
		report.addFailure(source, "document_write_failed", "写入文档元数据失败")
		return
	}
	if state.Ready {
		return
	}
	if err := migrator.vectors.Upsert(ctx, chunks); err != nil {
		report.addFailure(source, "vector_write_failed", "写入向量数据失败")
		return
	}
	completed, err := migrator.repository.CompleteDocument(ctx, item.DocumentID, len(chunks))
	if err != nil {
		report.addFailure(source, "document_complete_failed", "更新文档迁移状态失败")
		return
	}
	if state.Created {
		report.DocumentsAdded++
	}
	if completed {
		report.ChunksAdded += len(chunks)
	}
}

func (migrator *LegacyMigrator) validate(options Options) error {
	if migrator.repository == nil || migrator.vectors == nil || migrator.files == nil {
		return errors.New("迁移依赖不完整")
	}
	if options.TargetUser == uuid.Nil || strings.TrimSpace(options.SourcePath) == "" || strings.TrimSpace(options.BackupDir) == "" {
		return errors.New("target-user、source 和 backup-dir 均为必填项")
	}
	return nil
}
