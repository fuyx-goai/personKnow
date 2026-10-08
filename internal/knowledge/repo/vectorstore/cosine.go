// cosine.go —— 向量相似度与通用小工具（内存向量库的"检索内核"）
package vectorstore

import (
	"math"

	"github.com/cloudwego/eino/schema"
)

// cosine 计算两个向量的余弦相似度
// 值域 [-1, 1]：越接近 1 表示两段文字语义越相近
// 公式：cos(A, B) = (A · B) / (|A| * |B|)
func cosine(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i] // 点积
		na += a[i] * a[i]  // |A| 的平方
		nb += b[i] * b[i]  // |B| 的平方
	}
	if na == 0 || nb == 0 {
		return 0 // 零向量没有方向，约定相似度为 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// fileStem 从元数据里取来源文件名，作为 ID 前缀（没有就返回 doc）
func fileStem(doc *schema.Document) string {
	if v, ok := doc.MetaData["_file_name"].(string); ok {
		return v
	}
	return "doc"
}

// cloneMeta 复制一份元数据 map，避免多个文档共享同一份数据被误改
func cloneMeta(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// sourceOf 从元数据里取来源文件名
// "藏书"页要按来源分组、按来源删除，都靠它。取不到时返回占位文案，
// 与领域模型 model.Chunk.Source() 的约定保持一致。
func sourceOf(meta map[string]any) string {
	if v, ok := meta["_file_name"].(string); ok && v != "" {
		return v
	}
	return "未知来源"
}
