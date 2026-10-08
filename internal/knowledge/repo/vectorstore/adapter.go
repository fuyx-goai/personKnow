// adapter.go —— 领域端口适配器（Adapter）
//
// 向量库本质上是 Eino 的 indexer.Indexer / retriever.Retriever，参数用的是
// Eino 的 schema.Document；而领域层只认识知识片段 model.Chunk。
//
// 这一层适配器就负责"翻译"：
//
//	上层调用 model.Repository（领域语言）  <- 本适配器 ->  VectorStore（Eino 语言）
//
// 这样应用层 / Eino 编排层不用了解 schema.Document，向量库也不用了解领域模型。
package vectorstore

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"knowledge-base/internal/knowledge/model"
	platformvector "knowledge-base/internal/platform/vector"
)

// Repository 把 VectorStore 适配成领域层的 model.Repository
type Repository struct {
	store VectorStore
}

type ScopedRepository struct {
	store VectorStore
}

var _ platformvector.Repository = (*ScopedRepository)(nil)

func NewScopedRepository(store VectorStore) *ScopedRepository {
	return &ScopedRepository{store: store}
}

func (repository *ScopedRepository) Upsert(ctx context.Context, chunks []platformvector.Chunk) error {
	documents := make([]*schema.Document, 0, len(chunks))
	for _, chunk := range chunks {
		metadata := map[string]any{
			platformvector.MetaOwnerUserID: chunk.OwnerUserID.String(), platformvector.MetaLibraryID: chunk.LibraryID.String(),
			platformvector.MetaDocumentID: chunk.DocumentID.String(), platformvector.MetaContentVersion: chunk.ContentVersionID.String(),
			platformvector.MetaChunkIndex: chunk.ChunkIndex, platformvector.MetaSourceName: chunk.SourceName,
			platformvector.MetaLocationLabel: chunk.LocationLabel, platformvector.MetaVisibility: chunk.Visibility,
		}
		document := &schema.Document{ID: chunk.ID, Content: chunk.Content, MetaData: metadata}
		document.WithDenseVector(chunk.Vector)
		documents = append(documents, document)
	}
	_, err := repository.store.Store(ctx, documents)
	return err
}

func (repository *ScopedRepository) Search(ctx context.Context, query string, scope platformvector.SearchScope, topK int) ([]platformvector.Chunk, error) {
	count, err := repository.store.Count(ctx)
	if err != nil || count == 0 {
		return nil, err
	}
	documents, err := repository.store.Retrieve(ctx, query, retriever.WithTopK(count))
	if err != nil {
		return nil, err
	}
	return scopedChunks(documents, scope, topK), nil
}

func (repository *ScopedRepository) DeleteVersion(ctx context.Context, versionID uuid.UUID) (int, error) {
	documents, err := repository.all(ctx)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0)
	for _, document := range documents {
		if metadataUUID(document.MetaData, platformvector.MetaContentVersion) == versionID {
			ids = append(ids, document.ID)
		}
	}
	return repository.store.DeleteByIDs(ctx, ids)
}

func (repository *ScopedRepository) Count(ctx context.Context, scope platformvector.SearchScope) (int, error) {
	documents, err := repository.all(ctx)
	if err != nil {
		return 0, err
	}
	return len(scopedChunks(documents, scope, 0)), nil
}

func (repository *ScopedRepository) all(ctx context.Context) ([]*schema.Document, error) {
	count, err := repository.store.Count(ctx)
	if err != nil || count == 0 {
		return nil, err
	}
	return repository.store.List(ctx, count)
}

func scopedChunks(documents []*schema.Document, scope platformvector.SearchScope, limit int) []platformvector.Chunk {
	libraries := make(map[uuid.UUID]struct{}, len(scope.LibraryIDs))
	for _, libraryID := range scope.LibraryIDs {
		libraries[libraryID] = struct{}{}
	}
	chunks := make([]platformvector.Chunk, 0, len(documents))
	for _, document := range documents {
		chunk, err := decodeChunk(document)
		if err != nil || !scopeAllows(chunk, scope, libraries) {
			continue
		}
		chunks = append(chunks, chunk)
		if limit > 0 && len(chunks) == limit {
			break
		}
	}
	return chunks
}

func scopeAllows(chunk platformvector.Chunk, scope platformvector.SearchScope, libraries map[uuid.UUID]struct{}) bool {
	if _, allowed := libraries[chunk.LibraryID]; !allowed || chunk.Similarity < scope.MinSimilarity {
		return false
	}
	if len(scope.ActiveVersions) == 0 {
		return true
	}
	active, exists := scope.ActiveVersions[chunk.DocumentID]
	return exists && active == chunk.ContentVersionID
}

func decodeChunk(document *schema.Document) (platformvector.Chunk, error) {
	chunk := platformvector.Chunk{
		ID: document.ID, Content: document.Content, Similarity: document.Score(),
		OwnerUserID:      metadataUUID(document.MetaData, platformvector.MetaOwnerUserID),
		LibraryID:        metadataUUID(document.MetaData, platformvector.MetaLibraryID),
		DocumentID:       metadataUUID(document.MetaData, platformvector.MetaDocumentID),
		ContentVersionID: metadataUUID(document.MetaData, platformvector.MetaContentVersion),
		SourceName:       metadataString(document.MetaData, platformvector.MetaSourceName),
		LocationLabel:    metadataString(document.MetaData, platformvector.MetaLocationLabel),
		Visibility:       metadataString(document.MetaData, platformvector.MetaVisibility),
	}
	if chunk.LibraryID == uuid.Nil || chunk.DocumentID == uuid.Nil || chunk.ContentVersionID == uuid.Nil {
		return platformvector.Chunk{}, fmt.Errorf("向量元数据不完整")
	}
	switch value := document.MetaData[platformvector.MetaChunkIndex].(type) {
	case int:
		chunk.ChunkIndex = value
	case float64:
		chunk.ChunkIndex = int(value)
	}
	return chunk, nil
}

func metadataUUID(metadata map[string]any, key string) uuid.UUID {
	raw, _ := metadata[key].(string)
	parsed, _ := uuid.Parse(raw)
	return parsed
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}

// 编译期断言：本适配器必须满足领域端口
var _ model.Repository = (*Repository)(nil)

// NewRepository 用任意 VectorStore（内存版 / Milvus 版）构造领域端口
func NewRepository(store VectorStore) *Repository {
	return &Repository{store: store}
}

// Save 把领域片段转成 Eino 文档后入库
func (r *Repository) Save(ctx context.Context, chunks []model.Chunk) ([]string, error) {
	docs := make([]*schema.Document, 0, len(chunks))
	for _, c := range chunks {
		doc := &schema.Document{ID: c.ID, Content: c.Content, MetaData: c.Metadata}
		if len(c.Vector) > 0 {
			// Eino 约定：向量通过 WithDenseVector 携带，向量库从里面取
			doc.WithDenseVector(c.Vector)
		}
		docs = append(docs, doc)
	}
	return r.store.Store(ctx, docs)
}

// Search 语义检索，并把 Eino 文档转回领域片段
func (r *Repository) Search(ctx context.Context, query string, topK int) ([]model.Chunk, error) {
	count, err := r.store.Count(ctx)
	if err != nil || count == 0 {
		return nil, err
	}
	docs, err := r.store.Retrieve(ctx, query, retriever.WithTopK(count))
	if err != nil {
		return nil, err
	}
	out := make([]model.Chunk, 0, min(topK, len(docs)))
	for _, d := range docs {
		if !isLegacyDocument(d) {
			continue
		}
		out = append(out, model.Chunk{
			ID:       d.ID,
			Content:  d.Content,
			Metadata: d.MetaData,
			Score:    d.Score(),
		})
		if topK > 0 && len(out) == topK {
			break
		}
	}
	return out, nil
}

// Count 直接透传给向量库
func (r *Repository) Count(ctx context.Context) (int, error) {
	documents, err := r.legacyDocuments(ctx)
	return len(documents), err
}

// List 列出片段：Eino 文档 -> 领域片段
//
// 浏览场景不需要向量，这里刻意不填 Chunk.Vector，省内存也省带宽。
func (r *Repository) List(ctx context.Context, limit int) ([]model.Chunk, error) {
	docs, err := r.legacyDocuments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Chunk, 0, len(docs))
	for _, d := range docs {
		out = append(out, model.Chunk{
			ID:       d.ID,
			Content:  d.Content,
			Metadata: d.MetaData,
		})
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out, nil
}

// DeleteBySource 按来源删除（source 为空即清空整库），直接透传给向量库
func (r *Repository) DeleteBySource(ctx context.Context, source string) (int, error) {
	documents, err := r.legacyDocuments(ctx)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(documents))
	for _, document := range documents {
		if source == "" || sourceOf(document.MetaData) == source {
			ids = append(ids, document.ID)
		}
	}
	return r.store.DeleteByIDs(ctx, ids)
}

func (r *Repository) legacyDocuments(ctx context.Context) ([]*schema.Document, error) {
	count, err := r.store.Count(ctx)
	if err != nil || count == 0 {
		return nil, err
	}
	documents, err := r.store.List(ctx, count)
	if err != nil {
		return nil, err
	}
	legacy := make([]*schema.Document, 0, len(documents))
	for _, document := range documents {
		if isLegacyDocument(document) {
			legacy = append(legacy, document)
		}
	}
	return legacy, nil
}

func isLegacyDocument(document *schema.Document) bool {
	value, exists := document.MetaData[platformvector.MetaLibraryID]
	return !exists || value == nil || value == ""
}
