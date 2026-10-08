// splitter.go —— 自制 Markdown 文档切分器（Eino document.Transformer 组件）
//
// 为什么要切分？
//  1. 大模型一次能"看到"的字数有限，整篇文档塞不进去；
//  2. 向量化模型单次输入也有长度限制；
//  3. 检索时按"小段"匹配，召回的内容才足够精准。
//
// 在 Eino 里，切分器本质上是 document.Transformer 组件：
//
//	输入 []*schema.Document（整文档），输出 []*schema.Document（切好的小段）。
//
// 只要实现 Transform 方法，我们的切分器就能被挂进 Eino 的编排链里！
package repo

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

// Splitter 我们自己的切分器结构体
type Splitter struct {
	// MaxLen 单个小段的最大字符数，超过就按空行再细切
	MaxLen int
}

// 编译期断言：*Splitter 必须实现 document.Transformer 接口
var _ document.Transformer = (*Splitter)(nil)

// Transform 实现 Transformer 接口：把整篇文档按 Markdown 标题切成小段
//
// 切分策略（够用就好，教学版）：
//  1. 按 "# 标题" 切段，每个标题连同它的正文算一个"小段"；
//  2. 若某段超过 MaxLen，再按空行切细；
//  3. 元数据（来源文件名等）会原样保留到每个小段，方便追溯。
func (s *Splitter) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	out := make([]*schema.Document, 0, len(src))

	// 逐个处理输入的文档（我们的场景里 loader 每次只加载一个文件）
	for _, doc := range src {
		sections := splitByHeading(doc.Content)
		for _, sec := range sections {
			// 太长的段落再按空行细切
			pieces := splitIfTooLong(sec, s.MaxLen)
			for _, p := range pieces {
				p = strings.TrimSpace(p)
				if p == "" {
					continue // 跳过空段
				}
				// 每个小段都复制一份原文档的元数据，保留"出处"信息
				out = append(out, &schema.Document{
					Content:  p,
					MetaData: cloneMeta(doc.MetaData),
				})
			}
		}
	}
	return out, nil
}

// splitByHeading 按标题行（以 # 开头的行）把全文切成若干段
// 返回值：每段都自带标题，这样每段语义独立，检索效果好
func splitByHeading(content string) []string {
	lines := strings.Split(content, "\n")

	sections := []string{}
	cur := strings.Builder{} // 当前正在攒的段落

	for _, line := range lines {
		// 以 # 开头说明遇到了新标题，先把攒完的段落收起来，再开新段
		if strings.HasPrefix(line, "#") {
			if cur.Len() > 0 {
				sections = append(sections, cur.String())
				cur.Reset()
			}
		}
		cur.WriteString(line)
		cur.WriteString("\n")
	}
	// 别忘了最后一段
	if cur.Len() > 0 {
		sections = append(sections, cur.String())
	}
	return sections
}

// splitIfTooLong 如果段落超过 maxLen，就按空行再切成多段
func splitIfTooLong(sec string, maxLen int) []string {
	if maxLen <= 0 || len(sec) <= maxLen {
		return []string{sec}
	}

	pieces := []string{}
	// 按空行分段（\n\n 或多行之间有空行都能处理）
	for _, para := range strings.Split(sec, "\n\n") {
		if strings.TrimSpace(para) == "" {
			continue
		}
		pieces = append(pieces, para)
	}
	// 切完还不够细（单个超长段落）就交给向量模型硬吃——这里从简
	return pieces
}

// cloneMeta 复制一份元数据 map，避免多个小段共享同一份数据被误改
func cloneMeta(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
