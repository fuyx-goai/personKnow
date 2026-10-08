// knowledge.go —— DTO 层：业务用例的"出参"及其装配函数
//
// 出参 DTO 是"对外承诺的契约"：gateway 直接把它序列化成 JSON 返回给前端。
// 因此 json 标签写在这里（而不是写在 model 实体上），
// 这样将来改接口字段名，不会牵动领域模型。
package dto

import (
	"crypto/sha1"
	"encoding/hex"

	"knowledge-base/internal/knowledge/model"
)

// Answer 问答出参
type Answer struct {
	// Text 模型回答
	Text string `json:"answer"`
	// References 本次回答用到的资料片段
	References []model.Reference `json:"references"`
}

// NewAnswer 装配问答出参
func NewAnswer(text string, refs []model.Reference) Answer {
	return Answer{Text: text, References: refs}
}

// IngestResult 单个文件的摄入结果
type IngestResult struct {
	File    string `json:"file"`              // 文件路径
	Chunks  int    `json:"chunks"`            // 切分并入库的片段数
	Error   string `json:"error,omitempty"`   // 失败原因（成功时为空）
	Success bool   `json:"success,omitempty"` // 是否成功
}

// NewIngestResult 装配"成功"的摄入结果
func NewIngestResult(file string, chunks int) IngestResult {
	return IngestResult{File: file, Chunks: chunks, Success: true}
}

// NewIngestError 装配"失败"的摄入结果
func NewIngestError(file string, err error) IngestResult {
	return IngestResult{File: file, Error: err.Error()}
}

// IngestSummary 摄入总览：一次请求处理了哪些文件、共入库多少片段
type IngestSummary struct {
	TotalChunks int            `json:"totalChunks"` // 成功入库的片段总数
	Results     []IngestResult `json:"results"`     // 每个文件的明细
}

// NewIngestSummary 汇总各文件结果，顺手统计片段总数
func NewIngestSummary(results []IngestResult) IngestSummary {
	total := 0
	for _, r := range results {
		total += r.Chunks
	}
	return IngestSummary{TotalChunks: total, Results: results}
}

// ChunkView 知识片段浏览出参（"藏书"页用）
//
// 刻意不带向量：前端只展示正文与来源，把 Embedding 传过去纯属浪费带宽。
// ID 由正文的短哈希生成——同一段文字无论在内存库还是 Milvus 里算出来都一样，
// 前端拿它当列表 key 才稳定。
type ChunkView struct {
	ID      string `json:"id"`      // 片段标识（正文短哈希）
	Source  string `json:"source"`  // 来源文件名
	Content string `json:"content"` // 片段正文
	Bytes   int    `json:"bytes"`   // 正文长度（字节），前端用来显示"片段大小"
}

// NewChunkViews 把领域片段装配成浏览出参
func NewChunkViews(chunks []model.Chunk) []ChunkView {
	out := make([]ChunkView, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, ChunkView{
			ID:      shortHash(c.Content),
			Source:  c.Source(),
			Content: c.Content,
			Bytes:   len(c.Content),
		})
	}
	return out
}

// shortHash 取正文 sha1 的前 12 位（足够避免碰撞，又不会长得没法看）
func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}
