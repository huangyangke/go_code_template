# embedding — Shanhai Embedding 客户端

实现 [eino](https://github.com/cloudwego/eino) 框架的 `embedding.Embedder` 接口。

专为山海（Shanhai）私有 Embedding API 设计，**不兼容**标准 OpenAI Embedding 格式。

## 使用

```go
emb, err := embedding.NewEmbedder(ctx, &embedding.Config{
    Model:      "shanhai-embedding",
    APIKey:     "your-api-key",
    BaseURL:    "https://linghub.shuziwenbo.cn/v1",
    Dimensions: 1024,
})

// 实现 eino embedding.Embedder 接口
vecs, err := emb.EmbedStrings(ctx, []string{"你好", "世界"})
// vecs[0] = [0.1, 0.2, ...] (1024 维)

// 多模态 (图文混合，对应 Python shanhai_embedding 的 dict 输入)
vecs, err := emb.EmbedMultiModal(ctx, []embedding.MultiModalInput{
    {Text: "一只猫"},
    {Image: "https://example.com/cat.jpg"},
    {Text: "图文混合", Image: "https://example.com/cat.jpg"},
})
```

## 与 VectorDB 集成

```go
// 直接传给 EinoIndexer / EinoRetriever
indexer := vectordb.NewEinoIndexer(vdbClient, emb)
retriever := vectordb.NewEinoRetriever(vdbClient, emb, vectordb.WithTopK(10))
```

## 配置

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `model` | `shanhai-embedding` | 模型名称 |
| `api_key` | (必填) | API Key |
| `base_url` | (必填) | API 地址 (不含 /embeddings) |
| `dimensions` | `1024` | 向量维度 |
| `timeout` | `60s` | HTTP 超时 |
| `batch_size` | `25` | 批量请求大小 |

## 支持的服务

| 服务 | base_url | model |
|------|----------|-------|
| Shanhai | `https://linghub.shuziwenbo.cn/v1` | `shanhai-embedding` |
