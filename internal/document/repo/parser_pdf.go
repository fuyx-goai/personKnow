package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"

	. "knowledge-base/internal/document/entity"
)

type pdfParser struct{}

func (pdfParser) Parse(ctx context.Context, path string) (ParsedContent, error) {
	file, reader, err := pdf.Open(path)
	if err != nil {
		return ParsedContent{}, err
	}
	defer file.Close()
	var builder strings.Builder
	pages := make([]map[string]any, 0, reader.NumPage())
	for pageNumber := 1; pageNumber <= reader.NumPage(); pageNumber++ {
		if err := ctx.Err(); err != nil {
			return ParsedContent{}, err
		}
		page := reader.Page(pageNumber)
		if page.V.IsNull() {
			continue
		}
		plainText, err := page.GetPlainText(nil)
		if err != nil {
			return ParsedContent{}, fmt.Errorf("读取第 %d 页失败: %w", pageNumber, err)
		}
		text := strings.TrimSpace(plainText)
		if text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(text)
		pages = append(pages, map[string]any{"page": pageNumber})
	}
	if builder.Len() == 0 {
		return ParsedContent{}, fmt.Errorf("PDF 中没有可提取文本")
	}
	return ParsedContent{Text: builder.String(), StructureMap: map[string]any{"pages": pages}}, nil
}
