# middleware — Gin 中间件

Gin HTTP 中间件套件，通过 FastApp 自动集成或独立使用。

## 中间件列表

| 中间件 | 函数 | 说明 |
|---|---|---|
| CORS | `CORS(config ...CORSConfig)` | 跨域配置（FastApp 默认开启） |
| Prometheus | `Prometheus()` | HTTP 请求指标采集 |
| RequestID | `RequestID()` | 生成/传递 `X-Request-ID`（即 task_id） |
| RequestLog | `RequestLog()` | 请求日志（含 task_id） |
| TokenAuth | `TokenAuth(verify, whitelist...)` | Token 校验 |
| RateLimit | `RateLimit(rdb, cfg)` | Redis 滑动窗口限流 |

## RequestID（trace 传递核心）

```go
// Gin 中间件自动生成或从请求头读取 X-Request-ID
router.Use(middleware.RequestID())

// 在任意位置通过 context 获取
taskID := middleware.GetTaskID(ctx)

// 手动注入（如异步任务场景）
ctx = middleware.WithTaskID(ctx, "custom-id")
```

## 独立使用示例

```go
router := gin.New()
router.Use(
    middleware.RequestID(),
    middleware.CORS(),
    middleware.Prometheus(),
    middleware.RequestLog(),
    middleware.TokenAuth(func(ctx context.Context, token string) (bool, error) {
        return token == "valid-token", nil
    }, "/healthz", "/metrics"),
    middleware.RateLimit(redisClient, middleware.RateLimitConfig{
        Limit:  100,
        Window: time.Minute,
    }),
)
```
