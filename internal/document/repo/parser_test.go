package repo

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"

	. "knowledge-base/internal/document/entity"
)

func TestParserRegistrySupportsAllConfirmedFormats(t *testing.T) {
	registry := NewParserRegistry()
	for _, format := range []Format{FormatPDF, FormatDOCX, FormatMarkdown, FormatText, FormatHTML, FormatCSV, FormatPPTX} {
		if !registry.Supports(format) {
			t.Errorf("format %s is not supported", format)
		}
	}
}

func TestPDFParserExtractsPageText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.pdf")
	document := fpdf.New("P", "mm", "A4", "")
	document.AddPage()
	document.SetFont("Arial", "", 16)
	document.Cell(40, 10, "PDF Knowledge")
	if err := document.OutputFileAndClose(path); err != nil {
		t.Fatal(err)
	}
	parsed, err := NewParserRegistry().Parse(context.Background(), path, FormatPDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(parsed.Text, "PDF Knowledge") {
		t.Fatalf("unexpected PDF text: %q", parsed.Text)
	}
}

func TestTextHTMLAndCSVParsersExtractReadableText(t *testing.T) {
	registry := NewParserRegistry()
	tests := []struct {
		format Format
		name   string
		body   string
		want   string
	}{
		{FormatText, "note.txt", "纯文本知识", "纯文本知识"},
		{FormatMarkdown, "note.md", "# 标题\n正文", "标题"},
		{FormatHTML, "note.html", "<html><body><h1>标题</h1><p>正文</p></body></html>", "正文"},
		{FormatCSV, "note.csv", "名称,内容\nRAG,混合检索", "混合检索"},
	}
	for _, test := range tests {
		t.Run(string(test.format), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.name)
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			parsed, err := registry.Parse(context.Background(), path, test.format)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(parsed.Text, test.want) {
				t.Fatalf("expected %q in %q", test.want, parsed.Text)
			}
		})
	}
}

func TestOfficeParsersExtractDocumentAndSlideText(t *testing.T) {
	registry := NewParserRegistry()
	docxPath := filepath.Join(t.TempDir(), "note.docx")
	writeZipFixture(t, docxPath, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   `<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>DOCX知识</w:t></w:r></w:p></w:body></w:document>`,
	})
	pptxPath := filepath.Join(t.TempDir(), "slides.pptx")
	writeZipFixture(t, pptxPath, map[string]string{
		"[Content_Types].xml":   "<Types/>",
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="p" xmlns:a="a"><a:t>PPTX知识</a:t></p:sld>`,
	})

	for _, test := range []struct {
		path   string
		format Format
		want   string
	}{{docxPath, FormatDOCX, "DOCX知识"}, {pptxPath, FormatPPTX, "PPTX知识"}} {
		parsed, err := registry.Parse(context.Background(), test.path, test.format)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(parsed.Text, test.want) {
			t.Fatalf("expected %q in %q", test.want, parsed.Text)
		}
	}
}

func writeZipFixture(t *testing.T, path string, files map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, body := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
