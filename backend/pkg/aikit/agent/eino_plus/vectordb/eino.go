package vectordb

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

// compile-time interface checks.
var (
	_ indexer.Indexer     = (*EinoIndexer)(nil)
	_ retriever.Retriever = (*EinoRetriever)(nil)
)

// EinoIndexer 实现 eino Indexer 接口，将 schema.Document 写入腾讯云向量数据库.
type EinoIndexer struct {
	client   *Client
	embedder embedding.Embedder
}

// NewEinoIndexer 创建 EinoIndexer 实例.
// 参数：client - 向量数据库客户端, embedder - 嵌入模型.
// 返回值：EinoIndexer 实例.
func NewEinoIndexer(client *Client, embedder embedding.Embedder) *EinoIndexer {
	return &EinoIndexer{client: client, embedder: embedder}
}

// Store 实现 indexer.Indexer.
// 如果 document 没有嵌入向量，会自动调用 embedder 生成.
// 参数：ctx - 上下文, docs - 待索引的文档列表, opts - indexer 选项.
// 返回值：ids - 文档 ID 列表, err - 索引失败时的错误.
func (e *EinoIndexer) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
	texts := make([]string, 0, len(docs))
	for _, d := range docs {
		if d == nil {
			continue
		}
		texts = append(texts, d.Content)
	}

	var embeddings [][]float64
	if len(texts) > 0 && e.embedder != nil {
		var err error
		embeddings, err = e.embedder.EmbedStrings(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("vectordb/eino: embed: %w", err)
		}
	}

	vDocs := make([]Doc, 0, len(docs))
	embIdx := 0
	for _, d := range docs {
		if d == nil {
			continue
		}
		vd := Doc{
			Content:  d.Content,
			MetaData: d.MetaData,
		}
		if d.ID != "" {
			if vd.MetaData == nil {
				vd.MetaData = make(map[string]any)
			}
			vd.MetaData["eino_id"] = d.ID
		}
		if name, ok := d.MetaData["name"]; ok {
			if s, ok := name.(string); ok {
				vd.Name = s
			}
		}
		if embIdx < len(embeddings) {
			vd.Embedding = toFloat32(embeddings[embIdx])
			embIdx++
		}
		vDocs = append(vDocs, vd)
	}

	res, err := e.client.Upsert(ctx, vDocs, nil)
	if err != nil {
		return nil, err
	}
	return res.IDs, nil
}

// EinoRetriever 实现 eino Retriever 接口，从腾讯云向量数据库检索文档.
type EinoRetriever struct {
	client   *Client
	embedder embedding.Embedder
	topK     int
	filters  map[string]any
}

// RetrieverOption EinoRetriever 配置选项函数.
type RetrieverOption func(*EinoRetriever)

// WithTopK 设置检索返回的最大文档数.
// 参数：k - 返回数量.
// 返回值：RetrieverOption 函数.
func WithTopK(k int) RetrieverOption {
	return func(r *EinoRetriever) { r.topK = k }
}

// WithFilters 设置检索过滤条件.
// 参数：f - 过滤条件字典.
// 返回值：RetrieverOption 函数.
func WithFilters(f map[string]any) RetrieverOption {
	return func(r *EinoRetriever) { r.filters = f }
}

// NewEinoRetriever 创建 EinoRetriever 实例.
// 参数：client - 向量数据库客户端, embedder - 嵌入模型, opts - 配置选项.
// 返回值：EinoRetriever 实例.
func NewEinoRetriever(client *Client, embedder embedding.Embedder, opts ...RetrieverOption) *EinoRetriever {
	r := &EinoRetriever{
		client:   client,
		embedder: embedder,
		topK:     5,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Retrieve 实现 retriever.Retriever.
// 参数：ctx - 上下文, query - 查询文本, opts - retriever 选项.
// 返回值：schema.Document 列表, err - 检索失败时的错误.
func (e *EinoRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	if e.embedder == nil {
		return nil, fmt.Errorf("vectordb/eino: embedder is required for retrieval")
	}

	vecs, err := e.embedder.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("vectordb/eino: embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, nil
	}

	results, err := e.client.Search(ctx, toFloat32(vecs[0]), query, e.topK, e.filters)
	if err != nil {
		return nil, err
	}

	schemaDocs := make([]*schema.Document, 0, len(results))
	for _, r := range results {
		meta := r.MetaData
		if meta == nil {
			meta = make(map[string]any)
		}
		meta["score"] = r.Score
		if r.Name != "" {
			meta["name"] = r.Name
		}
		schemaDocs = append(schemaDocs, &schema.Document{
			ID:       r.ID,
			Content:  r.Content,
			MetaData: meta,
		})
	}
	return schemaDocs, nil
}

func toFloat32(f64 []float64) []float32 {
	f32 := make([]float32, len(f64))
	for i, v := range f64 {
		f32[i] = float32(v)
	}
	return f32
}
