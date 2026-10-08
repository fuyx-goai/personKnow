// handler.go —— 网关层：HTTP 控制器
//
// 控制器只做三件事：绑定参数 -> 翻译成业务命令 -> 调用 service 用例并序列化返回。
// 真正的 RAG 逻辑在 service 与 repo 层，这里保持"薄"。
//
// 它依赖的是 service 层（内层），不依赖 repo 层（更内层的细节实现），
// 因此换向量库、换大模型都不需要动这个文件。
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"knowledge-base/internal/gateway/request"
	"knowledge-base/internal/knowledge/dto"
	"knowledge-base/internal/knowledge/service"
	"knowledge-base/pkg/config"
)

// Handler HTTP 控制器：持有配置与用例服务
type Handler struct {
	cfg     config.Config
	chat    *service.ChatService
	ingest  *service.IngestService
	library *service.LibraryService
}

// New 组装控制器（由网关路由调用）
func New(cfg config.Config, chat *service.ChatService,
	ingest *service.IngestService, library *service.LibraryService) *Handler {
	return &Handler{cfg: cfg, chat: chat, ingest: ingest, library: library}
}

// Health 健康检查：进程活着就返回 ok（K8s 探针、负载均衡常用）
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Config 查看当前生效的配置（API Key / Milvus 密码均已打码）
//
// 除了给人看的摘要文本，另外返回几个结构化字段，前端状态栏直接取用，
// 免得去正则解析那串摘要。
func (h *Handler) Config(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"config":      h.cfg.Describe(),
		"vectorStore": h.cfg.VectorStore,
		"chatModel":   h.cfg.LLM.ChatModel,
		"embedModel":  h.cfg.LLM.EmbedModel,
		"embedAPI":    h.cfg.LLM.EmbedAPI,
	})
}

// Stats 知识库统计：当前用的哪种向量库、存了多少片段
func (h *Handler) Stats(c *gin.Context) {
	n, err := h.library.Chunks(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"vectorStore": h.cfg.VectorStore,
		"chunks":      n,
		"embedAPI":    h.cfg.LLM.EmbedAPI,
	})
}

// Chunks 列出知识库中的所有片段（"藏书"页用）
// 可用 ?limit= 控制返回条数，默认由向量库取上限
func (h *Handler) Chunks(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))

	views, err := h.library.List(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"chunks": views, "count": len(views)})
}

// DeleteChunks 删除片段：?source=go-notes.md 只删这个来源；不带 source 则清空整库
//
// 注意：清空整库不可逆，前端必须做二次确认。
func (h *Handler) DeleteChunks(c *gin.Context) {
	source := c.Query("source")

	deleted, err := h.library.Delete(c.Request.Context(), source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted, "source": source})
}

// Ingest 摄入文档：请求体形如 {"path":"./docs"} 或 {"paths":["./docs","./a.md"]}
func (h *Handler) Ingest(c *gin.Context) {
	var req request.Ingest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON，且包含 path 或 paths 字段"})
		return
	}

	paths := req.AllPaths()
	if len(paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请通过 path 或 paths 指定要摄入的文件/目录"})
		return
	}

	results, err := h.ingest.Ingest(c.Request.Context(), dto.IngestCommand{Paths: paths})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 汇总成出参（含成功入库的片段总数，方便一眼看出效果）
	c.JSON(http.StatusOK, dto.NewIngestSummary(results))
}

// Chat 非流式问答：一次性返回完整答案 + 引用
// 适合程序调用（比如给别的服务当接口用）；要想打字机效果用 /api/chat/stream
func (h *Handler) Chat(c *gin.Context) {
	cmd, ok := bindAsk(c)
	if !ok {
		return
	}

	ans, err := h.chat.Ask(c.Request.Context(), cmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ans)
}

// ChatStream 流式问答：用 SSE（Server-Sent Events）把大模型的增量内容实时推给前端
//
// 事件格式（每行一个 JSON）：
//
//	data: {"delta":"你"}          // 增量文本
//	data: {"delta":"好"}          // 再来一段
//	data: {"references":[...],"done":true}  // 结束，附带引用来源
//	data: [DONE]
func (h *Handler) ChatStream(c *gin.Context) {
	cmd, ok := bindAsk(c)
	if !ok {
		return
	}

	// 先声明这是 SSE 流，并关掉中间层缓冲（Nginx 等），否则前端看不到"逐字"效果
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	// 驱动流式用例：每段增量文本立刻刷给前端
	refs, err := h.chat.Stream(c.Request.Context(), cmd, func(delta string) error {
		writeSSE(c.Writer, gin.H{"delta": delta})
		c.Writer.Flush() // 立刻刷出去，前端才能实时看到
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		writeSSE(c.Writer, gin.H{"error": err.Error()})
	}

	// 收尾：把引用来源也推给前端，再发一个结束标记
	writeSSE(c.Writer, gin.H{"references": refs, "done": true})
	fmt.Fprint(c.Writer, "data: [DONE]\n\n")
	c.Writer.Flush()
}

// bindAsk 解析并校验问答请求体，翻译成业务命令；失败时已写好响应，返回 ok=false
func bindAsk(c *gin.Context) (dto.AskCommand, bool) {
	var req request.Chat
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON，且包含 question 字段"})
		return dto.AskCommand{}, false
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "question 不能为空"})
		return dto.AskCommand{}, false
	}
	return dto.AskCommand{Question: question}, true
}

// writeSSE 按 SSE 协议写一条事件：data: {...}\n\n
func writeSSE(w io.Writer, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", body)
}
