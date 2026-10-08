// llm.go —— 初始化大模型组件：ChatModel（对话） + Embedder（向量化）
//
// 本项目对接【豆包（火山方舟 Ark）】：方舟提供 OpenAI 兼容接口，
// 所以直接用 eino-ext 的 openai 组件，把 BaseURL 指到方舟地址即可。
//
// Eino 的设计思想：所有能力都抽象成"组件接口"（定义在 eino 核心包），
// 各家厂商的实现放在 eino-ext 扩展包里，换平台只改配置不改代码。
//
// 这些组件都属于"外部依赖实现"，因此和 Eino 链、向量库一起放在 repo 目录下。
package repo

import (
	"context"
	"errors"
	"fmt"

	aembedding "github.com/cloudwego/eino-ext/components/embedding/openai"
	aopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/embedding"

	"knowledge-base/pkg/config"
)

// NewChatModel 创建对话模型组件
// 返回的 *aopenai.ChatModel 实现了 eino 核心的 model.BaseChatModel 接口，
// 所以可以直接挂到 Chain 上（支持 Invoke 一次性调用 / Stream 流式调用）
func NewChatModel(ctx context.Context, cfg config.LLMConfig) (*aopenai.ChatModel, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("未配置 API Key：请在 configs/config.yaml 的 llm.api_key 填入方舟 API Key")
	}
	return aopenai.NewChatModel(ctx, &aopenai.ChatModelConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.ChatModel,
	})
}

// NewEmbedder 创建向量化（Embedding）组件
// "向量化"就是把一段文字变成一串数字（向量），语义相近的文字向量距离也近，
// 这是"语义搜索"的基础
//
// 返回类型是 Eino 的接口 embedding.Embedder，而不是某个具体实现——
// 这样上层（摄入管道 / 检索）就不在乎底下是标准 OpenAI 接口还是方舟多模态接口。
//
// 两种接口的差别（由 configs/config.yaml 的 llm.embed_api 决定）：
//
//	openai        标准 OpenAI 兼容 /embeddings，配"纯文本向量模型"
//	              （doubao-embedding-large-text-* 等）
//	ark_multimodal 方舟多模态 /embeddings/multimodal，配"多模态向量模型"
//	              （doubao-embedding-vision-* 等。这类模型调 /embeddings 会报
//	               "does not support this api"）
func NewEmbedder(ctx context.Context, cfg config.LLMConfig) (embedding.Embedder, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("未配置 API Key：请在 configs/config.yaml 的 llm.api_key 填入方舟 API Key")
	}

	// 多模态向量模型走自研适配器（见 ark_embedding.go）
	if cfg.EmbedAPI == config.EmbedAPIArkMultimodal {
		return newArkEmbedder(cfg), nil
	}

	// 默认走 eino-ext 自带的标准 OpenAI 兼容实现
	emb, err := aembedding.NewEmbedder(ctx, &aembedding.EmbeddingConfig{
		APIKey: cfg.APIKey,
		// 注意：必须用平台提供的 Embedding 模型，拿对话模型去调向量接口会报错
		BaseURL: cfg.BaseURL,
		Model:   cfg.EmbedModel,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 Embedder 失败: %w", err)
	}
	return emb, nil
}
