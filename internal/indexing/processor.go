package indexing

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/google/uuid"

	librarydomain "knowledge-base/internal/library"
	platformvector "knowledge-base/internal/platform/vector"
	usage "knowledge-base/internal/usage"
)

type IndexSource struct {
	OwnerUserID      uuid.UUID
	LibraryID        uuid.UUID
	DocumentID       uuid.UUID
	ContentVersionID uuid.UUID
	Content          string
	SourceName       string
	Visibility       string
	Settings         librarydomain.RetrievalSettings
}

type SourceLoader interface {
	Load(context.Context, Job) (IndexSource, error)
}

type TokenQuota interface {
	ReserveTokens(context.Context, uuid.UUID, int64) (usage.Reservation, error)
	SettleTokens(context.Context, usage.Reservation, usage.Breakdown) error
}

type ProcessorDependencies struct {
	Sources  SourceLoader
	Embedder embedding.Embedder
	Vectors  platformvector.Repository
	Quota    TokenQuota
}

type IndexProcessor struct {
	sources  SourceLoader
	embedder embedding.Embedder
	vectors  platformvector.Repository
	quota    TokenQuota
}

func NewProcessor(dependencies ProcessorDependencies) *IndexProcessor {
	return &IndexProcessor{
		sources: dependencies.Sources, embedder: dependencies.Embedder,
		vectors: dependencies.Vectors, quota: dependencies.Quota,
	}
}

func (processor *IndexProcessor) Process(ctx context.Context, job Job, report ProgressReporter) (ProcessResult, error) {
	if err := report(ctx, "loading", 10); err != nil {
		return ProcessResult{}, err
	}
	source, err := processor.sources.Load(ctx, job)
	if err != nil {
		return ProcessResult{}, WrapProcessError("SOURCE_LOAD_FAILED", "读取文件内容失败", false, err)
	}
	if job.Type == JobDelete {
		return ProcessResult{ContentVersionID: source.ContentVersionID}, nil
	}
	texts := splitContent(source.Content, source.Settings.ChunkSize, source.Settings.ChunkOverlap)
	if len(texts) == 0 {
		return ProcessResult{}, WrapProcessError("EMPTY_CONTENT", "文件没有可索引内容", false, nil)
	}
	estimated := estimateTokens(texts)
	reservation, err := processor.quota.ReserveTokens(ctx, source.OwnerUserID, estimated)
	if err != nil {
		return ProcessResult{}, WrapProcessError("TOKEN_QUOTA_EXCEEDED", "本月 Token 用量已达上限", false, err)
	}
	if err := report(ctx, "embedding", 40); err != nil {
		_ = processor.quota.SettleTokens(ctx, reservation, usage.Breakdown{})
		return ProcessResult{}, err
	}
	vectors, err := processor.embedder.EmbedStrings(ctx, texts)
	if err != nil || len(vectors) != len(texts) {
		_ = processor.quota.SettleTokens(ctx, reservation, usage.Breakdown{})
		return ProcessResult{}, WrapProcessError("EMBEDDING_FAILED", "向量化失败", true, err)
	}
	chunks := makeChunks(source, texts, vectors)
	if err := report(ctx, "storing", 75); err != nil {
		_ = processor.quota.SettleTokens(ctx, reservation, usage.Breakdown{Embedding: estimated})
		return ProcessResult{}, err
	}
	if err := processor.vectors.Upsert(ctx, chunks); err != nil {
		_ = processor.quota.SettleTokens(ctx, reservation, usage.Breakdown{Embedding: estimated})
		return ProcessResult{}, WrapProcessError("VECTOR_STORE_FAILED", "保存向量失败", true, err)
	}
	if err := processor.quota.SettleTokens(ctx, reservation, usage.Breakdown{Embedding: estimated}); err != nil {
		return ProcessResult{}, WrapProcessError("USAGE_SETTLEMENT_FAILED", "用量结算失败", true, err)
	}
	if err := report(ctx, "finalizing", 95); err != nil {
		return ProcessResult{}, err
	}
	return ProcessResult{ContentVersionID: source.ContentVersionID, ChunkCount: len(chunks), IndexedTokens: estimated}, nil
}

func (processor *IndexProcessor) CleanupVersion(ctx context.Context, versionID uuid.UUID) error {
	_, err := processor.vectors.DeleteVersion(ctx, versionID)
	return err
}

func splitContent(content string, chunkSize, overlap int) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 800
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = 0
	}
	runes := []rune(content)
	step := chunkSize - overlap
	chunks := make([]string, 0, (len(runes)+step-1)/step)
	for start := 0; start < len(runes); start += step {
		end := min(start+chunkSize, len(runes))
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) {
			break
		}
	}
	return chunks
}

func estimateTokens(texts []string) int64 {
	var runes int
	for _, text := range texts {
		runes += len([]rune(text))
	}
	return int64(max(1, (runes+3)/4))
}

func makeChunks(source IndexSource, texts []string, vectors [][]float64) []platformvector.Chunk {
	chunks := make([]platformvector.Chunk, len(texts))
	for index, text := range texts {
		chunks[index] = platformvector.Chunk{
			ID:      platformvector.StableChunkID(source.DocumentID, source.ContentVersionID, index),
			Content: text, Vector: vectors[index], OwnerUserID: source.OwnerUserID,
			LibraryID: source.LibraryID, DocumentID: source.DocumentID, ContentVersionID: source.ContentVersionID,
			ChunkIndex: index, SourceName: source.SourceName, LocationLabel: fmt.Sprintf("片段 %d", index+1),
			Visibility: source.Visibility,
		}
	}
	return chunks
}
