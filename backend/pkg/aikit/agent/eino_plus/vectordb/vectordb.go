package vectordb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tencent/vectordatabase-sdk-go/tcvdbtext/encoder"
	"github.com/tencent/vectordatabase-sdk-go/tcvectordb"
)

// Client 封装腾讯云向量数据库，字段规范与 Python tencentdb.py 完全一致：
//
//	id          — String, PRIMARY_KEY, md5(content + partition_fields)
//	vector      — HNSW 向量索引
//	text        — 文档原文 (非索引字段，存储在 Fields 中)
//	meta_data   — Json, FILTER
//	sparse_vector — SparseVector, SPARSE_INVERTED (仅 hybrid 模式)
type Client struct {
	cfg    Config
	cli    *tcvectordb.RpcClient
	bm25   encoder.SparseEncoder
	bm25mu sync.Mutex
}

// New 创建向量数据库 Client.
// 参数：cfg - 客户端配置.
// 返回值：Client 实例, err - 创建或校验失败时的错误.
func New(cfg Config) (*Client, error) {
	cfg.Fix()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	cli, err := tcvectordb.NewRpcClient(
		cfg.URL,
		cfg.Username,
		cfg.Key,
		&tcvectordb.ClientOption{
			ReadConsistency: tcvectordb.EventualConsistency,
			Timeout:         time.Duration(cfg.Timeout) * time.Second,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("vectordb: create client: %w", err)
	}

	return &Client{cfg: cfg, cli: cli}, nil
}

// bm25Encoder 懒加载 BM25 编码器（仅 hybrid 模式使用）.
func (c *Client) bm25Encoder() (encoder.SparseEncoder, error) {
	c.bm25mu.Lock()
	defer c.bm25mu.Unlock()
	if c.bm25 != nil {
		return c.bm25, nil
	}
	enc, err := encoder.NewBM25Encoder(&encoder.BM25EncoderParams{Bm25Language: "zh"})
	if err != nil {
		return nil, fmt.Errorf("vectordb: init bm25: %w", err)
	}
	c.bm25 = enc
	return enc, nil
}

// Close 关闭底层 RPC 连接.
func (c *Client) Close() {
	if c.cli != nil {
		c.cli.Close()
	}
}

// Config 返回当前客户端配置.
// 返回值：Config 实例.
func (c *Client) Config() Config { return c.cfg }

// ---------- Collection lifecycle ----------.

func (c *Client) buildIndexes() tcvectordb.Indexes {
	idx := tcvectordb.Indexes{}

	idx.FilterIndex = append(idx.FilterIndex,
		tcvectordb.FilterIndex{FieldName: "id", FieldType: tcvectordb.String, IndexType: tcvectordb.PRIMARY},
		tcvectordb.FilterIndex{FieldName: "meta_data", FieldType: tcvectordb.Json, IndexType: tcvectordb.FILTER},
	)

	idx.VectorIndex = append(idx.VectorIndex, tcvectordb.VectorIndex{
		FilterIndex: tcvectordb.FilterIndex{FieldName: "vector", FieldType: tcvectordb.Vector, IndexType: tcvectordb.HNSW},
		Dimension:   uint32(c.cfg.Dimensions),
		MetricType:  tcvectordb.MetricType(c.cfg.Metric),
		Params:      &tcvectordb.HNSWParam{M: 16, EfConstruction: 200},
	})

	if c.cfg.SearchType == SearchTypeHybrid {
		idx.SparseVectorIndex = append(idx.SparseVectorIndex, tcvectordb.SparseVectorIndex{
			FieldName:  "sparse_vector",
			FieldType:  tcvectordb.SparseVector,
			IndexType:  tcvectordb.SPARSE_INVERTED,
			MetricType: tcvectordb.IP,
		})
	}

	return idx
}

// CreateCollection 创建数据库与集合（如不存在）.
// 返回值：err - 创建失败时的错误.
func (c *Client) CreateCollection(ctx context.Context) error {
	_, err := c.cli.CreateDatabaseIfNotExists(ctx, c.cfg.DBName)
	if err != nil {
		return fmt.Errorf("vectordb: create database: %w", err)
	}

	db := c.cli.Database(c.cfg.DBName)
	exists, err := db.ExistsCollection(ctx, c.cfg.CollectionName)
	if err != nil {
		return fmt.Errorf("vectordb: check collection: %w", err)
	}
	if exists {
		return nil
	}

	_, err = db.CreateCollection(ctx, c.cfg.CollectionName, 1, 1,
		"agno collection for document storage", c.buildIndexes())
	if err != nil {
		return fmt.Errorf("vectordb: create collection: %w", err)
	}
	return nil
}

// DropCollection 删除集合.
// 返回值：err - 删除失败时的错误.
func (c *Client) DropCollection(ctx context.Context) error {
	db := c.cli.Database(c.cfg.DBName)
	exists, err := db.ExistsCollection(ctx, c.cfg.CollectionName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	_, err = db.DropCollection(ctx, c.cfg.CollectionName)
	return err
}

// CollectionExists 检查集合是否存在.
// 返回值：exists - 是否存在, err - 查询失败时的错误.
func (c *Client) CollectionExists(ctx context.Context) (bool, error) {
	return c.cli.Database(c.cfg.DBName).ExistsCollection(ctx, c.cfg.CollectionName)
}

// ---------- Document operations ----------.

// Doc 是与 Python 端完全对齐的文档结构。
type Doc struct {
	Content   string         // 文档原文
	Embedding []float32      // 向量
	MetaData  map[string]any // 元数据 (会写入 meta_data JSON 字段)
	Name      string         // 便捷字段，存入 meta_data.name
}

// UpsertResult 返回 upsert 结果。
type UpsertResult struct {
	IDs           []string
	AffectedCount int
}

// Upsert 插入或更新文档，与 Python insert/upsert 行为一致（均调用 client.upsert）。
// filters 会 merge 到每个 doc 的 MetaData 中（filters 优先级更高）。
// Upsert 插入或更新文档，与 Python insert/upsert 行为一致 (均调用 client.upsert).
// 参数：ctx - 上下文, docs - 文档列表, filters - 合并到每个 doc MetaData 的过滤字段.
// 返回值：UpsertResult, err - 操作失败时的错误.
func (c *Client) Upsert(ctx context.Context, docs []Doc, filters map[string]any) (*UpsertResult, error) {
	var enc encoder.SparseEncoder
	if c.cfg.SearchType == SearchTypeHybrid {
		var err error
		enc, err = c.bm25Encoder()
		if err != nil {
			return nil, err
		}
	}

	tDocs := make([]tcvectordb.Document, 0, len(docs))
	ids := make([]string, 0, len(docs))

	for _, d := range docs {
		td, id := c.toTencentDoc(d, filters, enc)
		tDocs = append(tDocs, td)
		ids = append(ids, id)
	}

	res, err := c.cli.Upsert(ctx, c.cfg.DBName, c.cfg.CollectionName, tDocs)
	if err != nil {
		return nil, fmt.Errorf("vectordb: upsert: %w", err)
	}

	return &UpsertResult{IDs: ids, AffectedCount: res.AffectedCount}, nil
}

func (c *Client) toTencentDoc(d Doc, filters map[string]any, enc encoder.SparseEncoder) (tcvectordb.Document, string) {
	cleaned := cleanContent(d.Content)

	meta := make(map[string]any)
	for k, v := range d.MetaData {
		meta[k] = v
	}
	for k, v := range filters {
		meta[k] = v
	}
	if d.Name != "" {
		meta["name"] = d.Name
	}

	docID := computeDocID(cleaned, meta, c.cfg.PartitionFields)

	fields := map[string]tcvectordb.Field{
		"text":      {Val: cleaned},
		"meta_data": {Val: meta},
	}

	td := tcvectordb.Document{
		Id:     docID,
		Vector: d.Embedding,
		Fields: fields,
	}

	if enc != nil {
		if sv, err := enc.EncodeText(cleaned); err == nil {
			td.SparseVector = sv
		}
	}

	return td, docID
}

// ---------- Search ----------.

// SearchResult 对应 Python _tencent_to_document 返回结构。
type SearchResult struct {
	ID       string
	Content  string
	Name     string
	MetaData map[string]any
	Score    float32
}

// Search 根据配置的 SearchType 自动选择 vector 或 hybrid 搜索。
// Search 根据配置的 SearchType 自动选择 vector 或 hybrid 搜索.
// 参数：ctx - 上下文, queryEmbedding - 查询向量, query - 原始查询文本（hybrid 模式用于 BM25 编码）, limit - 返回数量上限, filters - 过滤条件.
// 返回值：SearchResult 列表, err - 搜索失败时的错误.
func (c *Client) Search(ctx context.Context, queryEmbedding []float32, query string, limit int, filters map[string]any) ([]SearchResult, error) {
	if c.cfg.SearchType == SearchTypeHybrid {
		enc, err := c.bm25Encoder()
		if err != nil {
			return nil, err
		}
		sv, err := enc.EncodeQuery(query)
		if err != nil {
			return nil, fmt.Errorf("vectordb: bm25 encode query: %w", err)
		}
		return c.HybridSearch(ctx, queryEmbedding, sv, limit, filters)
	}
	return c.VectorSearch(ctx, queryEmbedding, limit, filters)
}

// VectorSearch 执行纯向量搜索，对应 Python vector_search.
// 参数：ctx - 上下文, queryEmbedding - 查询向量, limit - 返回数量上限, filters - 过滤条件.
// 返回值：SearchResult 列表, err - 搜索失败时的错误.
func (c *Client) VectorSearch(ctx context.Context, queryEmbedding []float32, limit int, filters map[string]any) ([]SearchResult, error) {
	params := &tcvectordb.SearchDocumentParams{
		Params:         &tcvectordb.SearchDocParams{Ef: 200},
		RetrieveVector: false,
		Limit:          int64(limit),
	}
	if expr := buildFilterExpr(filters); expr != "" {
		params.Filter = tcvectordb.NewFilter(expr)
	}

	res, err := c.cli.Search(ctx, c.cfg.DBName, c.cfg.CollectionName,
		[][]float32{queryEmbedding}, params)
	if err != nil {
		return nil, fmt.Errorf("vectordb: vector search: %w", err)
	}

	if len(res.Documents) == 0 {
		return nil, nil
	}
	return toSearchResults(res.Documents[0]), nil
}

// HybridSearch 执行混合检索 (dense + sparse), 对应 Python hybrid_search.
// sparseVector 为 BM25 编码后的稀疏向量; 如果为 nil, 则仅使用 dense 向量.
// 参数：ctx - 上下文, queryEmbedding - dense 查询向量, sparseVector - sparse 查询向量, limit - 返回数量上限, filters - 过滤条件.
// 返回值：SearchResult 列表, err - 搜索失败时的错误.
func (c *Client) HybridSearch(ctx context.Context, queryEmbedding []float32, sparseVector any, limit int, filters map[string]any) ([]SearchResult, error) {
	l := limit
	hParams := tcvectordb.HybridSearchDocumentParams{
		RetrieveVector: false,
		Limit:          &l,
		AnnParams: []*tcvectordb.AnnParam{{
			FieldName: "vector",
			Data:      queryEmbedding,
			Limit:     &l,
		}},
		Rerank: &tcvectordb.RerankOption{
			Method:    tcvectordb.RerankWeighted,
			FieldList: []string{"vector", "sparse_vector"},
			Weight:    []float32{float32(c.cfg.HybridVectorWeight), float32(1 - c.cfg.HybridVectorWeight)},
		},
	}

	if sparseVector != nil {
		hParams.Match = []*tcvectordb.MatchOption{{
			FieldName:       "sparse_vector",
			Data:            sparseVector,
			Limit:           &l,
			TerminateAfter:  4000,
			CutoffFrequency: 0.99,
		}}
	}

	if expr := buildFilterExpr(filters); expr != "" {
		hParams.Filter = tcvectordb.NewFilter(expr)
	}

	res, err := c.cli.HybridSearch(ctx, c.cfg.DBName, c.cfg.CollectionName, hParams)
	if err != nil {
		return nil, fmt.Errorf("vectordb: hybrid search: %w", err)
	}
	if len(res.Documents) == 0 {
		return nil, nil
	}
	return toSearchResults(res.Documents[0]), nil
}

func toSearchResults(docs []tcvectordb.Document) []SearchResult {
	results := make([]SearchResult, 0, len(docs))
	for _, doc := range docs {
		r := SearchResult{
			ID:    doc.Id,
			Score: doc.Score,
		}

		if f, ok := doc.Fields["text"]; ok {
			r.Content = f.String()
		}

		meta := make(map[string]any)
		if f, ok := doc.Fields["meta_data"]; ok {
			if m, ok := f.Val.(map[string]any); ok {
				for k, v := range m {
					meta[k] = v
				}
			}
		}
		if name, ok := meta["name"]; ok {
			if s, ok := name.(string); ok {
				r.Name = s
			}
			delete(meta, "name")
		}
		r.MetaData = meta

		results = append(results, r)
	}
	return results
}

// ---------- Query / Exists ----------.

// DocExists 检查文档是否已存在 (基于内容+元数据的 ID).
// 参数：ctx - 上下文, content - 文档内容, meta - 元数据.
// 返回值：exists - 是否存在, err - 查询失败时的错误.
func (c *Client) DocExists(ctx context.Context, content string, meta map[string]any) (bool, error) {
	cleaned := cleanContent(content)
	docID := computeDocID(cleaned, meta, c.cfg.PartitionFields)
	return c.IDExists(ctx, docID)
}

// IDExists 检查指定 ID 的文档是否已存在.
// 参数：ctx - 上下文, id - 文档 ID.
// 返回值：exists - 是否存在, err - 查询失败时的错误.
func (c *Client) IDExists(ctx context.Context, id string) (bool, error) {
	res, err := c.cli.Query(ctx, c.cfg.DBName, c.cfg.CollectionName, []string{id},
		&tcvectordb.QueryDocumentParams{
			RetrieveVector: false,
			Limit:          1,
		})
	if err != nil {
		return false, fmt.Errorf("vectordb: id exists: %w", err)
	}
	return len(res.Documents) > 0, nil
}

// NameExists 检查指定名称的文档是否已存在.
// 参数：ctx - 上下文, name - 文档名称.
// 返回值：exists - 是否存在, err - 查询失败时的错误.
func (c *Client) NameExists(ctx context.Context, name string) (bool, error) {
	expr := fmt.Sprintf(`meta_data.name = "%s"`, name)
	res, err := c.cli.Query(ctx, c.cfg.DBName, c.cfg.CollectionName, nil,
		&tcvectordb.QueryDocumentParams{
			Filter:         tcvectordb.NewFilter(expr),
			RetrieveVector: false,
			Limit:          1,
		})
	if err != nil {
		return false, fmt.Errorf("vectordb: name exists: %w", err)
	}
	return len(res.Documents) > 0, nil
}

// ContentHashExists 检查指定内容哈希的文档是否已存在.
// 参数：ctx - 上下文, contentHash - 内容哈希值.
// 返回值：exists - 是否存在, err - 查询失败时的错误.
func (c *Client) ContentHashExists(ctx context.Context, contentHash string) (bool, error) {
	expr := fmt.Sprintf(`meta_data.content_hash = "%s"`, contentHash)
	res, err := c.cli.Query(ctx, c.cfg.DBName, c.cfg.CollectionName, nil,
		&tcvectordb.QueryDocumentParams{
			Filter:         tcvectordb.NewFilter(expr),
			RetrieveVector: false,
			Limit:          1,
		})
	if err != nil {
		return false, fmt.Errorf("vectordb: content_hash exists: %w", err)
	}
	return len(res.Documents) > 0, nil
}

// Count 返回集合中的文档总数.
// 返回值：count - 文档数量, err - 查询失败时的错误.
func (c *Client) Count(ctx context.Context) (int64, error) {
	res, err := c.cli.Count(ctx, c.cfg.DBName, c.cfg.CollectionName)
	if err != nil {
		return 0, err
	}
	return int64(res.Count), nil
}

// ---------- Delete ----------.

const maxDeleteLimit = 16384

// DeleteByID 按 ID 删除文档.
// 参数：ctx - 上下文, ids - 要删除的文档 ID 列表.
// 返回值：affectedCount - 受影响行数, err - 删除失败时的错误.
func (c *Client) DeleteByID(ctx context.Context, ids ...string) (int, error) {
	res, err := c.cli.Delete(ctx, c.cfg.DBName, c.cfg.CollectionName, tcvectordb.DeleteDocumentParams{
		DocumentIds: ids,
		Limit:       int64(len(ids)),
	})
	if err != nil {
		return 0, fmt.Errorf("vectordb: delete by id: %w", err)
	}
	return res.AffectedCount, nil
}

// DeleteByName 按文档名称删除.
// 参数：ctx - 上下文, name - 文档名称.
// 返回值：affectedCount - 受影响行数, err - 删除失败时的错误.
func (c *Client) DeleteByName(ctx context.Context, name string) (int, error) {
	expr := fmt.Sprintf(`meta_data.name = "%s"`, name)
	return c.deleteByFilter(ctx, expr)
}

// DeleteByContentID 按内容 ID 删除.
// 参数：ctx - 上下文, contentID - 内容 ID.
// 返回值：affectedCount - 受影响行数, err - 删除失败时的错误.
func (c *Client) DeleteByContentID(ctx context.Context, contentID string) (int, error) {
	expr := fmt.Sprintf(`meta_data.content_id = "%s"`, contentID)
	return c.deleteByFilter(ctx, expr)
}

// DeleteByMetadata 按元数据过滤条件删除.
// 参数：ctx - 上下文, metadata - 过滤条件.
// 返回值：affectedCount - 受影响行数, err - 删除失败时的错误.
func (c *Client) DeleteByMetadata(ctx context.Context, metadata map[string]any) (int, error) {
	expr := buildFilterExpr(metadata)
	if expr == "" {
		return 0, fmt.Errorf("vectordb: no valid metadata for deletion")
	}
	return c.deleteByFilter(ctx, expr)
}

func (c *Client) deleteByFilter(ctx context.Context, expr string) (int, error) {
	res, err := c.cli.Delete(ctx, c.cfg.DBName, c.cfg.CollectionName, tcvectordb.DeleteDocumentParams{
		Filter: tcvectordb.NewFilter(expr),
		Limit:  maxDeleteLimit,
	})
	if err != nil {
		return 0, fmt.Errorf("vectordb: delete by filter [%s]: %w", expr, err)
	}
	return res.AffectedCount, nil
}

// ---------- Eino adapter helpers ----------.

// ToEinoDocID 暴露 doc ID 计算，供 eino Indexer adapter 使用。
func (c *Client) ToEinoDocID(content string, meta map[string]any) string {
	return computeDocID(cleanContent(content), meta, c.cfg.PartitionFields)
}

// FilterExpr 暴露 filter 构建，供外部组合更复杂的查询。
func FilterExpr(filters map[string]any) string {
	return buildFilterExpr(filters)
}

// ---------- helpers ----------.
