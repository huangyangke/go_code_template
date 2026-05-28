# async_queue — Redis Stream 异步任务队列

基于 Redis Stream 实现的异步任务队列，支持生产者/消费者模式、优先级、超时重试、PEL 恢复、SSE 事件推送。

## 架构

```
Producer (HTTP API) → Redis Stream → Consumer (Worker Pool)
    ↓                                    ↓
  SSE 事件推送                        Handler 执行 + 状态回写
```

## 生产者

```go
producer := async_queue.NewProducer(redisClient, async_queue.RedisConfig{
    StreamKey: "aikit:async_queue:my-service",
    Family:    "my-service",
}, endpoints, "my-service")

// 注册路由：POST /{endpoint}、GET /status/:task_id、POST /cancel/:task_id、GET /events
producer.RegisterRoutes(router.Group("/v1/async_queue"), "/v1/async_queue")
```

## 消费者

```go
consumer := async_queue.NewConsumer(redisClient, redisConfig, map[string]async_queue.EndpointConfig{
    "generate": {
        Handler:    func(ctx async_queue.Context) (any, error) {
            taskID := ctx.TaskID()
            params := ctx.Params()
            ctx.ReportProgress(50, "processing...")
            return result, nil
        },
        Timeout:        30 * time.Second,
        MaxConcurrency: 10,
        RetryOnTimeout: true,
    },
},
    "my-service",
    async_queue.WithGroupName("my_group"),
    async_queue.WithPel(async_queue.PelConfig{MinIdle: 5 * time.Minute, MaxRetries: 3}),
)
consumer.Start(ctx)
defer consumer.Stop()
```

## Context 接口

Handler 接收的 `Context` 同时实现了 `context.Context`，可直接传给 httpclient、log 等模块：

```go
Handler: func(ctx async_queue.Context) (any, error) {
    // task_id 已注入 context，下游自动透传
    resp, err := httpClient.Get(ctx, "/api/data")
    log.InfoCtx(ctx, "[generate] processing task %s", ctx.TaskID())
    return result, err
},
```

## 指标

| 指标名 | 类型 | Labels |
|---|---|---|
| `async_queue_enqueue_total` | counter | `family, endpoint, result` |
| `async_queue_consume_total` | counter | `family, endpoint, result` |
| `async_queue_handler_duration_seconds` | histogram | `family, endpoint, result` |
