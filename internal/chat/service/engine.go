package service

import (
	"context"
	"fmt"
	"strings"

	. "knowledge-base/internal/chat/entity"
	platformvector "knowledge-base/internal/platform/vector"
	usage "knowledge-base/internal/usage/entity"
)

type Generator interface {
	Stream(context.Context, string, func(string) error) (usage.Breakdown, error)
}

type VectorEngine struct {
	vectors   platformvector.Repository
	generator Generator
}

func NewVectorEngine(vectors platformvector.Repository, generator Generator) *VectorEngine {
	return &VectorEngine{vectors: vectors, generator: generator}
}

func (engine *VectorEngine) Stream(ctx context.Context, question string, scope platformvector.SearchScope, settings RetrievalSettings, onDelta func(string) error) (EngineResult, error) {
	topK := settings.TopK
	if topK <= 0 {
		topK = 5
	}
	hits, err := engine.vectors.Search(ctx, question, scope, topK)
	if err != nil {
		return EngineResult{}, err
	}
	references := make([]Reference, len(hits))
	var background strings.Builder
	for index, hit := range hits {
		background.WriteString(fmt.Sprintf("资料%d（%s，%s）:\n%s\n\n", index+1, hit.SourceName, hit.LocationLabel, hit.Content))
		references[index] = Reference{
			DocumentID: hit.DocumentID, ContentVersionID: hit.ContentVersionID, ChunkID: hit.ID,
			SourceName: hit.SourceName, LocationLabel: hit.LocationLabel, Excerpt: excerpt(hit.Content, 500),
			Similarity: hit.Similarity, Rank: index + 1,
		}
	}
	prompt := "请仅依据背景资料回答；资料不足时明确说明不知道，禁止编造。\n\n【背景资料】\n" +
		background.String() + "\n【问题】\n" + question
	breakdown, err := engine.generator.Stream(ctx, prompt, onDelta)
	return EngineResult{References: references, Usage: breakdown}, err
}

func excerpt(content string, limit int) string {
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	return string(runes[:limit])
}
