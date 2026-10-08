// chat_pipeline.go —— 问答管道：这就是 RAG 的"检索 + 生成"半场
//
// 流程：用户提问 -> 去向量库捞最相关的几段笔记 -> 把"问题 + 资料"拼进提示词
//
//	-> 交给大模型 -> 流式输出答案
//
// 同样用 Eino Chain 编排：
//
//	Lambda(检索+拼提示词) -> ChatModel(大模型)
//
// 用 Compile 后的 Stream 驱动，实现"打字机"般的流式输出效果。
package repo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	// Eino 的 model 与本项目的领域包 model 重名，这里给 Eino 的起个别名
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"knowledge-base/internal/knowledge/model"
)

// refCollector 引用收集器
//
// 为什么需要它？链的输出类型必须固定为 *schema.Message 才能流式分片，
// 没法顺便把"检索到了哪些资料"带出来。于是我们用 context 传一个收集器：
// 链里的 Lambda 节点把引用塞进去，调用方在链跑完后取出来一并返回。
type refCollector struct {
	mu   sync.Mutex
	refs []model.Reference
}

func (c *refCollector) add(refs ...model.Reference) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refs = append(c.refs, refs...)
}

func (c *refCollector) all() []model.Reference {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]model.Reference, len(c.refs))
	copy(out, c.refs)
	return out
}

// refCtxKey 自定义的 context key 类型（Go 官方推荐：不要用 string 当 key，避免冲突）
type refCtxKey struct{}

// withRefCollector 把收集器挂到 context 上
func withRefCollector(ctx context.Context, c *refCollector) context.Context {
	return context.WithValue(ctx, refCtxKey{}, c)
}

// refCollectorFrom 从 context 取出收集器（没有就返回 nil）
func refCollectorFrom(ctx context.Context) *refCollector {
	c, _ := ctx.Value(refCtxKey{}).(*refCollector)
	return c
}

// ChatPipeline 问答管道：把 Eino 的问答链封装成面向领域的对象
type ChatPipeline struct {
	runner compose.Runnable[string, *schema.Message]
}

// NewChatPipeline 构建并编译"问答链"：输入一个问题，输出模型的回答消息
//
// 链上只有两个节点：
//
//	节点1 Lambda    —— 自定义逻辑：语义检索 + 填充提示词模板
//	节点2 ChatModel —— 豆包对话模型（挂在链上，Stream 驱动即为流式输出）
func NewChatPipeline(ctx context.Context, cm einomodel.BaseChatModel, repo model.Repository) (*ChatPipeline, error) {
	// 1. 提示词模板：给大模型立规矩——只根据资料回答，答不上来就直说
	//    FString 格式用 {变量名} 占位，Format 时才真正填入内容
	tpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage(
			"你是一个严谨的个人知识库助手。请仅依据下面提供的【背景资料】回答问题；"+
				"如果资料不足以回答，请直接说明不知道，禁止编造内容。"),
		schema.UserMessage(
			"【背景资料】\n{context}\n\n【我的问题】\n{question}"),
	)

	// 2. 编排问答链：吃进 string（问题），吐出 *schema.Message（答案）
	chain := compose.NewChain[string, *schema.Message]()

	// 节点 1：自定义 Lambda —— 检索相关片段并填充模板，产出可直接发给模型的消息
	chain.AppendLambda(compose.InvokableLambda(func(ctx context.Context, question string) ([]*schema.Message, error) {
		// 2.1 语义检索：去向量库找与问题最相近的 3 段笔记
		chunks, err := repo.Search(ctx, question, 3)
		if err != nil {
			return nil, fmt.Errorf("检索失败: %w", err)
		}

		// 2.2 把检索到的片段拼成资料文本，并记下引用来源
		var sb strings.Builder
		refs := make([]model.Reference, 0, len(chunks))
		for i, c := range chunks {
			refs = append(refs, model.Reference{
				Source:  c.Source(),
				Score:   c.Score,
				Snippet: c.Content,
			})
			sb.WriteString(fmt.Sprintf("资料%d（来自 %s）:\n%s\n\n", i+1, c.Source(), c.Content))
		}
		// 顺手把引用交给 context 里的收集器，调用方会随回答一起返回
		if col := refCollectorFrom(ctx); col != nil {
			col.add(refs...)
		}

		// 2.3 填充提示词模板：{context} 填资料，{question} 填问题
		msgs, err := tpl.Format(ctx, map[string]any{
			"context":  sb.String(),
			"question": question,
		})
		if err != nil {
			return nil, fmt.Errorf("模板填充失败: %w", err)
		}
		return msgs, nil
	}))

	// 节点 2：大模型 —— 上一节点输出的 []*schema.Message 正好是它的输入
	chain.AppendChatModel(cm)

	// 3. 编译得到可执行对象
	runner, err := chain.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("编译问答链失败: %w", err)
	}
	return &ChatPipeline{runner: runner}, nil
}

// Ask 非流式问答：一次性拿到完整回答 + 引用来源
func (p *ChatPipeline) Ask(ctx context.Context, question string) (string, []model.Reference, error) {
	col := &refCollector{}
	ctx = withRefCollector(ctx, col)

	msg, err := p.runner.Invoke(ctx, question)
	if err != nil {
		return "", nil, err
	}
	return msg.Content, col.all(), nil
}

// Stream 流式问答：每收到一段增量文本就回调 onDelta（用于 SSE 推送），
// 返回本次用到的引用来源
func (p *ChatPipeline) Stream(ctx context.Context, question string, onDelta func(delta string) error) ([]model.Reference, error) {
	col := &refCollector{}
	ctx = withRefCollector(ctx, col)

	stream, err := p.runner.Stream(ctx, question)
	if err != nil {
		return nil, err
	}
	defer stream.Close() // 用完关闭，释放底层资源

	// 循环取流：Recv 每次返回一小段增量文本
	// io.EOF 表示模型说完了；其它错误说明流中断
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return col.all(), nil
			}
			return col.all(), err
		}
		if msg.Content == "" {
			continue // 有些分片只有元信息没有正文，跳过
		}
		if onDelta != nil {
			if err := onDelta(msg.Content); err != nil {
				return col.all(), err
			}
		}
	}
}
