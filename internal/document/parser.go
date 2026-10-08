package document

import (
	"context"
	"fmt"
)

type Parser interface {
	Parse(context.Context, string) (ParsedContent, error)
}

type ParserRegistry struct {
	parsers map[Format]Parser
}

func NewParserRegistry() *ParserRegistry {
	text := textParser{}
	return &ParserRegistry{parsers: map[Format]Parser{
		FormatPDF: pdfParser{}, FormatDOCX: officeParser{format: FormatDOCX},
		FormatMarkdown: text, FormatText: text, FormatHTML: htmlParser{},
		FormatCSV: text, FormatPPTX: officeParser{format: FormatPPTX},
	}}
}

func (registry *ParserRegistry) Supports(format Format) bool {
	_, ok := registry.parsers[format]
	return ok
}

func (registry *ParserRegistry) Parse(ctx context.Context, path string, format Format) (ParsedContent, error) {
	parser, ok := registry.parsers[format]
	if !ok {
		return ParsedContent{}, ErrUnsupportedFormat
	}
	parsed, err := parser.Parse(ctx, path)
	if err != nil {
		return ParsedContent{}, fmt.Errorf("解析 %s 文件失败: %w", format, err)
	}
	if parsed.StructureMap == nil {
		parsed.StructureMap = map[string]any{}
	}
	return parsed, nil
}
