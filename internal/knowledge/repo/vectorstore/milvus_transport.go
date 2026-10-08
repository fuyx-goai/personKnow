package vectorstore

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/schema"
)

func rowToDocument(row map[string]any) *schema.Document {
	document := &schema.Document{}
	if value, ok := row[milvusFieldID].(string); ok {
		document.ID = value
	}
	if value, ok := row[milvusFieldContent].(string); ok {
		document.Content = value
	}
	switch value := row[milvusFieldMetadata].(type) {
	case map[string]any:
		document.MetaData = value
	case string:
		_ = json.Unmarshal([]byte(value), &document.MetaData)
	}
	return document
}

type milvusHit struct {
	ID       string         `json:"id"`
	Distance float64        `json:"distance"`
	Entity   map[string]any `json:"entity"`
	Extra    map[string]any `json:"-"`
}

func (hit *milvusHit) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	hit.ID, _ = raw["id"].(string)
	switch value := raw["distance"].(type) {
	case float64:
		hit.Distance = value
	case json.Number:
		hit.Distance, _ = value.Float64()
	}
	hit.Entity, _ = raw["entity"].(map[string]any)
	hit.Extra = raw
	return nil
}

func (hit *milvusHit) content() string {
	for _, fields := range []map[string]any{hit.Entity, hit.Extra} {
		if value, ok := fields[milvusFieldContent].(string); ok {
			return value
		}
	}
	return ""
}

func (hit *milvusHit) meta() map[string]any {
	for _, fields := range []map[string]any{hit.Entity, hit.Extra} {
		if value, ok := fields[milvusFieldMetadata].(map[string]any); ok {
			return value
		}
	}
	return map[string]any{}
}

func (s *MilvusStore) post(ctx context.Context, path string, body any, out any) error {
	buffer, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+path, bytes.NewReader(buffer))
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if s.token != "" && s.token != ":" {
		request.Header.Set("Authorization", "Bearer "+s.token)
	}
	response, err := s.http.Do(request)
	if err != nil {
		return fmt.Errorf("请求 Milvus 失败: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("读取 Milvus 响应失败: %w", err)
	}
	return decodeMilvusResponse(response.StatusCode, raw, out)
}

func decodeMilvusResponse(status int, raw []byte, out any) error {
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("Milvus 返回的不是预期格式（HTTP %d）：%s", status, truncate(string(raw), 300))
	}
	if envelope.Code != 0 {
		return fmt.Errorf("Milvus 返回错误（code=%d）：%s", envelope.Code, envelope.Message)
	}
	if status >= http.StatusBadRequest {
		return fmt.Errorf("Milvus 返回 HTTP %d：%s", status, truncate(string(raw), 300))
	}
	if out == nil || len(envelope.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("解析 Milvus 数据失败: %w", err)
	}
	return nil
}

func normalizeMilvusMetric(name string) string {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "IP":
		return "IP"
	case "L2":
		return "L2"
	default:
		return "COSINE"
	}
}

func contentID(content string) string {
	sum := sha1.Sum([]byte(content))
	return hex.EncodeToString(sum[:])
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
