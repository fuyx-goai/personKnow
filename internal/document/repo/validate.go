package repo

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	. "knowledge-base/internal/document/entity"
)

func DetectFormat(path, originalName string) (Format, string, error) {
	extension := strings.ToLower(filepath.Ext(filepath.Base(originalName)))
	format, ok := extensionFormats[extension]
	if !ok {
		return "", "", ErrUnsupportedFormat
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	mimeType := http.DetectContentType(firstBytes(data, 512))
	switch format {
	case FormatPDF:
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			return "", "", ErrInvalidFile
		}
		mimeType = "application/pdf"
	case FormatDOCX:
		if !zipContains(path, "word/document.xml") {
			return "", "", ErrInvalidFile
		}
		mimeType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case FormatPPTX:
		if !zipHasPrefix(path, "ppt/slides/slide") {
			return "", "", ErrInvalidFile
		}
		mimeType = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	default:
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return "", "", ErrInvalidFile
		}
		mimeType = "text/plain; charset=utf-8"
		if format == FormatHTML {
			mimeType = "text/html; charset=utf-8"
		}
		if format == FormatCSV {
			mimeType = "text/csv; charset=utf-8"
		}
	}
	return format, mimeType, nil
}

var extensionFormats = map[string]Format{
	".pdf": FormatPDF, ".docx": FormatDOCX, ".md": FormatMarkdown,
	".markdown": FormatMarkdown, ".txt": FormatText, ".html": FormatHTML,
	".htm": FormatHTML, ".csv": FormatCSV, ".pptx": FormatPPTX,
}

func zipContains(path, target string) bool {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name == target {
			return true
		}
	}
	return false
}

func zipHasPrefix(path, prefix string) bool {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer archive.Close()
	for _, file := range archive.File {
		if strings.HasPrefix(file.Name, prefix) && strings.HasSuffix(file.Name, ".xml") {
			return true
		}
	}
	return false
}

func firstBytes(data []byte, limit int) []byte {
	if len(data) <= limit {
		return data
	}
	return data[:limit]
}
