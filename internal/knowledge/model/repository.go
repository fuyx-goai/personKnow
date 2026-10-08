// repository.go —— 领域模型层：向量库端口（Repository Port）
//
// 领域模型层只"声明"需要什么能力，不关心谁来"实现"。
// 依赖倒置：上层（service 层）依赖本接口，
// 具体是内存版还是 Milvus 版，由 repo 层去实现。
package model

import "context"

// Repository 向量库端口
type Repository interface {
	// Save 保存已向量化的片段，返回入库 ID 列表
	Save(ctx context.Context, chunks []Chunk) ([]string, error)
	// Search 语义检索：按 query 找最相近的 topK 个片段
	Search(ctx context.Context, query string, topK int) ([]Chunk, error)
	// Count 已入库的片段数量
	Count(ctx context.Context) (int, error)
	// List 列出已入库的片段（供"藏书"页浏览），limit <= 0 时由实现给默认上限
	List(ctx context.Context, limit int) ([]Chunk, error)
	// DeleteBySource 按来源文件名删除片段，返回删除条数；
	// source 传空字符串表示清空整库
	DeleteBySource(ctx context.Context, source string) (int, error)
}
