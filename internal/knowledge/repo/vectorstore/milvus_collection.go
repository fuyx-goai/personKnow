package vectorstore

import (
	"context"
	"fmt"
)

func (s *MilvusStore) ensureCollection(ctx context.Context) error {
	var names []string
	if err := s.post(ctx, "/v2/vectordb/collections/list", map[string]any{"dbName": s.dbName}, &names); err != nil {
		return fmt.Errorf("连接 Milvus（%s）失败，请确认服务已启动: %w", s.baseURL, err)
	}
	for _, name := range names {
		if name == s.collection {
			return s.loadCollection(ctx)
		}
	}
	if err := s.post(ctx, "/v2/vectordb/collections/create", map[string]any{
		"dbName": s.dbName, "collectionName": s.collection, "dimension": s.dim, "metricType": s.metric,
		"primaryFieldName": milvusFieldID, "idType": "VarChar", "vectorFieldName": milvusFieldVector,
		"autoID": false, "enableDynamicField": true, "params": map[string]any{"max_length": milvusMaxIDLen},
		"consistencyLevel": "Strong",
	}, nil); err != nil {
		return fmt.Errorf("创建集合 %s 失败: %w", s.collection, err)
	}
	return s.loadCollection(ctx)
}

func (s *MilvusStore) loadCollection(ctx context.Context) error {
	if err := s.post(ctx, "/v2/vectordb/collections/load", map[string]any{
		"dbName": s.dbName, "collectionName": s.collection,
	}, nil); err != nil {
		return fmt.Errorf("加载集合 %s 失败: %w", s.collection, err)
	}
	return nil
}
