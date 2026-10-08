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

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"knowledge-base/internal/knowledge/model"
)

// Repository 把 VectorStore 适配成领域层的 model.Repository
type Repository struct {
	store VectorStore
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
	docs, err := r.store.Retrieve(ctx, query, retriever.WithTopK(topK))
	if err != nil {
		return nil, err
	}
	out := make([]model.Chunk, 0, len(docs))
	for _, d := range docs {
		out = append(out, model.Chunk{
			ID:       d.ID,
			Content:  d.Content,
			Metadata: d.MetaData,
			Score:    d.Score(),
		})
	}
	return out, nil
}

// Count 直接透传给向量库
func (r *Repository) Count(ctx context.Context) (int, error) {
	return r.store.Count(ctx)
}

// List 列出片段：Eino 文档 -> 领域片段
//
// 浏览场景不需要向量，这里刻意不填 Chunk.Vector，省内存也省带宽。
func (r *Repository) List(ctx context.Context, limit int) ([]model.Chunk, error) {
	docs, err := r.store.List(ctx, limit)
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
	}
	return out, nil
}

// DeleteBySource 按来源删除（source 为空即清空整库），直接透传给向量库
func (r *Repository) DeleteBySource(ctx context.Context, source string) (int, error) {
	return r.store.DeleteBySource(ctx, source)
}
