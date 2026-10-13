package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"knowledge-base/internal/demo/entity"
)

func TestFetchRemoteDocumentRejectsUntrustedHost(t *testing.T) {
	fetcher := NewContentFetcher(http.DefaultClient)
	_, err := fetcher.Fetch(context.Background(), entity.RemoteDocumentSeed{RawURL: "https://example.com/tutorial.md"})
	if err == nil || !strings.Contains(err.Error(), "GitHub") {
		t.Fatalf("应拒绝非 GitHub 原文主机：got=%v", err)
	}
}

func TestFetchRemoteDocumentRejectsOversizedBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("a", maxRemoteDocumentBytes+1)))
	}))
	defer server.Close()
	fetcher := NewContentFetcher(server.Client())
	fetcher.allowedHosts = map[string]struct{}{strings.TrimPrefix(server.URL, "https://"): {}}
	_, err := fetcher.Fetch(context.Background(), entity.RemoteDocumentSeed{RawURL: server.URL + "/tutorial.md"})
	if err == nil || !strings.Contains(err.Error(), "过大") {
		t.Fatalf("应拒绝超限原文：got=%v", err)
	}
}

func TestFetchRemoteDocumentAddsSourceAndLicense(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("# Tutorial\n\nUse Go modules."))
	}))
	defer server.Close()
	source := entity.RemoteDocumentSeed{
		DocumentSeed: entity.DocumentSeed{Key: "go-modules", LibraryKey: "go", Name: "Go Modules.md", Tags: []string{"Go", "官方教程"}},
		RawURL:       server.URL + "/tutorial.md", SourceURL: "https://github.com/golang/website/blob/master/tutorial.md",
		Repository: "golang/website", License: "BSD-3-Clause",
	}
	fetcher := NewContentFetcher(server.Client())
	fetcher.allowedHosts = map[string]struct{}{strings.TrimPrefix(server.URL, "https://"): {}}
	document, err := fetcher.Fetch(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"golang/website", "BSD-3-Clause", source.SourceURL, "Use Go modules"} {
		if !strings.Contains(document.Content, expected) {
			t.Fatalf("导入正文缺少 %q：%s", expected, document.Content)
		}
	}
}
