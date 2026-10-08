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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/embedding"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"knowledge-base/internal/gateway/handler"
	"knowledge-base/internal/gateway/router"
	"knowledge-base/internal/knowledge/dto"
	"knowledge-base/internal/knowledge/model"
	"knowledge-base/internal/knowledge/repo"
	"knowledge-base/internal/knowledge/repo/vectorstore"
	"knowledge-base/internal/knowledge/service"
	"knowledge-base/pkg/config"
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

// findDocsDir 从当前测试目录向上找示例文档目录 docs/（cmd/gateway -> 项目根）
func findDocsDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		p := filepath.Join(dir, "docs")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("找不到示例文档目录 docs")
	return ""
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

	// 1. 先记下示例文档的绝对路径（下面会切目录）
	docsDir := findDocsDir(t)

	// 2. 切到临时目录运行，测试产生的 knowledge.json 不污染项目目录
	t.Chdir(t.TempDir())

	// 3. 组装组件（假模型 + 假向量化 + 内存向量库）
	kbRepo, ingestPipe, _ := newTestComponents(t, ctx, nil)

	// 4. 通过 service 层用例摄入示例文档
	svc := service.NewIngestService(ingestPipe)
	results, err := svc.Ingest(ctx, dto.IngestCommand{Paths: []string{docsDir}})
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

// TestHTTPEndpoints 用 httptest 驱动 Gin 路由，验证 HTTP 层是否把整条链对上了
func TestHTTPEndpoints(t *testing.T) {
	ctx := context.Background()

	// 1. 先取示例文档的绝对路径（下面会切目录）
	docsDir := findDocsDir(t)
	t.Chdir(t.TempDir())

	// 2. 组装应用：假模型 + 假向量化 + 内存向量库
	kbRepo, ingestPipe, chatPipe := newTestComponents(t, ctx, []string{"Goroutine", " 是", " 轻量级", "线程", "。"})
	cfg := config.Config{HTTPAddr: ":0", VectorStore: config.StoreMem}
	h := handler.New(cfg,
		service.NewChatService(chatPipe),
		service.NewIngestService(ingestPipe),
		service.NewLibraryService(kbRepo),
	)
	engine := router.New(h)

	// 3. 健康检查
	rec := doRequest(t, engine, http.MethodGet, "/api/health", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("健康检查失败: code=%d, body=%s", rec.Code, rec.Body.String())
	}

	// 4. 摄入接口：把 docs/ 目录灌进来
	body, _ := json.Marshal(map[string]string{"path": docsDir})
	rec = doRequest(t, engine, http.MethodPost, "/api/ingest", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("摄入失败: code=%d, body=%s", rec.Code, rec.Body.String())
	}
	var ingestResp struct {
		TotalChunks int                `json:"totalChunks"`
		Results     []dto.IngestResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ingestResp); err != nil {
		t.Fatal("解析摄入响应失败:", err)
	}
	if ingestResp.TotalChunks == 0 {
		t.Fatal("摄入成功但片段数为 0")
	}

	// 5. 非流式问答：应返回答案 + 引用
	rec = doRequest(t, engine, http.MethodPost, "/api/chat", `{"question":"goroutine 是什么"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("问答失败: code=%d, body=%s", rec.Code, rec.Body.String())
	}
	var chatResp struct {
		Answer     string            `json:"answer"`
		References []model.Reference `json:"references"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &chatResp); err != nil {
		t.Fatal("解析问答响应失败:", err)
	}
	if chatResp.Answer != "Goroutine 是 轻量级线程。" {
		t.Errorf("答案不对: %q", chatResp.Answer)
	}
	if len(chatResp.References) == 0 {
		t.Error("期望返回引用来源，实际为空")
	}

	// 6. 流式问答：SSE 应该分多条事件推出来
	rec = doRequest(t, engine, http.MethodPost, "/api/chat/stream", `{"question":"goroutine 是什么"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("流式问答失败: code=%d, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type 应为 text/event-stream，实际: %s", ct)
	}
	sseBody := rec.Body.String()
	if strings.Count(sseBody, "data: ") < 3 {
		t.Errorf("期望多条 SSE 事件，实际内容:\n%s", sseBody)
	}
	if !strings.Contains(sseBody, `"done":true`) || !strings.Contains(sseBody, "[DONE]") {
		t.Errorf("SSE 缺少结束标记:\n%s", sseBody)
	}

	// 7. 参数校验：question 为空应返回 400
	rec = doRequest(t, engine, http.MethodPost, "/api/chat", `{"question":"  "}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("空问题应返回 400，实际 %d", rec.Code)
	}

	// 8. 统计接口
	rec = doRequest(t, engine, http.MethodGet, "/api/stats", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), config.StoreMem) {
		t.Errorf("统计接口异常: code=%d, body=%s", rec.Code, rec.Body.String())
	}

	// 9. 列出片段（藏书页）：刚摄入过，列表不应为空
	rec = doRequest(t, engine, http.MethodGet, "/api/chunks", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("列出片段失败: code=%d, body=%s", rec.Code, rec.Body.String())
	}
	var chunksResp struct {
		Chunks []dto.ChunkView `json:"chunks"`
		Count  int             `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &chunksResp); err != nil {
		t.Fatal("解析片段列表失败:", err)
	}
	if chunksResp.Count == 0 {
		t.Fatal("摄入过文档，片段列表不应为空")
	}
	if first := chunksResp.Chunks[0]; first.ID == "" || first.Source == "" {
		t.Errorf("片段缺少 ID 或来源: %+v", first)
	}

	// 10. 按来源删除：删掉其中一个来源，片段总数应当减少
	src := chunksResp.Chunks[0].Source
	before := chunksResp.Count
	rec = doRequest(t, engine, http.MethodDelete, "/api/chunks?source="+url.QueryEscape(src), "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deleted"`) {
		t.Fatalf("按来源删除失败: code=%d, body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, engine, http.MethodGet, "/api/chunks", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &chunksResp); err != nil {
		t.Fatal("解析片段列表失败:", err)
	}
	if chunksResp.Count >= before {
		t.Errorf("删除 %s 后片段数应该减少：之前 %d，现在 %d", src, before, chunksResp.Count)
	}

	// 11. 清空整库（不带 source）：片段数应归零
	rec = doRequest(t, engine, http.MethodDelete, "/api/chunks", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("清空整库失败: code=%d, body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, engine, http.MethodGet, "/api/stats", "")
	var statsResp struct {
		Chunks int `json:"chunks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &statsResp); err != nil {
		t.Fatal("解析统计响应失败:", err)
	}
	if statsResp.Chunks != 0 {
		t.Errorf("清空后片段数应为 0，实际 %d", statsResp.Chunks)
	}
}

// doRequest 用 httptest 发一个请求给 Gin 路由（不需要真的监听端口）
func doRequest(t *testing.T, engine http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}
