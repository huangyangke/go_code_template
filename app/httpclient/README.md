# httpclient — HTTP 客户端

可配置的 HTTP 客户端，内置中间件链：Recovery → 熔断 → 重试 → 指标 → RequestID 透传 → 访问日志。

## 用法

```go
client := httpclient.New(httpclient.Config{
    Name:    "downstream",
    Addr:    "http://api.example.com",
    Timeout: 5 * time.Second,
    Breaker: &httpclient.BreakerConfig{},  // nil 则不启用
    Retry:   &httpclient.RetryConfig{MaxRetries: 3},
})

resp, err := client.Get(ctx, "/api/data")
resp, err := client.Post(ctx, "/api/data", "application/json", body)
resp, err := client.Put(ctx, "/api/data", "application/json", body)
resp, err := client.Delete(ctx, "/api/data")
```

## Trace 透传

`RequestIDMiddleware` 自动从 `ctx` 读取 `task_id` 并注入 `X-Request-ID` 到出站请求，无需手动设置：

```go
// 入站请求经过 middleware.RequestID 后，task_id 已在 ctx 中
resp, err := client.Get(ctx, "/downstream") // 自动携带 X-Request-ID
```

## 中间件链

| 顺序 | 中间件 | 说明 |
|---|---|---|
| 1 | Recovery | panic 恢复 |
| 2 | Breaker | 熔断器（5xx + 网络错误触发，4xx 不触发） |
| 3 | Retry | 指数退避重试（5xx + 网络错误重试） |
| 4 | Metrics | Prometheus 指标 |
| 5 | RequestID | 从 ctx 透传 `X-Request-ID` 到出站请求 |
| 6 | AccessLog | 访问日志 |

## 配置

```yaml
name: downstream
addr: http://api.example.com
timeout: 5s
breaker:
  timeout: 2s
  max_concurrent_requests: 100
  error_percent_threshold: 50
  sleep_window: 5s
retry:
  max_retries: 3
  wait_between: 1s
  jitter_fraction: 0.1
disable_metrics: false
```

## 指标

| 指标名 | 类型 | Labels |
|---|---|---|
| `http_client_requests_total` | counter | `family, client, method, endpoint, status` |
| `http_client_request_duration_seconds` | histogram | `family, client, method, endpoint` |
