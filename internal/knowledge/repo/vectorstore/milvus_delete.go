package vectorstore

import (
	"context"
	"fmt"
)

func (s *MilvusStore) DeleteBySource(ctx context.Context, source string) (int, error) {
	documents, err := s.List(ctx, milvusDeleteScanLimit)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(documents))
	for _, document := range documents {
		if document.ID != "" && (source == "" || sourceOf(document.MetaData) == source) {
			ids = append(ids, document.ID)
		}
	}
	return s.DeleteByIDs(ctx, ids)
}

func (s *MilvusStore) DeleteByIDs(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var result struct {
		DeleteCount int `json:"deleteCount"`
	}
	if err := s.post(ctx, "/v2/vectordb/entities/delete", map[string]any{
		"dbName": s.dbName, "collectionName": s.collection, "ids": ids,
	}, &result); err != nil {
		return 0, fmt.Errorf("删除 Milvus 片段失败: %w", err)
	}
	return result.DeleteCount, nil
}
