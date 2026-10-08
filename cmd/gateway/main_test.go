// main_test.go —— 集成测试：不需要 API Key、也不需要 Milvus，就能验证整条链路！
//
// 思路：写"假的" Embedder（按字符词袋生成向量）和"假的" ChatModel（分片吐字），
// 替代真实的豆包服务，从而零成本跑通：
//
//	加载 -> 切分 -> 向量化 -> 入库 -> 检索 -> 流式生成 -> HTTP 接口
//
// 运行方式：go test ./cmd/gateway -v
//
// 命名注意：本文件同时用到两种 "model"——
//
//	model     本项目领域模型层（Chunk / Reference / Repository）
//	einomodel Eino 的大模型组件包，因重名而起了别名
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/embedding"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"knowledge-base/internal/knowledge/dto"
	"knowledge-base/internal/knowledge/model"
	"knowledge-base/internal/knowledge/repo"
	"knowledge-base/internal/knowledge/repo/vectorstore"
	"knowledge-base/internal/knowledge/service"
)

// fakeEmbedder 假向量化组件：实现 embedding.Embedder 接口即可直接替换真组件
// （这也是 Eino "面向接口编程"的好处——测试时随手换实现）
type fakeEmbedder struct{}

// EmbedStrings 把每段文字变成 128 维"字符词袋"向量：
// 出现过的字符越多，对应维度越大；文字相似 → 向量也相似，足够测试用
func (f *fakeEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	const dims = 128
	out := make([][]float64, len(texts))
	for i, t := range texts {
		v := make([]float64, dims)
		for _, r := range t {
			v[int(r)%dims]++
		}
		out[i] = v
	}
	return out, nil
}

// fakeChatModel 假对话模型：同样只需实现 einomodel.BaseChatModel 的两个方法
//   - Generate：一次性返回完整回答
//   - Stream  ：返回"一段段"的流，用来验证流式输出链路
type fakeChatModel struct {
	chunks []string // 假装模型是一个字一个字往外吐的
}

// Generate 一次性返回完整回答
func (f *fakeChatModel) Generate(ctx context.Context, in []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	return schema.AssistantMessage(strings.Join(f.chunks, ""), nil), nil
}

// Stream 流式返回：把 chunks 变成 eino 的流
func (f *fakeChatModel) Stream(ctx context.Context, in []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	msgs := make([]*schema.Message, 0, len(f.chunks))
	for _, c := range f.chunks {
		msgs = append(msgs, schema.AssistantMessage(c, nil))
	}
	// StreamReaderFromArray：把数组包装成流（Eino 提供的测试利器）
	return schema.StreamReaderFromArray(msgs), nil
}

// writeIngestFixtures 创建不受生产文档改动影响的固定摄入样本。
func writeIngestFixtures(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	fixtures := map[string]string{
		"go-notes.md":     "# Goroutine\n\nGoroutine 是 Go 语言的轻量级线程。",
		"deploy-notes.md": "# 部署说明\n\n生产服务应使用 HTTPS，并配置健康检查。",
	}
	for name, content := range fixtures {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

// newTestComponents 组装一套测试用的组件：假模型 + 假向量化 + 内存向量库 + 领域端口
func newTestComponents(t *testing.T, ctx context.Context, chunks []string) (*vectorstore.Repository, *repo.IngestPipeline, *repo.ChatPipeline) {
	t.Helper()

	emb := &fakeEmbedder{}
	store, err := vectorstore.NewMemStore(ctx, emb)
	if err != nil {
		t.Fatal(err)
	}
	// 注意：变量名不能叫 repo，否则会遮蔽同名的 repo 包
	kbRepo := vectorstore.NewRepository(store)

	ingestPipe, err := repo.NewIngestPipeline(ctx, emb, kbRepo)
	if err != nil {
		t.Fatal("构建摄入管道失败:", err)
	}
	chatPipe, err := repo.NewChatPipeline(ctx, &fakeChatModel{chunks: chunks}, kbRepo)
	if err != nil {
		t.Fatal("构建问答管道失败:", err)
	}
	return kbRepo, ingestPipe, chatPipe
}

// seed 手动往向量库塞一条知识（真实流程里由摄入链负责向量化）
func seed(t *testing.T, ctx context.Context, kbRepo *vectorstore.Repository) {
	t.Helper()
	emb := &fakeEmbedder{}
	vecs, err := emb.EmbedStrings(ctx, []string{"Goroutine 是 Go 语言的轻量级线程。"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = kbRepo.Save(ctx, []model.Chunk{{
		Content:  "Goroutine 是 Go 语言的轻量级线程。",
		Vector:   vecs[0],
		Metadata: map[string]any{"_file_name": "go-notes.md"},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

// TestIngestAndRetrieve 端到端冒烟测试：摄入两篇示例文档，然后做语义检索
func TestIngestAndRetrieve(t *testing.T) {
	ctx := context.Background()

	// 1. 先生成固定测试文档（下面会切目录）
	docsDir := writeIngestFixtures(t)

	// 2. 切到临时目录运行，测试产生的 knowledge.json 不污染项目目录
	t.Chdir(t.TempDir())

	// 3. 组装组件（假模型 + 假向量化 + 内存向量库）
	kbRepo, ingestPipe, _ := newTestComponents(t, ctx, nil)

	// 4. 通过 service 层用例摄入示例文档
	svc := service.NewIngestService(ingestPipe)
	results, err := svc.Ingest(ctx, dto.IngestCommand{Paths: []string{
		filepath.Join(docsDir, "go-notes.md"),
		filepath.Join(docsDir, "deploy-notes.md"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if !r.Success {
			t.Fatalf("摄入 %s 失败: %s", r.File, r.Error)
		}
	}

	// 5. 检索：问一个示例文档里有的问题，最相关的结果应该包含 "goroutine"
	chunks, err := kbRepo.Search(ctx, "goroutine 是什么", 3)
	if err != nil {
		t.Fatal("检索失败:", err)
	}
	if len(chunks) == 0 {
		t.Fatal("检索结果为空")
	}
	if !strings.Contains(strings.ToLower(chunks[0].Content), "goroutine") {
		t.Errorf("最相关的结果应该是 Go 笔记，实际是:\n%s", chunks[0].Content)
	}

	// 6. 检索结果必须按相似度降序
	for i := 1; i < len(chunks); i++ {
		if chunks[i-1].Score < chunks[i].Score {
			t.Error("检索结果没有按相似度降序排列")
		}
	}

	// 7. 知识库应该已落盘（下次启动时能直接加载）
	if _, err := os.Stat("knowledge.json"); err != nil {
		t.Error("knowledge.json 没有生成:", err)
	}

	// 8. Count 应该能数出片段数
	n, err := kbRepo.Count(ctx)
	if err != nil || n == 0 {
		t.Errorf("Count 结果异常: n=%d, err=%v", n, err)
	}
}

// TestChatChainStreaming 验证问答链能"流式"吐字：
// 用的是假模型，所以不需要豆包 API Key，也能测出流式链路是否通畅
func TestChatChainStreaming(t *testing.T) {
	ctx := context.Background()
	t.Chdir(t.TempDir())

	kbRepo, _, chatPipe := newTestComponents(t, ctx, []string{"Goroutine", " 是", " 轻量级", "线程", "。"})
	seed(t, ctx, kbRepo)

	// 通过 service 层用例流式问答：一块一块地接收
	svc := service.NewChatService(chatPipe)
	recvCount := 0
	var answer strings.Builder
	refs, err := svc.Stream(ctx, dto.AskCommand{Question: "goroutine 是什么"}, func(delta string) error {
		recvCount++
		answer.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatal("流式调用失败:", err)
	}

	// 校验：确实分多片返回（说明是流式，而不是一次性返回）
	if recvCount < 2 {
		t.Errorf("期望分片返回（流式），实际只收到 %d 片", recvCount)
	}
	if got := answer.String(); got != "Goroutine 是 轻量级线程。" {
		t.Errorf("回答内容不对，实际是: %q", got)
	}
	if len(refs) == 0 {
		t.Error("期望返回引用来源，实际为空")
	}
}
