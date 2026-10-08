package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"knowledge-base/internal/gateway/handler"
	"knowledge-base/internal/gateway/router"
	"knowledge-base/internal/knowledge/dto"
	"knowledge-base/internal/knowledge/model"
	"knowledge-base/internal/knowledge/service"
	"knowledge-base/pkg/config"
)

func TestHTTPEndpoints(t *testing.T) {
	ctx := context.Background()
	docsDir := writeIngestFixtures(t)
	t.Chdir(t.TempDir())
	kbRepo, ingestPipe, chatPipe := newTestComponents(t, ctx, []string{"Goroutine", " 是", " 轻量级", "线程", "。"})
	cfg := config.Config{HTTPAddr: ":0", VectorStore: config.StoreMem}
	h := handler.New(cfg, service.NewChatService(chatPipe), service.NewIngestService(ingestPipe), service.NewLibraryService(kbRepo))
	engine := router.New(h)

	recorder := doRequest(t, engine, http.MethodGet, "/api/health", "")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "ok") {
		t.Fatalf("健康检查失败: code=%d, body=%s", recorder.Code, recorder.Body.String())
	}
	body, _ := json.Marshal(map[string]string{"path": docsDir})
	recorder = doRequest(t, engine, http.MethodPost, "/api/ingest", string(body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("摄入失败: code=%d, body=%s", recorder.Code, recorder.Body.String())
	}
	var ingestResponse struct {
		TotalChunks int                `json:"totalChunks"`
		Results     []dto.IngestResult `json:"results"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &ingestResponse); err != nil || ingestResponse.TotalChunks == 0 {
		t.Fatalf("摄入响应异常: response=%+v err=%v", ingestResponse, err)
	}

	recorder = doRequest(t, engine, http.MethodPost, "/api/chat", `{"question":"goroutine 是什么"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("问答失败: code=%d, body=%s", recorder.Code, recorder.Body.String())
	}
	var chatResponse struct {
		Answer     string            `json:"answer"`
		References []model.Reference `json:"references"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &chatResponse); err != nil {
		t.Fatal("解析问答响应失败:", err)
	}
	if chatResponse.Answer != "Goroutine 是 轻量级线程。" || len(chatResponse.References) == 0 {
		t.Fatalf("问答响应异常: %+v", chatResponse)
	}

	assertStreamingEndpoint(t, engine)
	recorder = doRequest(t, engine, http.MethodPost, "/api/chat", `{"question":"  "}`)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("空问题应返回 400，实际 %d", recorder.Code)
	}
	recorder = doRequest(t, engine, http.MethodGet, "/api/stats", "")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), config.StoreMem) {
		t.Errorf("统计接口异常: code=%d, body=%s", recorder.Code, recorder.Body.String())
	}
	assertChunkEndpoints(t, engine)
}

func assertStreamingEndpoint(t *testing.T, engine http.Handler) {
	t.Helper()
	recorder := doRequest(t, engine, http.MethodPost, "/api/chat/stream", `{"question":"goroutine 是什么"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("流式问答失败: code=%d, body=%s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Errorf("Content-Type 应为 text/event-stream，实际: %s", contentType)
	}
	body := recorder.Body.String()
	if strings.Count(body, "data: ") < 3 || !strings.Contains(body, `"done":true`) || !strings.Contains(body, "[DONE]") {
		t.Errorf("SSE 事件不完整:\n%s", body)
	}
}

func assertChunkEndpoints(t *testing.T, engine http.Handler) {
	t.Helper()
	recorder := doRequest(t, engine, http.MethodGet, "/api/chunks", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("列出片段失败: code=%d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Chunks []dto.ChunkView `json:"chunks"`
		Count  int             `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Count == 0 {
		t.Fatalf("片段列表异常: response=%+v err=%v", response, err)
	}
	if first := response.Chunks[0]; first.ID == "" || first.Source == "" {
		t.Errorf("片段缺少 ID 或来源: %+v", first)
	}
	source, before := response.Chunks[0].Source, response.Count
	recorder = doRequest(t, engine, http.MethodDelete, "/api/chunks?source="+url.QueryEscape(source), "")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"deleted"`) {
		t.Fatalf("按来源删除失败: code=%d, body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = doRequest(t, engine, http.MethodGet, "/api/chunks", "")
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Count >= before {
		t.Fatalf("删除来源后片段数异常: before=%d after=%d err=%v", before, response.Count, err)
	}
	recorder = doRequest(t, engine, http.MethodDelete, "/api/chunks", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("清空整库失败: code=%d, body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = doRequest(t, engine, http.MethodGet, "/api/stats", "")
	var stats struct {
		Chunks int `json:"chunks"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &stats); err != nil || stats.Chunks != 0 {
		t.Fatalf("清空后统计异常: stats=%+v err=%v", stats, err)
	}
}

func doRequest(t *testing.T, engine http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}
