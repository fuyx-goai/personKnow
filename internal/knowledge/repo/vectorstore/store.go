// store.go —— 向量库统一接口
//
// Eino 官方扩展（eino-ext）里的向量库都要连外部服务（Milvus/Redis/ES...），
// 对新手不友好。其实"向量库"的本质只有两件事：
//  1. 存：把每个小段连同它的向量存起来          —— 实现 indexer.Indexer 接口
//  2. 查：把用户问题也变成向量，算"相似度"排序   —— 实现 retriever.Retriever 接口
//
// 本包提供两个实现：内存版（mem.go）与 Milvus 版（milvus.go），
// 它们共用下面的 VectorStore 接口；再通过 repository.go 的适配器
// 暴露为领域层的 model.Repository 端口，供上层使用。
package vectorstore

import (
	"context"

	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

// defaultListLimit 浏览接口的默认返回上限（调用方不指定 limit 时生效）
const defaultListLimit = 500

// VectorStore 向量库统一接口：既能"存"（indexer.Indexer），又能"查"（retriever.Retriever）
type VectorStore interface {
	indexer.Indexer
	retriever.Retriever
	// Count 已入库的片段数量（用于 /api/stats 展示）
	Count(ctx context.Context) (int, error)
	// List 列出已入库片段（供"藏书"页浏览），limit <= 0 时取 defaultListLimit
	List(ctx context.Context, limit int) ([]*schema.Document, error)
	// DeleteBySource 按来源文件名删除片段，返回删除条数；source 为空表示清空整库
	DeleteBySource(ctx context.Context, source string) (int, error)
}
