package repo

import (
	"context"
	"os"
	"strings"

	"golang.org/x/net/html"

	. "knowledge-base/internal/document/entity"
)

type textParser struct{}

func (textParser) Parse(ctx context.Context, path string) (ParsedContent, error) {
	if err := ctx.Err(); err != nil {
		return ParsedContent{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ParsedContent{}, err
	}
	return ParsedContent{Text: string(content), StructureMap: map[string]any{}}, nil
}

type htmlParser struct{}

func (htmlParser) Parse(ctx context.Context, path string) (ParsedContent, error) {
	file, err := os.Open(path)
	if err != nil {
		return ParsedContent{}, err
	}
	defer file.Close()
	document, err := html.Parse(file)
	if err != nil {
		return ParsedContent{}, err
	}
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			text := strings.TrimSpace(node.Data)
			if text != "" {
				builder.WriteString(text)
				builder.WriteByte('\n')
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	if err := ctx.Err(); err != nil {
		return ParsedContent{}, err
	}
	return ParsedContent{Text: builder.String(), StructureMap: map[string]any{}}, nil
}
