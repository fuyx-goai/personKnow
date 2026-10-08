// ingest_pipeline.go —— 知识摄入管道：把你的 Markdown 笔记变成可检索的向量数据
//
// 完整流程（这就是 RAG 的"入库"半场）：
//
//	加载文件 -> 按标题切分 -> 逐段向量化 -> 存入向量库
//
// 重点：这条流水线不是手写 for 循环串起来的，而是用 Eino 的 Chain（链）编排的：
//
//	chain.AppendLoader(...)              挂上"加载器"节点
//	chain.AppendDocumentTransformer(...) 挂上"切分器"节点
//	chain.AppendLambda(...)              挂上自定义节点（向量化 + 入库）
//
// Compile 之后得到 Runnable，用 Invoke 驱动整条链。这正是 Eino 的核心玩法。
package repo

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"knowledge-base/internal/knowledge/model"
)

// IngestPipeline 摄入管道：把 Eino 的链封装成一个"面向领域"的对象，
// 上层（应用层）只需调用 IngestFile，不必接触 document.Source / compose.Runnable 等细节。
type IngestPipeline struct {
	runner compose.Runnable[document.Source, []string]
}

// NewIngestPipeline 构建并编译"摄入链"
//
// repo 是领域端口：传内存实现就是写内存，传 Milvus 实现就是写 Milvus，
// 链本身完全不需要知道用的是哪个。
func NewIngestPipeline(ctx context.Context, emb embedding.Embedder, repo model.Repository) (*IngestPipeline, error) {
	// 1. 文件加载器：把磁盘上的文件读成 schema.Document
	//    不传 Parser 时默认按扩展名解析，.md/.txt 会按纯文本处理，正好够用
	loader, err := file.NewFileLoader(ctx, &file.FileLoaderConfig{})
	if err != nil {
		return nil, fmt.Errorf("初始化文件加载器失败: %w", err)
	}

	splitter := &Splitter{MaxLen: 800}

	// 2. 用 Eino Chain 把"加载->切分->向量化->入库"编排成一条流水线
	//    NewChain[输入类型, 输出类型]：类型参数决定了整条链"吃进什么、吐出什么"
	chain := compose.NewChain[document.Source, []string]()

	// 节点 1：加载器 —— 把文件内容读出来，包成 schema.Document
	chain.AppendLoader(loader)
	// 节点 2：切分器 —— 实现 document.Transformer 接口的组件都可以这样挂上来
	chain.AppendDocumentTransformer(splitter)
	// 节点 3：Lambda 自定义节点 —— 干三件事：覆盖去重 + 批量向量化 + 调用领域端口入库
	chain.AppendLambda(compose.InvokableLambda(func(ctx context.Context, docs []*schema.Document) ([]string, error) {
		// 3.1 覆盖同来源的旧片段：同一份笔记改了之后再摄入，应该"替换"而不是"追加"。
		//     内存库的 ID 是按追加顺序生成的，不会自动去重，必须在这里先清一次，
		//     否则反复摄入同一文件会不断堆出重复内容，检索质量也会被拖垮。
		for _, src := range distinctSources(docs) {
			if _, err := repo.DeleteBySource(ctx, src); err != nil {
				return nil, fmt.Errorf("清理「%s」的旧片段失败: %w", src, err)
			}
		}

		// 3.2 把所有小段的文本抽出来，一次请求批量算向量（省 API 调用次数）
		texts := make([]string, len(docs))
		for i, d := range docs {
			texts[i] = d.Content
		}
		vectors, err := emb.EmbedStrings(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("向量化失败: %w", err)
		}
		if len(vectors) != len(docs) {
			return nil, fmt.Errorf("向量化结果数量异常：期望 %d，实际 %d", len(docs), len(vectors))
		}
		// 3.3 组装领域片段（带上算好的向量），交给领域端口入库
		chunks := make([]model.Chunk, len(docs))
		for i, d := range docs {
			chunks[i] = model.Chunk{
				ID:       d.ID,
				Content:  d.Content,
				Vector:   vectors[i],
				Metadata: d.MetaData,
			}
		}
		// 内存版写进内存并落盘；Milvus 版写进 Milvus 集合 —— 上层无感
		return repo.Save(ctx, chunks)
	}))

	// 3. Compile：把链编译成可执行对象（此步骤会校验各节点输入输出能否对得上）
	runner, err := chain.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("编译摄入链失败: %w", err)
	}
	return &IngestPipeline{runner: runner}, nil
}

// distinctSources 收集这批文档涉及的来源文件名（去重后返回）
//
// 切分后的小段会原样保留 "_file_name" 元数据，所以这里拿到的是文件名而不是路径，
// 与"藏书"页按来源分组的粒度保持一致。
func distinctSources(docs []*schema.Document) []string {
	seen := make(map[string]struct{}, 2)
	out := make([]string, 0, 2)
	for _, d := range docs {
		src, ok := d.MetaData["_file_name"].(string)
		if !ok || src == "" {
			continue // 没有来源信息就无从"按来源覆盖"，跳过
		}
		if _, dup := seen[src]; dup {
			continue
		}
		seen[src] = struct{}{}
		out = append(out, src)
	}
	return out
}

// IngestFile 摄入单个文件，返回入库的片段数
func (p *IngestPipeline) IngestFile(ctx context.Context, path string) (int, error) {
	// Invoke：链的输入是 document.Source{URI: 文件路径}
	ids, err := p.runner.Invoke(ctx, document.Source{URI: path})
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}
