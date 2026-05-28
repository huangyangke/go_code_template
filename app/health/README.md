# health — HTTP 健康检查

为 FastApp 和自定义服务提供 `/healthz` 健康检查端点。

## 用法

FastApp 自动注册 `/healthz`。独立使用时：

```go
checker := health.HealthChecker // 任何实现了 Ping(ctx) error 的对象

// 结构化健康状态
status := health.HealthStatus{
    Status: "healthy",
    Services: map[string]*health.ServiceHealth{
        "mysql": {Status: "healthy"},
        "redis": {Status: "healthy"},
    },
}
```

## 接口

```go
type HealthChecker interface {
    Ping(ctx context.Context) error
}
```
