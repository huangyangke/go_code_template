# chatmodel — Shanhai/OpenAI 兼容 Chat Completions 客户端

实现 [eino](https://github.com/cloudwego/eino) 框架的 `model.BaseChatModel` 接口。

兼容所有 OpenAI Chat Completions API 格式的服务（Shanhai、DashScope、OpenAI 等），
支持阻塞生成、SSE 流式输出与多模态（图文）输入，是 RAG 流程中"生成"环节的基础组件，
与同目录的 `embedding`（向量化）+ `vectordb`（检索）组成完整 RAG 三件套。

## 使用

```go
cm, err := chatmodel.NewChatModel(ctx, &chatmodel.Config{
    Model:   "shanhai-max",
    APIKey:  "your-api-key",
    BaseURL: "https://linghub.shuziwenbo.cn/v1", // 不含 /chat/completions
})

// 阻塞生成
text, err := cm.GenerateText(ctx, []*schema.Message{
    chatmodel.SystemMessage("你是知识库问答助手"),
    chatmodel.UserMessage("什么是向量数据库？"),
})

// 流式生成（SSE 场景，逐片回调）
err = cm.StreamText(ctx, messages, func(chunk string) error {
    fmt.Print(chunk) // 写入 SSE 响应
    return nil
})

// 多模态（图文混合）
msg := chatmodel.UserMultiModalMessage("描述这张图", "https://example.com/a.jpg")
text, err = cm.GenerateText(ctx, []*schema.Message{msg})
```

## 实现 eino 接口

`ChatModel` 实现 `model.BaseChatModel`，可直接接入 eino 编排链：

```go
var _ model.BaseChatModel = (*chatmodel.ChatModel)(nil)

msg, err := cm.Generate(ctx, input)               // 阻塞
reader, err := cm.Stream(ctx, input)              // 流式，调用方负责 reader.Close()
```

## 配置

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `model` | `shanhai-max` | 模型名称 |
| `api_key` | (必填) | API Key |
| `base_url` | (必填) | API 地址（不含 /chat/completions） |
| `temperature` | (不传) | 采样温度，指针类型区分"未设置"与"显式 0" |
| `max_tokens` | `0`（不传） | 最大生成 token 数 |
| `top_p` | (不传) | 核采样 |
| `enable_thinking` | (不传) | 国产模型思考开关 |
| `timeout` | `5m` | HTTP 超时（生成耗时较长） |

## 支持的服务

| 服务 | base_url | model |
|------|----------|-------|
| Shanhai | `https://linghub.shuziwenbo.cn/v1` | `shanhai-max` |
| DashScope | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `qwen-max` |
| OpenAI | `https://api.openai.com/v1` | `gpt-4o` |
