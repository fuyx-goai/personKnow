// chat.go —— 应用服务层：问答用例编排
//
// 服务层只负责"编排用例"，不关心用了什么框架：
// 它依赖 model.Answerer 端口（接口），由 repo 层的 Eino 问答管道实现。
// 这就是依赖倒置——高层（用例）不依赖低层（Eino），两者都依赖抽象。
package service

import (
	"context"

	"knowledge-base/internal/knowledge/dto"
	"knowledge-base/internal/knowledge/model"
)

// ChatService 问答用例
type ChatService struct {
	answerer model.Answerer
}

// NewChatService 注入问答端口
func NewChatService(answerer model.Answerer) *ChatService {
	return &ChatService{answerer: answerer}
}

// Ask 非流式问答：一次性返回完整答案 + 引用
func (s *ChatService) Ask(ctx context.Context, cmd dto.AskCommand) (dto.Answer, error) {
	text, refs, err := s.answerer.Ask(ctx, cmd.Question)
	if err != nil {
		return dto.Answer{}, err
	}
	return dto.NewAnswer(text, refs), nil
}

// Stream 流式问答：把增量文本实时交给调用方（gateway 层再包成 SSE 推给前端）
func (s *ChatService) Stream(ctx context.Context, cmd dto.AskCommand,
	onDelta func(delta string) error) ([]model.Reference, error) {
	return s.answerer.Stream(ctx, cmd.Question, onDelta)
}
