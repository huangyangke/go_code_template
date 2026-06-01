# eino_plus — RAG 基础组件

基于 [eino](https://github.com/cloudwego/eino) 框架封装的大模型 RAG 三件套，
兼容 OpenAI 接口规范的国产/海外服务（Shanhai、DashScope、OpenAI 等）。

| 模块 | 职责 | eino 接口 |
|------|------|-----------|
| [`embedding`](./embedding) | 文本/多模态向量化 | `embedding.Embedder` |
| [`vectordb`](./vectordb) | 腾讯云向量库存储与检索 | `indexer.Indexer` / `retriever.Retriever` |
| [`chatmodel`](./chatmodel) | 对话生成（阻塞 + 流式 + 多模态） | `model.BaseChatModel` |

## 典型 RAG 流程

```go
// 1. 向量化
emb, _ := embedding.NewEmbedder(ctx, &embedding.Config{
    APIKey: embKey, BaseURL: embURL, Dimensions: 1024,
})

// 2. 向量检索
vdb, _ := vectordb.New(vectordb.Config{
    URL: vdbURL, Key: vdbKey, DBName: "agno",
    CollectionName: "knowledge_go", Dimensions: 1024,
})
results, _ := vdb.Search(ctx, queryVec, 10, map[string]any{"field": "history"})

// 3. 拼接上下文并生成回答
cm, _ := chatmodel.NewChatModel(ctx, &chatmodel.Config{
    Model: "shanhai-max", APIKey: llmKey, BaseURL: llmURL,
})
answer, _ := cm.GenerateText(ctx, []*schema.Message{
    chatmodel.SystemMessage("基于以下知识库内容回答：\n" + context),
    chatmodel.UserMessage(question),
})
```

每个模块的详细用法见各自目录下的 README。
