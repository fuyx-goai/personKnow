package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"knowledge-base/internal/demo/entity"
)

const maxRemoteDocumentBytes = 1024 * 1024

// ContentFetcher 仅抓取内置白名单中的 GitHub 原文，防止导入命令变成通用 SSRF 工具。
type ContentFetcher struct {
	client       *http.Client
	allowedHosts map[string]struct{}
	now          func() time.Time
}

func NewContentFetcher(client *http.Client) *ContentFetcher {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &ContentFetcher{
		client: client,
		allowedHosts: map[string]struct{}{
			"raw.githubusercontent.com": {},
		},
		now: time.Now,
	}
}

func (fetcher *ContentFetcher) Fetch(ctx context.Context, source entity.RemoteDocumentSeed) (entity.DocumentSeed, error) {
	parsed, err := url.Parse(source.RawURL)
	if err != nil || parsed.Scheme != "https" {
		return entity.DocumentSeed{}, fmt.Errorf("GitHub 原文地址必须使用 HTTPS")
	}
	if _, allowed := fetcher.allowedHosts[parsed.Host]; !allowed {
		return entity.DocumentSeed{}, fmt.Errorf("拒绝非 GitHub 原文主机：%s", parsed.Host)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return entity.DocumentSeed{}, err
	}
	request.Header.Set("Accept", "text/plain")
	request.Header.Set("User-Agent", "PersonKnow-Content-Importer/1.0")
	response, err := fetcher.client.Do(request)
	if err != nil {
		return entity.DocumentSeed{}, fmt.Errorf("抓取 %s 失败：%w", source.Name, err)
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL == nil {
		return entity.DocumentSeed{}, fmt.Errorf("抓取 %s 缺少最终请求地址", source.Name)
	}
	if _, allowed := fetcher.allowedHosts[response.Request.URL.Host]; !allowed {
		return entity.DocumentSeed{}, fmt.Errorf("拒绝 GitHub 原文重定向到非受信主机：%s", response.Request.URL.Host)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return entity.DocumentSeed{}, fmt.Errorf("抓取 %s 失败：HTTP %d", source.Name, response.StatusCode)
	}
	limited := io.LimitReader(response.Body, maxRemoteDocumentBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return entity.DocumentSeed{}, fmt.Errorf("读取 %s 失败：%w", source.Name, err)
	}
	if len(body) > maxRemoteDocumentBytes {
		return entity.DocumentSeed{}, fmt.Errorf("原文 %s 过大，超过 %d 字节限制", source.Name, maxRemoteDocumentBytes)
	}
	if strings.TrimSpace(string(body)) == "" {
		return entity.DocumentSeed{}, fmt.Errorf("原文 %s 内容为空", source.Name)
	}
	document := source.DocumentSeed
	document.Content = fmt.Sprintf("# 来源与授权\n\n- 来源仓库：%s\n- 原文链接：%s\n- 许可证：%s\n- 抓取日期：%s\n\n---\n\n%s",
		source.Repository, source.SourceURL, source.License, fetcher.now().UTC().Format("2006-01-02"), body)
	document.ChunkCount = estimateChunks(document.Content)
	document.IndexedToken = estimateTokens(document.Content)
	return document, nil
}

func estimateChunks(content string) int {
	const charactersPerChunk = 800
	count := (len([]rune(content)) + charactersPerChunk - 1) / charactersPerChunk
	if count < 1 {
		return 1
	}
	return count
}

func estimateTokens(content string) int64 {
	count := int64(len([]rune(content)) / 2)
	if count < 1 {
		return 1
	}
	return count
}
