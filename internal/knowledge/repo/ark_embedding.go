// ark_embedding.go —— 方舟"多模态向量"接口适配器
//
// 背景：eino-ext 自带的 openai Embedder 只会调标准的 /embeddings，
// 而豆包的"多模态向量模型"（doubao-embedding-vision-*）在方舟上挂在
// /embeddings/multimodal 这个端点下，且请求/响应规格与 OpenAI 不同：
//
//	请求：{"model":"...", "input":[{"type":"text","text":"..."}]}
//	响应：{"data":{"embedding":[...]}}   ← 一次请求只返回"一个"向量
//
// 所以这里手写一个适配器，把方舟的多模态接口"翻译"成 Eino 的
// embedding.Embedder 接口（EmbedStrings：一批文本 → 一批向量）。
//
// 这正是"依赖倒置"的价值：上层（摄入管道 / 检索）只认 Eino 的接口，
// 底层换一个厂商、换一种协议，只要补一个适配器即可，上层一行都不用改。
package repo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/embedding"

	"knowledge-base/pkg/config"
)

const (
	// arkMultimodalPath 方舟多模态向量接口路径（拼在 base_url 之后）
	// 最终地址形如：https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal
	arkMultimodalPath = "/embeddings/multimodal"

	// arkMultimodalTimeout 单次向量化请求的超时时间
	arkMultimodalTimeout = 60 * time.Second
)

// arkEmbedder 把方舟多模态向量接口适配成 Eino 的 embedding.Embedder
type arkEmbedder struct {
	apiKey  string
	baseURL string // 已去掉末尾斜杠，如 https://ark.cn-beijing.volces.com/api/v3
	model   string
	client  *http.Client
}

// 编译期断言：arkEmbedder 必须实现 Eino 的向量化接口
// （写错方法签名会在编译期立刻暴露，而不是等到跑起来才报错）
var _ embedding.Embedder = (*arkEmbedder)(nil)

// newArkEmbedder 创建方舟多模态向量化组件
func newArkEmbedder(cfg config.LLMConfig) *arkEmbedder {
	return &arkEmbedder{
		apiKey:  cfg.APIKey,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		model:   cfg.EmbedModel,
		client:  &http.Client{Timeout: arkMultimodalTimeout},
	}
}

// EmbedStrings 把一批文本变成一批向量（返回顺序与入参严格一一对应）
//
// 注意：方舟这个端点一次只吃"一个"输入、只吐"一个"向量，
// 所以这里只能逐条请求。文档量大时这会成为瓶颈，届时可改成
// 并发提交（配 errgroup + 结果按下标回填），此处为教学可读性保持串行。
func (e *arkEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	out := make([][]float64, len(texts))
	for i, text := range texts {
		vec, err := e.embedOne(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("第 %d/%d 条文本向量化失败: %w", i+1, len(texts), err)
		}
		out[i] = vec
	}
	return out, nil
}

// embedOne 向量化单条文本
func (e *arkEmbedder) embedOne(ctx context.Context, text string) ([]float64, error) {
	// 1. 组装请求体：input 是"内容块"数组，纯文本就是 [{type:text, text:...}]
	//    将来若要支持图片，往数组里塞 {"type":"image_url",...} 即可，接口不用变
	payload := map[string]any{
		"model": e.model,
		"input": []map[string]string{{"type": "text", "text": text}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// 2. 发请求（带 60 秒超时，避免网络卡死拖住整个摄入流程）
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+arkMultimodalPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", e.baseURL+arkMultimodalPath, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 3. 非 200：把方舟的报错原文带出来。
	//    最常见的两类：404 说明 base_url 填错（多写/少写路径段），
	//    400 说明模型名不对或该模型不支持这个接口
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("向量化接口返回 HTTP %d: %s", resp.StatusCode, truncate(strings.TrimSpace(string(raw)), 300))
	}

	// 4. 解析响应：多模态接口的向量藏在 data.embedding（注意不是 data[0].embedding）
	var parsed struct {
		Data struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("解析向量化响应失败: %w（响应原文: %s）", err, truncate(string(raw), 200))
	}
	if len(parsed.Data.Embedding) == 0 {
		return nil, fmt.Errorf("向量化响应中没有向量（响应原文: %s）", truncate(string(raw), 200))
	}
	return parsed.Data.Embedding, nil
}

// truncate 截断过长的报错文本，避免日志被刷屏
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
