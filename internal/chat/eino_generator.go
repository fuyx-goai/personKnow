package chat

import (
	"context"
	"errors"
	"io"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	usage "knowledge-base/internal/usage"
)

type EinoGenerator struct {
	model einomodel.BaseChatModel
}

func NewEinoGenerator(model einomodel.BaseChatModel) *EinoGenerator {
	return &EinoGenerator{model: model}
}

func (generator *EinoGenerator) Stream(ctx context.Context, prompt string, onDelta func(string) error) (usage.Breakdown, error) {
	stream, err := generator.model.Stream(ctx, []*schema.Message{
		schema.SystemMessage("你是严谨的个人知识库助手。"), schema.UserMessage(prompt),
	})
	if err != nil {
		return usage.Breakdown{}, err
	}
	defer stream.Close()
	var breakdown usage.Breakdown
	for {
		message, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return breakdown, nil
		}
		if err != nil {
			return breakdown, err
		}
		if message.ResponseMeta != nil && message.ResponseMeta.Usage != nil {
			breakdown.Input = int64(message.ResponseMeta.Usage.PromptTokens)
			breakdown.Output = int64(message.ResponseMeta.Usage.CompletionTokens)
		}
		if message.Content != "" {
			if err := onDelta(message.Content); err != nil {
				return breakdown, err
			}
		}
	}
}
