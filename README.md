# go-aikit

自包含的 Go 生产级基础设施工具包，无私有库依赖。为 Go AI 后端服务提供开箱即用的日志、指标、缓存、数据库、中间件、熔断重试、异步队列等能力。

**模块路径**：`github.com/huangyangke/go-aikit`

## 模块总览

| 模块 | 路径 | 说明 |
|---|---|---|
| [FastApp](app/README.md) | `app/` | 应用引导（Gin 服务、生命周期管理、资源注册、优雅关闭） |
| [Async Queue](app/async_queue/README.md) | `app/async_queue/` | Redis Stream 异步任务队列（生产者 + 消费者） |
| [Auth](app/auth/README.md) | `app/auth/` | JWT + OAuth2 + 密码哈希 认证体系 |
| [HTTP Client](app/httpclient/README.md) | `app/httpclient/` | 可配置 HTTP 客户端（熔断/重试/指标/日志/trace 透传） |
| [Middleware](app/middleware/README.md) | `app/middleware/` | Gin 中间件套件（CORS、Prometheus、限流、RequestID、日志、Token 认证） |
| [Response](app/response/README.md) | `app/response/` | 统一 API 响应结构 |
| [Health](app/health/README.md) | `app/health/` | HTTP 健康检查 |
| [XJob](app/xjob/README.md) | `app/xjob/` | XXL-Job 执行器集成 |
| [Cache](cache/README.md) | `cache/` | 多级缓存（本地 + Redis） |
| [Config](config/README.md) | `config/` | 配置加载（YAML/env/Nacos/热重载） |
| [MySQL](database/mysql/README.md) | `database/mysql/` | GORM v2 封装（熔断/指标插件、泛型 Repository） |
| [Redis](database/redis/README.md) | `database/redis/` | go-redis/v9 三模式客户端（分布式锁、指标、扩展命令） |
| [Pulsar](database/pulsar/README.md) | `database/pulsar/` | Apache Pulsar 客户端封装（Client/Producer/Consumer 分离） |
| [Log](log/README.md) | `log/` | 零分配日志（文件轮转、UDP sink、context-aware） |
| [Metrics](metrics/README.md) | `metrics/` | Prometheus 指标工厂 + 预定义指标 |
| [Resilience](resilience/README.md) | `resilience/` | 熔断器 + 重试（指数退避） |
| [GoPool](utils/gopool/README.md) | `utils/gopool/` | 有界 goroutine 池 |
| [Upload](utils/upload/README.md) | `utils/upload/` | COS 文件上传 |
| [XStr](utils/xstr/README.md) | `utils/xstr/` | 字符串工具 |
| [Version](version/README.md) | `version/` | 构建版本信息 |
| [Embedding](agent/eino_plus/embedding/README.md) | `agent/eino_plus/embedding/` | Shanhai/OpenAI 兼容 Embedding 客户端（实现 eino Embedder 接口） |
| [VectorDB](agent/eino_plus/vectordb/README.md) | `agent/eino_plus/vectordb/` | 腾讯云向量数据库客户端（实现 eino Indexer/Retriever 接口） |

## 快速开始

### 方式一：FastApp 一站式引导（推荐）

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/huangyangke/go-aikit/app"
    "github.com/huangyangke/go-aikit/app/middleware"
    "github.com/huangyangke/go-aikit/app/response"
    "github.com/huangyangke/go-aikit/config"
    dbredis "github.com/huangyangke/go-aikit/database/redis"
    dbmysql "github.com/huangyangke/go-aikit/database/mysql"
    "github.com/huangyangke/go-aikit/log"
)

func main() {
    const family = "my-service"

    // 1. 配置加载
    loader, _ := config.New("config.yaml")

    // 2. 日志初始化（可选 — FastApp.Run() 会自动设置 log.Family 和 metrics.Family）
    log.Init(&log.Config{
        Stdout: true,
        Level:  "info",
        WithFields: map[string]log.WithField{
            "task_id": func(ctx context.Context) map[string]interface{} {
                if id := middleware.GetTaskID(ctx); id != "" {
                    return map[string]interface{}{"task_id": id}
                }
                return nil
            },
        },
    })

    // 3. 创建 FastApp（Family 自动同步到 log、metrics、xjob、async_queue）
    fa := app.NewFastApp(app.FastAppConfig{
        Family: family,
        Port:   8080,
    })

    // 4. 配置中间件
    fa.SetMiddlewares(app.MiddlewareConfig{
        EnableRequestID:  true,
        EnableRequestLog: true,
        EnablePrometheus: true,
    })

    // 5. 注册数据库
    fa.RegisterRedis("main", &dbredis.Config{
        Addrs: []string{"localhost:6379"},
        Type:  "standalone",
    })
    fa.RegisterMySQL("main", &dbmysql.Config{
        DSN: "user:pass@tcp(localhost:3306)/db",
    })

    // 6. 注册路由
    fa.SetRouteRegistrar(func(r *gin.Engine) {
        r.GET("/api/hello", func(c *gin.Context) {
            response.JSON(c, gin.H{"message": "hello"}, middleware.GetTaskID(c.Request.Context()))
        })
    })

    // 7. 启动
    fa.Run()
}
```

### 方式二：独立使用各模块

```go
import (
    "github.com/huangyangke/go-aikit/config"
    "github.com/huangyangke/go-aikit/log"
    "github.com/huangyangke/go-aikit/metrics"
    dbRedis "github.com/huangyangke/go-aikit/database/redis"
    dbMySQL "github.com/huangyangke/go-aikit/database/mysql"
    "github.com/huangyangke/go-aikit/cache"
    "github.com/huangyangke/go-aikit/resilience"
)

const family = "my-service"

// 配置
loader, _ := config.New("config.yaml")

// 日志（Config.Family 设置 _appID 字段）
log.Init(&log.Config{Family: family, Stdout: true, Level: "info"})

// 指标（设置所有预定义指标的 family label，不设置则 label 为空）
metrics.SetFamily(family)

// Redis
rdb := dbRedis.New(&dbRedis.Config{Addrs: []string{"localhost:6379"}, Type: "standalone"})

// MySQL
db := dbMySQL.New(&dbMySQL.Config{DSN: "user:pass@tcp(localhost:3306)/db"})

// 缓存（Config.Family 必传，否则返回 error）
c, _ := cache.New(cache.Config{Family: family, Name: "app", LocalMaxSize: 100 * 1024 * 1024})

// 重试
err := resilience.Retry(func() error { return callService() }, resilience.WithTimes(3))
```

## Trace 透传

go-aikit 使用 `task_id` 作为轻量 trace ID，在 HTTP 入口生成后自动贯穿整个调用链：

```
HTTP 入口 (X-Request-ID) → middleware.RequestID → context
    → log.InfoCtx(ctx, ...)        // 日志自动附加 task_id
    → httpclient.Get(ctx, ...)     // 出站请求自动带 X-Request-ID
    → async_queue handler(ctx)     // 异步任务继承 task_id
```

详见各模块 README。

## Redis Key 规范

所有 key 统一使用 `aikit:{module}:{family}:{resource}:{id}` 格式：

| Key | 格式 | 模块 |
|---|---|---|
| 任务状态 | `aikit:async:{ns}:task:status:{taskID}` | async_queue |
| 取消标记 | `aikit:async:{ns}:task:cancel:{taskID}` | async_queue |
| 取消通知 | `aikit:async:{ns}:channel:cancel` | async_queue (Pub/Sub) |
| 事件推送 | `aikit:async:{ns}:channel:events` | async_queue (Pub/Sub) |
| 消费者心跳 | `aikit:async:{ns}:heartbeat:{group}:{consumer}` | async_queue |
| 端点限流 | `aikit:async:{ns}:limit:{endpoint}` | async_queue |
| Stream | `aikit:async:{ns}:stream` | async_queue |
| 缓存条目 | `aikit:cache:{family}:{name}:{key}` | cache |
| 缓存失效 | `aikit:cache:{family}:{name}:invalidate` | cache (Pub/Sub) |
| 限流计数 | `aikit:ratelimit:{clientKey}:{bucket}` | middleware |

## 构建

```bash
go build ./...
go test ./...
```
