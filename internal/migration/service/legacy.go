package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	document "knowledge-base/internal/document/entity"
	. "knowledge-base/internal/migration/entity"
)

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
		addFailure(report, source, "content_write_failed", "写入内容版本失败")
		return
	}
	item.ContentPath, item.ContentHash, item.OriginalBytes = stored.RelativePath, stored.SHA256, stored.Size
	state, err := migrator.repository.PrepareDocument(ctx, item)
	if err != nil {
		addFailure(report, source, "document_write_failed", "写入文档元数据失败")
		return
	}
	if state.Ready {
		return
	}
	if err := migrator.vectors.Upsert(ctx, chunks); err != nil {
		addFailure(report, source, "vector_write_failed", "写入向量数据失败")
		return
	}
	completed, err := migrator.repository.CompleteDocument(ctx, item.DocumentID, len(chunks))
	if err != nil {
		addFailure(report, source, "document_complete_failed", "更新文档迁移状态失败")
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
