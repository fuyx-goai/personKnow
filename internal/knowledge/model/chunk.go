// chunk.go —— 领域模型层：知识片段实体与引用值对象
//
// 领域模型层只放"业务概念"本身，不依赖 Eino / Gin / 数据库等任何外部框架，
// 因此它可以被 service / repo / gateway 各层任意引用，而它自己不引用任何人。
//
// 这是 DDD 分层的第一原则：依赖只允许由外向内。
package model

// Chunk 知识片段：切分后、已向量化的最小知识单元（领域实体）。
//
// 它取代了原先直接在上层流转的 schema.Document，让业务代码不必了解 Eino 细节。
type Chunk struct {
	ID       string         // 片段 ID（可为空，由向量库按内容生成）
	Content  string         // 片段正文
	Vector   []float64      // 语义向量（入库时必填，检索结果里为空）
	Metadata map[string]any // 元数据（来源文件、标题等）
	Score    float64        // 相似度分数（检索结果里才有意义）
}

// Source 片段来源文件名（从元数据里取），取不到时返回占位文案
func (c Chunk) Source() string {
	if v, ok := c.Metadata["_file_name"].(string); ok && v != "" {
		return v
	}
	return "未知来源"
}

// Reference 引用来源（值对象）：回答里用到了哪段资料、来自哪个文件、相似度多少
// 会作为 JSON 字段一起返回给前端，方便核对答案有没有依据
type Reference struct {
	Source  string  `json:"source"`  // 来源文件名
	Score   float64 `json:"score"`   // 相似度（越大越相关）
	Snippet string  `json:"snippet"` // 命中的片段内容
}
