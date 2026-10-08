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
	"context"
	"encoding/json"
	"fmt"
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
