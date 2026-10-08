// port.go —— 领域模型层：出站端口（业务需要"外界"提供的能力）
//
// 领域模型层只"声明"需要什么能力，不关心谁来"实现"。
// 依赖倒置：内层的 service 依赖下面的接口，具体实现放在 repo 层。
//
// 命名约定：
//
//	Repository 仓储端口 —— 数据的存取
//	Answerer   问答端口 —— 大模型问答能力
//	Ingester   摄入端口 —— 文档切分与入库能力
package model

import "context"

// Answerer 问答端口：由 repo 层的 Eino 问答管道实现
type Answerer interface {
	// Ask 非流式问答：返回答案 + 引用来源
	Ask(ctx context.Context, question string) (string, []Reference, error)
	// Stream 流式问答：每段增量文本回调 onDelta，返回引用来源
	Stream(ctx context.Context, question string, onDelta func(delta string) error) ([]Reference, error)
}

// Ingester 摄入端口：由 repo 层的 Eino 摄入管道实现
type Ingester interface {
	// IngestFile 摄入单个文件，返回入库的片段数
	IngestFile(ctx context.Context, path string) (int, error)
}
