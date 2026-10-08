// milvus.go —— Milvus 向量库实现（可选，默认不启用）
//
// Milvus 是业界最常用的开源向量数据库，单机版一条 Docker 命令就能起：
//
//	docker run -d --name milvus -p 19530:19530 -p 9091:9091 \
//	  -e ETCD_USE_EMBED=true -e COMMON_STORAGETYPE=local \
//	  milvusdb/milvus:v2.4.15 milvus run standalone
//
// 启用方式：把 configs/config.yaml 里的 vector_store 改成 milvus，并填好 milvus 那一段参数
//
// 这里走的是 Milvus 官方的 RESTful API（/v2/vectordb/... 和 gRPC 共用 19530 端口），
// 好处是不用引入 gRPC/Protobuf 一整套重型依赖，纯 net/http 就能把向量库用起来。
//
// 本文件只做一件事：把 Milvus 包装成和 MemStore 一模一样的 VectorStore 接口，
// 于是摄入链、问答链、HTTP 层全都不用改，改个配置就能从"内存版"切到"生产版"。
//
// 集合结构（不存在会自动创建，并开启动态字段以便存 content/metadata）：
//
//	id        主键（用内容哈希，天然去重）
//	vector    向量（维度由 milvus.dim 指定，必须与 Embedding 模型输出一致）
//	content   原文片段
//	metadata  元数据（来源文件名等）
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
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"knowledge-base/pkg/config"
)

// 集合字段名（建表、插入、检索时保持一致）
const (
	milvusFieldID       = "id"
	milvusFieldVector   = "vector"
	milvusFieldContent  = "content"
	milvusFieldMetadata = "metadata"

	milvusMaxIDLen = 255

	// milvusDeleteScanLimit 删除时一次最多扫描多少行
	// 个人知识库的规模远小于这个数；真到了几十万条，应改成用过滤表达式在服务端筛
	milvusDeleteScanLimit = 10000
)

// MilvusStore 基于 Milvus 的向量库
type MilvusStore struct {
	baseURL    string             // 形如 http://127.0.0.1:19530
	token      string             // 认证用（username:password），未开启鉴权时为空
	dbName     string             // 数据库名
	collection string             // 集合名
	dim        int                // 向量维度
	metric     string             // 相似度度量：COSINE / IP / L2
	emb        embedding.Embedder // 检索时把问题向量化，需与入库用同一个模型
	http       *http.Client
}

// 编译期断言：MilvusStore 同样满足 VectorStore 接口
var _ VectorStore = (*MilvusStore)(nil)

// NewMilvusStore 连接 Milvus 并确保集合就绪
func NewMilvusStore(ctx context.Context, cfg config.MilvusConfig, emb embedding.Embedder) (*MilvusStore, error) {
	// 1. 配置自检：把常见填错的地方提前拦住，给出人话提示
	if cfg.Address == "" {
		return nil, fmt.Errorf("未配置 Milvus 地址：请在 configs/config.yaml 的 milvus.address 填写（如 127.0.0.1:19530），"+
			"或把 vector_store 改回 %q 用内存版", config.StoreMem)
	}
	if cfg.Dim <= 0 {
		return nil, fmt.Errorf("Milvus 向量维度不合法（%d）：请检查 configs/config.yaml 的 milvus.dim，"+
			"且必须与向量化模型的输出维度一致", cfg.Dim)
	}
	if cfg.Collection == "" {
		return nil, fmt.Errorf("未配置 Milvus 集合名：请检查 configs/config.yaml 的 milvus.collection")
	}

	// 2. 拼 baseURL：允许只写 host:port，也允许写完整 http:// 地址
	addr := strings.TrimSpace(cfg.Address)
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	addr = strings.TrimRight(addr, "/")

	s := &MilvusStore{
		baseURL:    addr,
		token:      strings.TrimSpace(cfg.Username + ":" + cfg.Password),
		dbName:     cfg.DBName,
		collection: cfg.Collection,
		dim:        cfg.Dim,
		metric:     normalizeMilvusMetric(cfg.Metric),
		emb:        emb,
		http:       &http.Client{Timeout: 60 * time.Second},
	}

	// 3. 建表 + 加载，任一步失败都说明配置或服务有问题，直接返回友好错误
	if err := s.ensureCollection(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// ensureCollection 集合不存在就创建（建表语句里同时指定了向量索引参数），并加载进内存
func (s *MilvusStore) ensureCollection(ctx context.Context) error {
	// 3.1 先列一遍已有集合（顺便起到"连通性检查"的作用）
	var names []string
	if err := s.post(ctx, "/v2/vectordb/collections/list", map[string]any{
		"dbName": s.dbName,
	}, &names); err != nil {
		return fmt.Errorf("连接 Milvus（%s）失败，请确认服务已启动: %w", s.baseURL, err)
	}
	for _, n := range names {
		if n == s.collection {
			// 已存在：直接加载（数据必须先 Load 进内存才能被检索）
			return s.loadCollection(ctx)
		}
	}

	// 3.2 建表：快速建表模式只需给出维度、度量方式、主键与向量字段名
	//     enableDynamicField 打开后，content / metadata 这类"没在表结构里声明"的字段也能直接插入
	if err := s.post(ctx, "/v2/vectordb/collections/create", map[string]any{
		"dbName":             s.dbName,
		"collectionName":     s.collection,
		"dimension":          s.dim,
		"metricType":         s.metric,
		"primaryFieldName":   milvusFieldID,
		"idType":             "VarChar",
		"vectorFieldName":    milvusFieldVector,
		"autoID":             false,
		"enableDynamicField": true,
		"params":             map[string]any{"max_length": milvusMaxIDLen},
		"consistencyLevel":   "Strong",
	}, nil); err != nil {
		return fmt.Errorf("创建集合 %s 失败: %w", s.collection, err)
	}

	// 3.3 加载
	return s.loadCollection(ctx)
}

// loadCollection 把集合加载进内存（Milvus 的数据必须先 Load 才能检索）
func (s *MilvusStore) loadCollection(ctx context.Context) error {
	if err := s.post(ctx, "/v2/vectordb/collections/load", map[string]any{
		"dbName":         s.dbName,
		"collectionName": s.collection,
	}, nil); err != nil {
		return fmt.Errorf("加载集合 %s 失败: %w", s.collection, err)
	}
	return nil
}

// Store 实现 indexer.Indexer：把切分好的文档写进 Milvus
//
// 与 MemStore 的约定完全一致：向量由上游（摄入链的 Lambda 节点）算好，
// 用 doc.WithDenseVector() 传进来，这里只负责写库，不重复调用 Embedding API。
func (s *MilvusStore) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	rows := make([]map[string]any, 0, len(docs))
	ids := make([]string, 0, len(docs))

	for _, doc := range docs {
		vec := doc.DenseVector()
		if len(vec) == 0 {
			return nil, fmt.Errorf("文档 %q 没有向量，请先调用 Embedder 再入库", doc.ID)
		}
		if len(vec) != s.dim {
			return nil, fmt.Errorf("向量维度不匹配：文档是 %d 维，集合 %s 需要 %d 维"+
				"（请把 configs/config.yaml 的 milvus.dim 改成与 Embedding 模型一致的维度）",
				len(vec), s.collection, s.dim)
		}

		// Milvus 的 float 向量统一用 float32 精度传输
		f32 := make([]float32, len(vec))
		for i, v := range vec {
			f32[i] = float32(v)
		}

		id := doc.ID
		if id == "" {
			// 用内容哈希当主键：同样的内容重复摄入只会是同一个 ID，天然去重
			id = contentID(doc.Content)
		}

		rows = append(rows, map[string]any{
			milvusFieldID:       id,
			milvusFieldVector:   f32,
			milvusFieldContent:  doc.Content,
			milvusFieldMetadata: doc.MetaData, // 动态字段，直接塞 map
		})
		ids = append(ids, id)
	}

	// 批量插入（Milvus 对批量写入很友好，一次几百上千条都没问题）
	if err := s.post(ctx, "/v2/vectordb/entities/insert", map[string]any{
		"dbName":         s.dbName,
		"collectionName": s.collection,
		"data":           rows,
	}, nil); err != nil {
		return nil, fmt.Errorf("写入 Milvus 失败: %w", err)
	}
	return ids, nil
}

// Retrieve 实现 retriever.Retriever：语义检索
//
// 流程：问题 -> 向量化 -> Milvus 向量检索（按 metric 算相似度）-> 前 TopK 段
func (s *MilvusStore) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	// 1. 把问题也变成向量（必须和入库用同一个 Embedder，否则向量不在同一空间）
	vecs, err := s.emb.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("问题向量化结果异常：期望 1 条，实际 %d 条", len(vecs))
	}
	f32 := make([]float32, len(vecs[0]))
	for i, v := range vecs[0] {
		f32[i] = float32(v)
	}

	// 2. 取 TopK（不传默认 3 段，与 MemStore 保持一致）
	k := 3
	if o := retriever.GetCommonOptions(&retriever.Options{}, opts...); o != nil && o.TopK != nil {
		k = *o.TopK
	}

	// 3. 检索：data 是"查询向量列表"，outputFields 指定要带回来的字段
	var hits []milvusHit
	if err := s.post(ctx, "/v2/vectordb/entities/search", map[string]any{
		"dbName":         s.dbName,
		"collectionName": s.collection,
		"data":           [][]float32{f32},
		"annsField":      milvusFieldVector,
		"limit":          k,
		"outputFields":   []string{milvusFieldContent, milvusFieldMetadata},
	}, &hits); err != nil {
		return nil, fmt.Errorf("Milvus 检索失败: %w", err)
	}

	// 4. 把返回结果转成 Eino 的 Document（Milvus 已按相似度排好序）
	docs := make([]*schema.Document, 0, len(hits))
	for _, h := range hits {
		doc := &schema.Document{
			ID:       h.ID,
			Content:  h.content(),
			MetaData: h.meta(),
		}
		// 把相似度写进文档，前端可以展示"命中依据"
		docs = append(docs, doc.WithScore(h.Distance))
	}
	return docs, nil
}

// Count 返回集合中的片段数量（用于 /api/stats）
func (s *MilvusStore) Count(ctx context.Context) (int, error) {
	var rows []map[string]any
	if err := s.post(ctx, "/v2/vectordb/entities/query", map[string]any{
		"dbName":         s.dbName,
		"collectionName": s.collection,
		"filter":         "",
		"outputFields":   []string{"count(*)"},
	}, &rows); err != nil {
		return 0, fmt.Errorf("统计 Milvus 片段数失败: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	// 返回形如 [{"count(*)": 12}]，数值可能是 float64 也可能是 json.Number
	switch v := rows[0]["count(*)"].(type) {
	case float64:
		return int(v), nil
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("解析片段数失败: %w", err)
		}
		return int(n), nil
	default:
		return 0, nil
	}
}

// List 列出已入库片段（供"藏书"页浏览），同样不返回向量
func (s *MilvusStore) List(ctx context.Context, limit int) ([]*schema.Document, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}

	var rows []map[string]any
	if err := s.post(ctx, "/v2/vectordb/entities/query", map[string]any{
		"dbName":         s.dbName,
		"collectionName": s.collection,
		"filter":         "", // 空表达式 = 不筛选，即查全表
		"outputFields":   []string{milvusFieldID, milvusFieldContent, milvusFieldMetadata},
		"limit":          limit,
	}, &rows); err != nil {
		return nil, fmt.Errorf("列出 Milvus 片段失败: %w", err)
	}

	docs := make([]*schema.Document, 0, len(rows))
	for _, row := range rows {
		docs = append(docs, rowToDocument(row))
	}
	return docs, nil
}

// DeleteBySource 按来源文件名删除片段，返回删除条数；source 为空表示清空整库
//
// 实现上的取舍：metadata 是 Milvus 的动态字段，用过滤表达式去匹配里面的
// _file_name 时，"$meta[...]"/"metadata[...]" 之类的 JSON 路径写法跨版本有差异，
// 写错了还很难排查。这里改用"先查后删"——先把主键查回来，在 Go 里按来源过滤，
// 再按主键批量删除，只依赖最基础的 query + delete 两个接口，行为完全可预期。
func (s *MilvusStore) DeleteBySource(ctx context.Context, source string) (int, error) {
	docs, err := s.List(ctx, milvusDeleteScanLimit)
	if err != nil {
		return 0, err
	}

	ids := make([]string, 0, len(docs))
	for _, d := range docs {
		if d.ID == "" {
			continue
		}
		if source == "" || sourceOf(d.MetaData) == source {
			ids = append(ids, d.ID)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}

	var res struct {
		DeleteCount int `json:"deleteCount"`
	}
	if err := s.post(ctx, "/v2/vectordb/entities/delete", map[string]any{
		"dbName":         s.dbName,
		"collectionName": s.collection,
		"ids":            ids, // 按主键删除，不必写过滤表达式
	}, &res); err != nil {
		return 0, fmt.Errorf("删除 Milvus 片段失败: %w", err)
	}
	return res.DeleteCount, nil
}

// rowToDocument 把 query 接口返回的一行转成 Eino 文档
func rowToDocument(row map[string]any) *schema.Document {
	doc := &schema.Document{}
	if v, ok := row[milvusFieldID].(string); ok {
		doc.ID = v
	}
	if v, ok := row[milvusFieldContent].(string); ok {
		doc.Content = v
	}
	// 动态字段的返回形态不固定：可能是对象，也可能是 JSON 字符串，两种都兼容
	switch v := row[milvusFieldMetadata].(type) {
	case map[string]any:
		doc.MetaData = v
	case string:
		var m map[string]any
		if err := json.Unmarshal([]byte(v), &m); err == nil {
			doc.MetaData = m
		}
	}
	return doc
}

// milvusHit 检索返回的一条命中结果
//
// Milvus 不同版本/接口的返回结构略有差异：outputFields 有时嵌在 entity 里，
// 有时平铺在同一层。这里两种都兼容，取到即可。
type milvusHit struct {
	ID       string         `json:"id"`
	Distance float64        `json:"distance"`
	Entity   map[string]any `json:"entity"`
	Extra    map[string]any `json:"-"`
}

// UnmarshalJSON 自定义解析：把 entity 之外的字段也收进 Extra，方便兼容平铺结构
func (h *milvusHit) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["id"].(string); ok {
		h.ID = v
	}
	switch v := raw["distance"].(type) {
	case float64:
		h.Distance = v
	case json.Number:
		f, _ := v.Float64()
		h.Distance = f
	}
	if e, ok := raw["entity"].(map[string]any); ok {
		h.Entity = e
	}
	h.Extra = raw
	return nil
}

// content 取命中片段的正文
func (h *milvusHit) content() string {
	if h.Entity != nil {
		if v, ok := h.Entity[milvusFieldContent].(string); ok {
			return v
		}
	}
	if v, ok := h.Extra[milvusFieldContent].(string); ok {
		return v
	}
	return ""
}

// meta 取命中片段的元数据
func (h *milvusHit) meta() map[string]any {
	for _, m := range []map[string]any{h.Entity, h.Extra} {
		if m == nil {
			continue
		}
		if v, ok := m[milvusFieldMetadata].(map[string]any); ok {
			return v
		}
	}
	return map[string]any{}
}

// post 统一发起 Milvus REST 请求，并把 {"code":0,"data":...} 里的 data 解出来
//
// Milvus 的 RESTful 响应固定是 {code, message, data} 三段式：
//
//	code = 0  表示成功，data 里才是真正的业务数据
//	code != 0 表示业务失败，message 里是原因
func (s *MilvusStore) post(ctx context.Context, path string, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.token != "" && s.token != ":" {
		// Milvus 的鉴权头就是 "Bearer 用户名:密码"
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("请求 Milvus 失败: %w", err)
	}
	defer resp.Body.Close()

	// 限制读取大小，避免异常响应把内存撑爆
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("读取 Milvus 响应失败: %w", err)
	}

	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("Milvus 返回的不是预期格式（HTTP %d）：%s", resp.StatusCode, truncate(string(raw), 300))
	}
	if env.Code != 0 {
		return fmt.Errorf("Milvus 返回错误（code=%d）：%s", env.Code, env.Message)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("Milvus 返回 HTTP %d：%s", resp.StatusCode, truncate(string(raw), 300))
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("解析 Milvus 数据失败: %w", err)
	}
	return nil
}

// normalizeMilvusMetric 归一化度量方式：只认 COSINE / IP / L2，其它一律按余弦处理
func normalizeMilvusMetric(name string) string {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "IP":
		return "IP"
	case "L2":
		return "L2"
	default:
		return "COSINE" // 文本向量最常用余弦相似度
	}
}

// contentID 用内容算一个稳定的短 ID（40 位以内，满足主键长度限制）
func contentID(content string) string {
	sum := sha1.Sum([]byte(content))
	return hex.EncodeToString(sum[:]) // 40 个字符，远小于 varchar(255)
}

// truncate 截断过长的字符串，避免把整页 HTML 塞进错误信息里
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
