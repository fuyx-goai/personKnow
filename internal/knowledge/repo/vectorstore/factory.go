// factory.go —— 向量库工厂：按配置挑一个实现
//
// 上层代码（摄入链 / 问答链 / HTTP 层）只依赖 model.Repository 接口，
// 到底用内存版还是 Milvus，由这里的 switch 决定。
// 这就是"面向接口编程"最直接的收益：换向量库不改业务代码。
package vectorstore

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/embedding"

	"knowledge-base/pkg/config"
)

// NewVectorStore 根据 cfg.VectorStore 创建向量库实例
func NewVectorStore(ctx context.Context, cfg config.Config, emb embedding.Embedder) (VectorStore, error) {
	switch cfg.VectorStore {
	case config.StoreMilvus:
		return NewMilvusStore(ctx, cfg.Milvus, emb)
	case config.StoreMem, "":
		return NewMemStore(ctx, emb)
	default:
		return nil, fmt.Errorf("未知的向量库类型 %q，可选值：%s、%s",
			cfg.VectorStore, config.StoreMem, config.StoreMilvus)
	}
}
