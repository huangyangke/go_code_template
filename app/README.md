# app — FastApp 应用引导

一站式应用编排：Gin HTTP 服务、中间件套件、资源注册、异步队列、XXL-Job、生命周期管理、优雅关闭。

## 用法

```go
fa := app.NewFastApp(app.FastAppConfig{
    Family: "my-service",
    Port:   8080,
})

// 中间件
fa.SetMiddlewares(app.MiddlewareConfig{
    EnableRequestID:  true,
    EnableRequestLog: true,
    EnablePrometheus: true,
    EnablePprof:      true,
})

// 注册资源
fa.RegisterRedis("main", &dbredis.Config{...})
fa.RegisterMySQL("main", &dbmysql.Config{...})
fa.RegisterCache("main", cache.Config{...})
fa.RegisterHTTPClient("downstream", httpclient.Config{...})

// 路由
fa.SetRouteRegistrar(func(r *gin.Engine) {
    r.GET("/api/hello", handler)
})

// 生命周期钩子
fa.OnStart(func(ctx context.Context) error { return warmUp() })
fa.OnStop(func(ctx context.Context) error { return cleanup() })

// 启动（阻塞，监听 SIGINT/SIGTERM 优雅关闭）
fa.Run()
```

## 内置端点

| 路径 | 说明 |
|---|---|
| `/healthz` | 健康检查 |
| `/metrics` | Prometheus 指标 |
| `/debug/pprof/*` | pprof（需开启 `EnablePprof`） |
| `/swagger/*` | Swagger UI（需开启 `EnableSwagger`） |

## 资源访问

```go
rdb := fa.GetRedis("main")
db  := fa.GetMySQL("main")
c   := fa.GetCache("main")
hc  := fa.GetHTTPClient("downstream")
```

## 配置项

- `FastAppConfig`：Family、Host（默认 `0.0.0.0`）、Port（默认 `8080`）、Mode（默认 `release`）
- `MiddlewareConfig`：控制 RequestID/RequestLog/Prometheus/CORS/TokenAuth/RateLimit/pprof/Swagger 开关
- `AsyncQueueConfig`：Redis Stream 异步队列配置
- `XxlJobConfig`：XXL-Job 执行器配置
