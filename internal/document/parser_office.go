package document

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

type officeParser struct {
	format Format
}

func (parser officeParser) Parse(ctx context.Context, path string) (ParsedContent, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return ParsedContent{}, err
	}
	defer archive.Close()
	entries := officeEntries(archive.File, parser.format)
	if len(entries) == 0 {
		return ParsedContent{}, ErrInvalidFile
	}
	var sections []map[string]any
	var builder strings.Builder
	for index, entry := range entries {
		if err := ctx.Err(); err != nil {
			return ParsedContent{}, err
		}
		text, err := extractXMLText(entry)
		if err != nil {
			return ParsedContent{}, err
		}
		if text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(text)
		sections = append(sections, map[string]any{"index": index + 1, "source": entry.Name})
	}
	return ParsedContent{Text: builder.String(), StructureMap: map[string]any{"sections": sections}}, nil
}

func officeEntries(files []*zip.File, format Format) []*zip.File {
	var entries []*zip.File
	for _, file := range files {
		if format == FormatDOCX && file.Name == "word/document.xml" {
			entries = append(entries, file)
		}
		if format == FormatPPTX && strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			entries = append(entries, file)
		}
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name < entries[right].Name })
	return entries
}

func extractXMLText(file *zip.File) (string, error) {
	reader, err := file.Open()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	decoder := xml.NewDecoder(reader)
	var builder strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "t" {
			continue
		}
		var text string
		if err := decoder.DecodeElement(&text, &start); err != nil {
			return "", err
		}
		if text = strings.TrimSpace(text); text != "" {
			if builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteString(text)
		}
	}
	if builder.Len() == 0 {
		return "", fmt.Errorf("Office XML 中没有可读文本")
	}
	return builder.String(), nil
}
