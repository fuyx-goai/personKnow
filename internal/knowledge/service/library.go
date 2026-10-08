// library.go —— 应用服务层：知识库用例（统计 / 浏览 / 清理）
//
// 对应前端"总览"和"藏书"两个页面。它和摄入、问答一样，只依赖
// model.Repository 端口，因此换内存库还是 Milvus，这里一行都不用改。
package service

import (
	"context"
	"strings"

	"knowledge-base/internal/knowledge/dto"
	"knowledge-base/internal/knowledge/model"
)

// LibraryService 知识库用例
type LibraryService struct {
	repo model.Repository
}

// NewLibraryService 注入仓储端口
func NewLibraryService(repo model.Repository) *LibraryService {
	return &LibraryService{repo: repo}
}

// Chunks 返回已入库的片段数量
func (s *LibraryService) Chunks(ctx context.Context) (int, error) {
	return s.repo.Count(ctx)
}

// List 列出片段（"藏书"页浏览用），limit <= 0 时由向量库取默认上限
func (s *LibraryService) List(ctx context.Context, limit int) ([]dto.ChunkView, error) {
	chunks, err := s.repo.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	return dto.NewChunkViews(chunks), nil
}

// Delete 删除片段：source 非空时只删这个来源，为空则清空整库；返回删除条数
func (s *LibraryService) Delete(ctx context.Context, source string) (int, error) {
	return s.repo.DeleteBySource(ctx, strings.TrimSpace(source))
}
