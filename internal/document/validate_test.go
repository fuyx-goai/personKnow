package document

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFormatRejectsExtensionMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake.pdf")
	writeZipFixture(t, path, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   `<w:document xmlns:w="w"><w:t>not pdf</w:t></w:document>`,
	})
	if _, _, err := DetectFormat(path, "fake.pdf"); !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("expected extension mismatch, got %v", err)
	}
}

func TestDetectFormatAcceptsSupportedTextAndOffice(t *testing.T) {
	textPath := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(textPath, []byte("# 知识"), 0o600); err != nil {
		t.Fatal(err)
	}
	format, _, err := DetectFormat(textPath, "note.md")
	if err != nil || format != FormatMarkdown {
		t.Fatalf("unexpected markdown detection: %s %v", format, err)
	}

	docxPath := filepath.Join(t.TempDir(), "note.docx")
	writeZipFixture(t, docxPath, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   `<w:document xmlns:w="w"><w:t>知识</w:t></w:document>`,
	})
	format, _, err = DetectFormat(docxPath, "note.docx")
	if err != nil || format != FormatDOCX {
		t.Fatalf("unexpected docx detection: %s %v", format, err)
	}
}
