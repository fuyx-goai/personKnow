// mem.go —— 手写内存向量库（本项目最有含金量的一步）
//
// 先手写一个内存版，既能零依赖跑通全流程，又能真正看懂向量检索的原理；
// 等原理吃透了，再把生产里常用的 Milvus 接进来（见 milvus.go），
// 两者共用同一个 VectorStore 接口，换个配置就能切换。
package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

// storeFile 知识库落盘文件：摄入后保存在本地，下次问答不用重新灌数据
const storeFile = "knowledge.json"

// entry 落盘时每条知识的存储结构
type entry struct {
	ID      string         `json:"id"`
	Content string         `json:"content"`        // 小段正文
	Vector  []float64      `json:"vector"`         // 这段话的向量
	Meta    map[string]any `json:"meta,omitempty"` // 元数据（来源文件等）
}

// MemStore 内存向量库
type MemStore struct {
	mu    sync.RWMutex       // 读写锁：防多个 goroutine 同时读写数据
	items []entry            // 所有已入库的小段
	emb   embedding.Embedder // 查询时把"问题"也变成向量，需要用到 Embedder
}

// 编译期断言：MemStore 必须满足 VectorStore 接口
// （如果方法签名写错了，编译期立刻报错，这是 Go 的常用技巧）
var _ VectorStore = (*MemStore)(nil)

// NewMemStore 创建向量库：先尝试加载本地已保存的数据
func NewMemStore(ctx context.Context, emb embedding.Embedder) (*MemStore, error) {
	s := &MemStore{emb: emb}

	// 如果之前摄入过（knowledge.json 存在），把数据读回内存
	if raw, err := os.ReadFile(storeFile); err == nil {
		if err := json.Unmarshal(raw, &s.items); err != nil {
			return nil, fmt.Errorf("解析知识库文件 %s 失败: %w", storeFile, err)
		}
	}
	return s, nil
}

// Store 实现 indexer.Indexer 接口：把切分并算好向量的文档存起来
//
// 注意：按照 Eino 的约定，向量由调用方算好，放在 doc.WithDenseVector() 里传进来；
// 我们把它取出来存进自己的 entry。
func (s *MemStore) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		vec := doc.DenseVector() // 取出该文档的向量
		if len(vec) == 0 {
			return nil, fmt.Errorf("文档 %q 没有向量，请先调用 Embedder 再入库", doc.ID)
		}
		id := doc.ID
		if id == "" {
			id = contentID(doc.Content)
		}
		item := entry{ID: id, Content: doc.Content, Vector: vec, Meta: cloneMeta(doc.MetaData)}
		if index := s.indexOf(id); index >= 0 {
			s.items[index] = item
		} else {
			s.items = append(s.items, item)
		}
		ids = append(ids, id)
	}

	// 每次入库后立刻落盘，防止程序退出丢数据
	if err := s.save(); err != nil {
		return nil, err
	}
	return ids, nil
}

// Retrieve 实现 retriever.Retriever 接口：语义搜索
//
// 流程：问题 -> 向量化 -> 与库中每段算余弦相似度 -> 按分数从高到低返回前 TopK 段
func (s *MemStore) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	// 1. 把用户的问题也变成向量（和入库时用同一个 Embedder，否则向量对不上）
	vecs, err := s.emb.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}
	qv := vecs[0]

	// 2. 遍历库中所有小段，计算相似度
	s.mu.RLock()
	items := append([]entry(nil), s.items...)
	s.mu.RUnlock()
	type scored struct {
		idx   int
		score float64
	}
	hits := make([]scored, 0, len(items))
	for i, it := range items {
		hits = append(hits, scored{idx: i, score: cosine(qv, it.Vector)})
	}

	// 3. 按相似度从高到低排序
	sort.Slice(hits, func(a, b int) bool { return hits[a].score > hits[b].score })

	// 4. 从选项里取 TopK（不传默认取 3 段）
	k := 3
	if o := retriever.GetCommonOptions(&retriever.Options{}, opts...); o != nil && o.TopK != nil {
		k = *o.TopK
	}
	if k > len(hits) {
		k = len(hits)
	}

	// 5. 组装返回值：带分数的 schema.Document 列表
	docs := make([]*schema.Document, 0, k)
	for _, h := range hits[:k] {
		docs = append(docs, (&schema.Document{
			ID:       items[h.idx].ID,
			Content:  items[h.idx].Content,
			MetaData: cloneMeta(items[h.idx].Meta),
		}).WithScore(h.score)) // 把相似度分数写进文档，调用方可以拿来展示
	}
	return docs, nil
}

// Count 返回已入库的片段数量
func (s *MemStore) Count(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items), nil
}

// List 列出已入库片段（供"藏书"页浏览）
//
// 只带正文与元数据，不带向量：前端只是"看"，用不上那几千个浮点数。
func (s *MemStore) List(ctx context.Context, limit int) ([]*schema.Document, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	n := len(s.items)
	if n > limit {
		n = limit
	}
	docs := make([]*schema.Document, 0, n)
	for i := 0; i < n; i++ {
		docs = append(docs, &schema.Document{
			ID:       s.items[i].ID,
			Content:  s.items[i].Content,
			MetaData: cloneMeta(s.items[i].Meta),
		})
	}
	return docs, nil
}

func (s *MemStore) DeleteByIDs(ctx context.Context, ids []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	kept := make([]entry, 0, len(s.items))
	for _, item := range s.items {
		if _, exists := wanted[item.ID]; !exists {
			kept = append(kept, item)
		}
	}
	deleted := len(s.items) - len(kept)
	if deleted == 0 {
		return 0, nil
	}
	s.items = kept
	if err := s.save(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *MemStore) indexOf(id string) int {
	for index := range s.items {
		if s.items[index].ID == id {
			return index
		}
	}
	return -1
}

// DeleteBySource 按来源文件名删除片段，返回删除条数；source 为空表示清空整库
func (s *MemStore) DeleteBySource(ctx context.Context, source string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 重建切片：留下没被删的（Go 里没有"删除元素"的原地操作，重建最直观）
	kept := make([]entry, 0, len(s.items))
	deleted := 0
	for _, it := range s.items {
		if source == "" || sourceOf(it.Meta) == source {
			deleted++
			continue
		}
		kept = append(kept, it)
	}
	if deleted == 0 {
		return 0, nil // 没删到东西，不必白写一次磁盘
	}

	s.items = kept
	if err := s.save(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// save 把内存数据写到本地文件
func (s *MemStore) save() error {
	raw, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(storeFile, raw, 0o644)
}
