// ingest.go —— 应用服务层：知识摄入用例
//
// 它做两件事：
//  1. 找出要处理的 Markdown 文件（目录遍历、后缀过滤）；
//  2. 逐个文件交给 model.Ingester 端口（由 repo 层的 Eino 摄入管道实现）驱动入库。
package service

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"knowledge-base/internal/knowledge/dto"
	"knowledge-base/internal/knowledge/model"
)

// IngestService 摄入用例
type IngestService struct {
	ingester model.Ingester
}

// NewIngestService 注入摄入端口
func NewIngestService(ingester model.Ingester) *IngestService {
	return &IngestService{ingester: ingester}
}

// Ingest 把一批文件（或目录）灌进知识库，返回每个文件的处理结果
func (s *IngestService) Ingest(ctx context.Context, cmd dto.IngestCommand) ([]dto.IngestResult, error) {
	// 1. 收集要处理的文件：单个文件直接处理，目录则遍历其中的 .md 文件
	var files []string
	for _, p := range cmd.Paths {
		found, err := collectMarkdown(p)
		if err != nil {
			return nil, fmt.Errorf("读取路径 %s 失败: %w", p, err)
		}
		files = append(files, found...)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("没有找到任何 .md 文件，请检查路径")
	}

	// 2. 逐个文件驱动流水线
	results := make([]dto.IngestResult, 0, len(files))
	for _, f := range files {
		n, err := s.ingester.IngestFile(ctx, f)
		if err != nil {
			// 单个文件失败不影响其它文件，如实记录后继续
			results = append(results, dto.NewIngestError(f, err))
			continue
		}
		results = append(results, dto.NewIngestResult(f, n))
	}
	return results, nil
}

// collectMarkdown 收集路径下的所有 Markdown 文件（单文件则原样返回）
func collectMarkdown(path string) ([]string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return []string{path}, nil // 单个文件直接返回
	}

	files := []string{}
	// filepath.WalkDir 遍历目录树，比 filepath.Walk 更省内存
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// 只收 .md / .markdown 文件，跳过子目录本身
		if !d.IsDir() && (strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".markdown")) {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}
