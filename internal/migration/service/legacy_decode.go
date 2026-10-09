package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	. "knowledge-base/internal/migration/entity"
	platformvector "knowledge-base/internal/platform/vector"
)

func decodeLegacy(raw []byte) (map[string][]legacyEntry, []Failure, error) {
	var entries []legacyEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, nil, fmt.Errorf("解析旧知识库 JSON 失败: %w", err)
	}
	groups := map[string][]legacyEntry{}
	failures := []Failure{}
	for _, entry := range entries {
		if isScopedEntry(entry.Meta) {
			continue
		}
		source := sourceName(entry.Meta)
		switch {
		case strings.TrimSpace(entry.Content) == "":
			failures = append(failures, Failure{Source: source, Code: "invalid_content", Message: "正文为空"})
		case !validVector(entry.Vector):
			failures = append(failures, Failure{Source: source, Code: "invalid_vector", Message: "向量为空或包含非法数值"})
		default:
			groups[source] = append(groups[source], entry)
		}
	}
	return groups, failures, nil
}

func buildImport(userID, libraryID uuid.UUID, source string, entries []legacyEntry, now time.Time) (ImportDocument, string, []platformvector.Chunk) {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, entry.Content)
	}
	content := strings.Join(parts, "\n\n")
	hash := contentHash(content)
	documentID := uuid.NewSHA1(LegacyNamespace, []byte(userID.String()+"\x00"+source+"\x00"+hash))
	versionID := uuid.NewSHA1(LegacyNamespace, []byte(documentID.String()+"\x001"))
	item := ImportDocument{
		TargetUser: userID, LibraryID: libraryID, DocumentID: documentID, ContentVersionID: versionID,
		SourceName: source, DisplayName: displayName(source), Extension: extension(source),
		OriginalHash: hash, OriginalBytes: int64(len(content)), ContentHash: hash,
		ChunkCount: len(entries), CreatedAt: now,
	}
	chunks := make([]platformvector.Chunk, 0, len(entries))
	for index, entry := range entries {
		chunks = append(chunks, platformvector.Chunk{
			ID: platformvector.StableChunkID(documentID, versionID, index), Content: entry.Content,
			Vector: entry.Vector, OwnerUserID: userID, LibraryID: libraryID, DocumentID: documentID,
			ContentVersionID: versionID, ChunkIndex: index, SourceName: source,
			LocationLabel: metadataString(entry.Meta, "page_or_section"), Visibility: "private",
		})
	}
	return item, content, chunks
}

func backupSource(sourcePath, backupDir string, now time.Time) ([]byte, string, error) {
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, "", fmt.Errorf("读取旧知识库失败: %w", err)
	}
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return nil, "", fmt.Errorf("创建备份目录失败: %w", err)
	}
	name := fmt.Sprintf("%s.%s.bak", filepath.Base(sourcePath), now.UTC().Format("20060102T150405.000000000Z"))
	target := filepath.Join(backupDir, name)
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		return nil, "", fmt.Errorf("备份旧知识库失败: %w", err)
	}
	return raw, target, nil
}

func addFailure(report *Report, source, code, message string) {
	report.Failures = append(report.Failures, Failure{Source: source, Code: code, Message: message})
}

func sortedSources(groups map[string][]legacyEntry) []string {
	sources := make([]string, 0, len(groups))
	for source := range groups {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

func sourceName(metadata map[string]any) string {
	for _, key := range []string{"_source", "_file_name", platformvector.MetaSourceName} {
		if value := strings.TrimSpace(metadataString(metadata, key)); value != "" {
			return value
		}
	}
	return "unknown-source"
}

func isScopedEntry(metadata map[string]any) bool {
	return metadataString(metadata, platformvector.MetaOwnerUserID) != "" &&
		metadataString(metadata, platformvector.MetaContentVersion) != ""
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}

func validVector(vector []float64) bool {
	if len(vector) == 0 {
		return false
	}
	for _, value := range vector {
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return false
		}
	}
	return true
}

func displayName(source string) string {
	name := filepath.Base(filepath.Clean(source))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "旧知识资料.txt"
	}
	return name
}

func extension(source string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(displayName(source))), ".")
	if ext == "" || len(ext) > 16 {
		return "txt"
	}
	return ext
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
