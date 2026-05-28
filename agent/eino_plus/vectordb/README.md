# vectordb — 腾讯云向量数据库客户端

封装 [Tencent VectorDB Go SDK](https://github.com/Tencent/vectordatabase-sdk-go)，字段规范与 Python `aikit/agno_plus/tencentdb.py` 完全对齐。

同时实现 [eino](https://github.com/cloudwego/eino) 框架的 `Indexer` 和 `Retriever` 接口，可直接用于 RAG pipeline 编排。

## 字段规范

| 字段名 | 类型 | 索引 | 说明 |
|--------|------|------|------|
| `id` | String | PRIMARY_KEY | md5(content + partition_fields) |
| `vector` | Vector | HNSW (m=16, ef=200) | 文档向量 |
| `text` | - | 无 (payload) | 文档原文 |
| `meta_data` | Json | FILTER | 元数据 JSON |
| `sparse_vector` | SparseVector | SPARSE_INVERTED | 仅 hybrid 模式 |

## 使用

```go
cfg := vectordb.Config{
    URL:            "http://10.76.1.28",
    Key:            "your-api-key",
    Dimensions:     1024,
    CollectionName: "knowledge",
}

client, err := vectordb.New(cfg)
defer client.Close()

// 创建 collection
client.CreateCollection(ctx)

// 写入文档
client.Upsert(ctx, []vectordb.Doc{{
    Content:   "人工智能是...",
    Embedding: embedding,
    Name:      "ai_intro",
    MetaData:  map[string]any{"field": "universal"},
}}, nil)

// 搜索
results, _ := client.Search(ctx, queryEmbedding, 5, map[string]any{"field": "universal"})
```

## Eino 集成

```go
// 作为 eino Indexer
indexer := vectordb.NewEinoIndexer(client, embedder)
ids, _ := indexer.Store(ctx, docs)

// 作为 eino Retriever
retriever := vectordb.NewEinoRetriever(client, embedder,
    vectordb.WithTopK(10),
    vectordb.WithFilters(map[string]any{"field": "universal"}),
)
results, _ := retriever.Retrieve(ctx, "什么是人工智能")
```

## 配置

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `url` | (必填) | VectorDB 地址 |
| `key` | (必填) | API Key |
| `dimensions` | (必填) | 向量维度 |
| `username` | `root` | 用户名 |
| `db_name` | `agno` | 数据库名 |
| `collection_name` | `agno` | 集合名 |
| `search_type` | `vector` | `vector` 或 `hybrid` |
| `metric` | `IP` | `IP`/`COSINE`/`L2` |
| `timeout` | `30` | 超时秒数 |
| `partition_fields` | - | 参与 ID hash 的 metadata key |
| `hybrid_vector_weight` | `0.7` | 混合检索中向量权重 |
