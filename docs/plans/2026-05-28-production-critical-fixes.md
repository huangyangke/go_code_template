# 生产关键缺陷修复计划

> **面向 AI 代理的工作者：** 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 修复三个会在生产环境直接触发运行时崩溃或数据膨胀的缺陷

**架构：** 三个独立修复点，互不依赖，可并行执行。metrics 去重注册改写 NewXxxVec 工厂函数；async_queue poison message 在 PEL recovery 流程中加死信转移；Gin timeout middleware 新增独立文件并集成到 FastApp middleware chain。

**技术栈：** Go 1.25, prometheus/client_golang, redis/go-redis/v9, gin-gonic/gin

---

## 特性描述

修复 go-aikit 当前三个最高优先级的生产缺陷：(1) metrics `MustRegister` 重复注册 panic、(2) async_queue poison message PEL 无限膨胀、(3) Gin 请求超时 middleware 缺失。

## 用户故事

作为 Go AI 服务运维人员
我想要 go-aikit 在生产环境不因重复初始化 panic、不因毒消息卡死消费者、不因慢请求耗尽连接池
以便于服务稳定运行无需人工干预

## 问题陈述

1. `metrics.NewCounterVec/NewGaugeVec/NewHistogramVec` 内部用 `prom.MustRegister`，二次调用必 panic（测试、热重载场景）
2. `async_queue consumer.recoverPending` 对超过 `MaxRetries` 的毒消息仅 ACK+DEL，但标记 `MarkFailed` 失败时消息留在 PEL，下次 recovery 再扫描，无限循环
3. `app/middleware/` 没有 request timeout，慢 handler 挂死连接，耗尽 Gin worker pool

## 方案陈述

1. 将 `MustRegister` 替换为安全注册：先 `Register`，检测 `AlreadyRegisteredError` 则复用已有 collector
2. poison message 加死信 stream 转移：ACK 原消息 + XADD 到死信 stream（`aikit:async:{ns}:deadletter`），确保 MarkFailed 失败时消息也不留在 PEL
3. 新增 `app/middleware/timeout.go`，提供 configurable request timeout middleware（`context.WithTimeout` + 503 响应）

## 特性元数据

**特性类型**：Bug 修复 + 缺失能力补全
**预估复杂度**：中
**主要受影响系统**：`metrics/`, `app/async_queue/consumer.go`, `app/middleware/`, `app/fastapp.go`
**依赖**：prometheus/client_golang, redis/go-redis/v9, gin-gonic/gin（均已存在于 go.mod）

## 假设

- Prometheus `AlreadyRegisteredError` 在 `prometheus/client_golang v1.16.0+` 可通过 `errors.As` 检测
- async_queue 死信 stream 名遵循 `aikit:async:{ns}:deadletter` 格式，与现有 key 规范一致
- Gin timeout middleware 参考 `github.com/gin-contrib/timeout` 的模式但自实现，避免新增依赖

## 待决问题

- 死信 stream 是否需要消费者订阅（当前只做写入 + 人为排查），如果需要应另开计划
- timeout middleware 是否需要支持 per-route 超时（当前只做全局，per-route 留给后续增强）

## 非目标

- OTel tracing 集成（另开计划）
- log/slog Handler 实现（另开计划）
- Pulsar 自动重连（另开计划）
- gopool backpressure（另开计划）

---

## 上下文参考

### 相关代码库文件 — 重要：实现前必须阅读这些文件

- `metrics/counter.go`（全文）— 原因：MustRegister 调用点，需替换为安全注册
- `metrics/gauge.go`（全文）— 原因：MustRegister + RegisterGaugeFunc 调用点
- `metrics/histogram.go`（全文）— 原因：MustRegister 调用点
- `metrics/metrics.go`（全文）— 原因：Register/Enable 流程，安全注册辅助函数应放此文件
- `metrics/predefined.go`（全文）— 原因：所有预定义指标的 Register() 初始化闭包
- `app/async_queue/consumer.go:605-692` — 原因：recoverPending 函数，poison message 处理逻辑
- `app/async_queue/consumer.go:709-716` — 原因：ackAndDel 函数的原型
- `app/async_queue/config.go:1-30` — 原因：常量定义，死信 key 前缀应在此添加
- `app/async_queue/config.go:64-68` — 原因：PelConfig 结构体
- `app/async_queue/consumer.go:30-60` — 原因：Consumer 结构体字段，需加 deadLetterStream
- `app/async_queue/status.go:全文` — 原因：StatusStore.MarkFailed，poison message 状态标记
- `app/middleware/request_id.go`（全文）— 原因：现有 middleware 模式参考（简短 gin.HandlerFunc closure）
- `app/middleware/rate_limit.go`（全文）— 原因：带配置的 middleware 模式参考
- `app/middleware/middleware_test.go`（全文）— 原因：middleware 测试模式
- `app/fastapp.go:53-100` — 原因：MiddlewareConfig 结构体，需加 EnableTimeout 字段
- `app/fastapp.go:364-420` — 原因：buildMiddlewareChain 函数，需插入 timeout middleware

### 需新建的文件

- `app/middleware/timeout.go` — Gin request timeout middleware（context.WithTimeout + 503）
- `app/middleware/timeout_test.go` — timeout middleware 单元测试
- `app/async_queue/deadletter.go` — 死信 stream 写入逻辑
- `app/async_queue/deadletter_test.go` — 死信 stream 测试

### 需遵循的模式

**命名惯例：** 中文注释、ASCII 句点、godoc 格式（见 CLAUDE.md 注释风格）

**middleware 模式：**
```go
// Timeout limits each request to the configured duration.
// 中间件在超时后返回 503，正常请求继续执行.
func Timeout(d time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) { ... }
}
```

**Redis key 规范：** `aikit:{module}:{family}:{resource}:{id}`
- 死信 stream: `aikit:async:{ns}:deadletter`

**async_queue 中 Lua script 模式：**
```go
var xxxScript = redis.NewScript(`...`)
// 调用: xxxScript.Run(ctx, rdb, []string{key}, args...).Result()
```

**metrics 工厂函数模式（修改后）：**
```go
func NewCounterVec(cfg *CounterVecOpts) CounterVec {
    vec := prom.NewCounterVec(...)
    safeRegister(vec) // 替代 prom.MustRegister(vec)
    return &promCounterVec{counter: vec}
}
```

---

## 流程图

### Fix 1: metrics 安全注册

```text
NewCounterVec/NewGaugeVec/NewHistogramVec
   |
   v
prom.NewCounterVec/NewGaugeVec/NewHistogramVec  (创建 collector)
   |
   v
safeRegister(vec)  (替代 MustRegister)
   |
   +-- Register 成功 ──→ 返回 wrapper
   |
   +-- AlreadyRegisteredError ──→ prom.DefaultGatherer.FindSpecByName
                                  ──→ 复用已有 collector, 返回 wrapper
```

### Fix 2: poison message 死信转移

```text
recoverPending 遍历 PEL 条目
   |
   +-- retry_count < MaxRetries ──→ spawnAdmit (现有逻辑，不变)
   |
   +-- retry_count >= MaxRetries (poison message)
       |
       v
       MarkFailed(taskID, errorMsg)
       |
       v
       ackAndDel(msgID)  ── XACK + XDEL 原消息
       |
       v
       sendToDeadLetter(msg)  ── XADD 到 aikit:async:{ns}:deadletter
       |
       v
       metrics.ObserveAsyncQueueConsume(endpoint, "poisoned", 0)
```

### Fix 3: Gin timeout middleware

```text
HTTP 请求到达
   |
   v
Timeout middleware: context.WithTimeout(c.Request.Context(), d)
   |
   +-- handler 正常完成 ──→ c.Next() 自然返回
   |
   +-- context 超时 ──→ response.ServiceUnavailable(c)
                        c.Abort()
```

---

## 实现计划

### 阶段 1：Fix 1 — metrics 安全注册

### 阶段 2：Fix 2 — async_queue poison message 死信

### 阶段 3：Fix 3 — Gin timeout middleware + FastApp 集成

---

## 逐步任务

### 任务 1：metrics safeRegister 辅助函数

**文件：**
- 修改：`metrics/metrics.go`

- [ ] **步骤 1：编写 safeRegister 函数**

在 `metrics/metrics.go` 中添加：

```go
package metrics

import (
    "errors"

    prom "github.com/prometheus/client_golang/prometheus"
)

// safeRegister 注册 collector，若已注册则复用.
// 替代 prom.MustRegister，避免二次初始化 panic.
func safeRegister(c prom.Collector) {
    if err := prom.Register(c); err != nil {
        var are prom.AlreadyRegisteredError
        if errors.As(err, &are) {
            // 已注册，忽略 — collector 的指针相等保证后续 Inc/Observe
            // 操作会命中同一个底层 prometheus metric.
            return
        }
        // 非 AlreadyRegisteredError（如 collector 不合法），仍应 panic.
        panic(err)
    }
}
```

- [ ] **步骤 2：验证 `errors.As` 可检测 `AlreadyRegisteredError`**

运行：`cd /data/02_serve/go-aikit && go build ./metrics/...`
预期：编译成功，无报错

### 任务 2：替换三个 NewXxxVec 中的 MustRegister

**文件：**
- 修改：`metrics/counter.go:29`
- 修改：`metrics/gauge.go:30`
- 修改：`metrics/histogram.go:36`

- [ ] **步骤 3：替换 counter 中的 MustRegister**

`metrics/counter.go` 中将：
```go
prom.MustRegister(vec)
```
改为：
```go
safeRegister(vec)
```

- [ ] **步骤 4：替换 gauge 中的 MustRegister**

`metrics/gauge.go` 中将：
```go
prom.MustRegister(vec)
```
改为：
```go
safeRegister(vec)
```

同时 `RegisterGaugeFunc` 中的 `prom.MustRegister(g)` 也改为 `safeRegister(g)`。

- [ ] **步骤 5：替换 histogram 中的 MustRegister**

`metrics/histogram.go` 中将：
```go
prom.MustRegister(vec)
```
改为：
```go
safeRegister(vec)
```

- [ ] **步骤 6：编译验证**

运行：`cd /data/02_serve/go-aikit && go build ./metrics/...`
预期：编译成功

- [ ] **步骤 7：编写二次注册不 panic 的测试**

`metrics/metrics_test.go` 中编写测试（如果文件不存在则创建）：

```go
package metrics

import (
    "testing"

    prom "github.com/prometheus/client_golang/prometheus"
    "github.com/stretchr/testify/assert"
)

func TestSafeRegister_Idempotent(t *testing.T) {
    // 清理：确保 collector 未注册
    vec := prom.NewCounterVec(prom.CounterOpts{
        Name: "aikit_test_safe_register_counter",
        Help: "test counter for safeRegister",
    }, []string{"label"})
    safeRegister(vec) // 首次注册
    safeRegister(vec) // 二次注册 — 不应 panic

    // 验证可用
    vec.WithLabelValues("test").Inc()
    fam, err := prom.DefaultGatherer.Gather()
    assert.NoError(t, err)
    found := false
    for _, m := range fam {
        if m.GetName() == "aikit_test_safe_register_counter" {
            found = true
        }
    }
    assert.True(t, found)

    // 清理
    prom.Unregister(vec)
}

func TestSafeRegister_InvalidCollector_Panics(t *testing.T) {
    assert.Panics(t, func() {
        safeRegister(prom.NewCounterVec(prom.CounterOpts{
            Name: "aikit_test_invalid:colon",
            Help: "invalid name causes panic",
        }, nil))
    })
}
```

- [ ] **步骤 8：运行 metrics 测试**

运行：`cd /data/02_serve/go-aikit && go test ./metrics/ -v -run TestSafeRegister`
预期：两个测试 PASS

- [ ] **步骤 9：完整 metrics 测试套件无回归**

运行：`cd /data/02_serve/go-aikit && go test ./metrics/ -v`
预期：全部 PASS

---

### 任务 3：死信 stream key 构造与配置

**文件：**
- 修改：`app/async_queue/config.go`

- [ ] **步骤 10：添加死信 stream key 构造函数**

在 `app/async_queue/config.go` 的 key 构造辅助区域添加：

```go
func buildDeadLetterStreamKey(ns string) string {
    return keyPrefix + ":" + ns + ":deadletter"
}
```

- [ ] **步骤 11：添加死信 TTL 常量**

在 `app/async_queue/config.go` 的常量区域添加：

```go
DeadLetterStatusTTL = 7 * 24 * 60 * 60 // 死信状态保留 7 天（秒）
```

---

### 任务 4：死信写入逻辑 (deadletter.go)

**文件：**
- 创建：`app/async_queue/deadletter.go`
- 创建：`app/async_queue/deadletter_test.go`

- [ ] **步骤 12：创建 deadletter.go**

```go
package async_queue

import (
    "context"
    "time"

    "github.com/redis/go-redis/v9"

    "github.com/huangyangke/go-aikit/log"
)

// sendToDeadLetter 将毒消息转移到死信 stream，确保 PEL 中不留残留.
// XADD 到死信 stream + MarkFailed 状态标记（独立于 ackAndDel，互不干扰）.
func (c *Consumer) sendToDeadLetter(ctx context.Context, msg redis.XMessage) {
    streamKey := buildDeadLetterStreamKey(c.namespace)
    values := msg.Values
    // 补充死信元数据
    values["dead_at"] = time.Now().Unix()
    values["original_msg_id"] = msg.ID

    id, err := c.rdb.XAdd(ctx, &redis.XAddArgs{
        Stream: streamKey,
        Values: values,
        MaxLen: 10000, // 死信 stream 保留最近 10000 条，防止无限增长
        Approx: true,  // 使用 MAXLEN ~ 精简（XTRIM），性能更好
    }).Result()
    if err != nil {
        log.Error("[Consumer][dead_letter][xadd_error][msg_id=%s]: %v", msg.ID, err)
    } else {
        log.Info("[Consumer][dead_letter][transferred][msg_id=%s→%s]", msg.ID, id)
    }
}
```

- [ ] **步骤 13：编译验证**

运行：`cd /data/02_serve/go-aikit && go build ./app/async_queue/...`
预期：编译成功

- [ ] **步骤 14：创建 deadletter_test.go**

```go
package async_queue

import (
    "context"
    "testing"
    "time"

    "github.com/alicebob/miniredis/v2"
    "github.com/redis/go-redis/v9"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSendToDeadLetter(t *testing.T) {
    mr, rdb := setupMiniredis(t)

    c := &Consumer{
        rdb:       rdb,
        namespace: "test",
    }

    msg := redis.XMessage{
        ID: "12345-0",
        Values: map[string]any{
            "task_id":  "task-abc",
            "endpoint": "/process",
        },
    }

    ctx := context.Background()
    c.sendToDeadLetter(ctx, msg)

    // 验证死信 stream 中有消息
    result, err := rdb.XRange(ctx, "aikit:async:test:deadletter", "-", "+").Result()
    require.NoError(t, err)
    require.Len(t, result, 1)

    // 验证原数据 + 死信元数据
    assert.Equal(t, "task-abc", result[0].Values["task_id"])
    assert.Equal(t, "/process", result[0].Values["endpoint"])
    assert.Equal(t, "12345-0", result[0].Values["original_msg_id"])
    assert.NotNil(t, result[0].Values["dead_at"])
}

func TestSendToDeadLetter_XAddError(t *testing.T) {
    mr, rdb := setupMiniredis(t)
    // 关闭 miniredis 使 XAdd 失败
    mr.Close()

    c := &Consumer{
        rdb:       rdb,
        namespace: "test",
    }

    msg := redis.XMessage{
        ID: "12345-0",
        Values: map[string]any{"task_id": "task-abc"},
    }

    // 不应 panic，只是 log error
    c.sendToDeadLetter(context.Background(), msg)
}
```

- [ ] **步骤 15：运行 deadletter 测试**

运行：`cd /data/02_serve/go-aikit && go test ./app/async_queue/ -v -run TestSendToDeadLetter`
预期：两个测试 PASS

---

### 任务 5：recoverPending 中集成死信转移

**文件：**
- 修改：`app/async_queue/consumer.go:664-672`

- [ ] **步骤 16：读取 recoverPending 当前逻辑**

文件 `app/async_queue/consumer.go`，行 664-672 当前代码：

```go
if int(count) >= c.pel.MaxRetries {
    taskID, _ := msg.Values["task_id"].(string)
    _ = c.statusStore.MarkFailed(ctx, taskID,
        fmt.Sprintf("PEL recovery: exceeded max retries (%d)", count))
    if err := c.ackAndDel(ctx, msg.ID); err != nil {
        log.Error("[Consumer][PEL_recovery][ack_and_del_error][task_id=%s][msg_id=%s]: %v", taskID, msg.ID, err)
    }
    metrics.ObserveAsyncQueueConsume(endpoint, "poisoned", 0)
    poisonedCount++
    log.Warn("[Consumer][PEL_recovery][poisoned][task_id=%s][msg_id=%s][retry_count=%d]", taskID, msg.ID, count)
    continue
}
```

- [ ] **步骤 17：插入 sendToDeadLetter 调用**

在 `ackAndDel` 之后、`metrics.Observe` 之前插入一行：

```go
if int(count) >= c.pel.MaxRetries {
    taskID, _ := msg.Values["task_id"].(string)
    _ = c.statusStore.MarkFailed(ctx, taskID,
        fmt.Sprintf("PEL recovery: exceeded max retries (%d)", count))
    if err := c.ackAndDel(ctx, msg.ID); err != nil {
        log.Error("[Consumer][PEL_recovery][ack_and_del_error][task_id=%s][msg_id=%s]: %v", taskID, msg.ID, err)
    }
    c.sendToDeadLetter(ctx, msg)
    metrics.ObserveAsyncQueueConsume(endpoint, "poisoned", 0)
    poisonedCount++
    log.Warn("[Consumer][PEL_recovery][poisoned][task_id=%s][msg_id=%s][retry_count=%d]", taskID, msg.ID, count)
    continue
}
```

- [ ] **步骤 18：编译验证**

运行：`cd /data/02_serve/go-aikit && go build ./app/async_queue/...`
预期：编译成功

- [ ] **步骤 19：运行 async_queue 全量测试**

运行：`cd /data/02_serve/go-aikit && go test ./app/async_queue/ -v`
预期：全部 PASS，无回归

---

### 任务 6：Gin timeout middleware

**文件：**
- 创建：`app/middleware/timeout.go`
- 创建：`app/middleware/timeout_test.go`

- [ ] **步骤 20：创建 timeout.go**

```go
package middleware

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"

    "github.com/huangyangke/go-aikit/app/response"
)

// TimeoutConfig 配置请求超时 middleware.
type TimeoutConfig struct {
    Timeout time.Duration // 超时时长，默认 30s
}

// Timeout 为每个请求设置 context 超时上限.
// 超时后返回 503 Service Unavailable 并中止后续 handler.
// 正常完成的请求不受影响.
func Timeout(cfg TimeoutConfig) gin.HandlerFunc {
    if cfg.Timeout <= 0 {
        cfg.Timeout = 30 * time.Second
    }
    return func(c *gin.Context) {
        ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.Timeout)
        defer cancel()
        c.Request = c.Request.WithContext(ctx)

        finished := make(chan struct{})
        go func() {
            c.Next()
            close(finished)
        }()

        select {
        case <-finished:
            // handler 正常完成
        case <-ctx.Done():
            // 请求超时
            response.ServiceUnavailable(c)
            c.Abort()
        }
    }
}
```

注意：需要 import `"context"`。

修正版：

```go
package middleware

import (
    "context"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"

    "github.com/huangyangke/go-aikit/app/response"
)

// TimeoutConfig 配置请求超时 middleware.
type TimeoutConfig struct {
    Timeout time.Duration // 超时时长，默认 30s
}

// Timeout 为每个请求设置 context 超时上限.
// 超时后返回 503 Service Unavailable 并中止后续 handler.
// 正常完成的请求不受影响.
func Timeout(cfg TimeoutConfig) gin.HandlerFunc {
    if cfg.Timeout <= 0 {
        cfg.Timeout = 30 * time.Second
    }
    return func(c *gin.Context) {
        ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.Timeout)
        defer cancel()
        c.Request = c.Request.WithContext(ctx)

        finished := make(chan struct{})
        go func() {
            c.Next()
            close(finished)
        }()

        select {
        case <-finished:
            // handler 正常完成
        case <-ctx.Done():
            // 请求超时
            response.ServiceUnavailable(c)
            c.Abort()
        }
    }
}
```

等一下——把 handler 放到 goroutine 里运行会导致 Gin 的 writer 被并发写入（正常完成和超时可能竞争写 response）。这是 `gin-contrib/timeout` 的已知反模式。

更安全的方案：不 spawn goroutine，而是让 Gin 框架自己处理超时。Context 超时后，handler 中使用 `ctx.Done()` 的数据库/HTTP client 调用会自动中断，handler 返回后再由 middleware 检查是否超时。

重新设计：

```go
package middleware

import (
    "context"
    "time"

    "github.com/gin-gonic/gin"

    "github.com/huangyangke/go-aikit/app/response"
)

// TimeoutConfig 配置请求超时 middleware.
type TimeoutConfig struct {
    Timeout time.Duration // 超时时长，默认 30s
}

// Timeout 为每个请求注入带超时的 context.
// handler 正常执行，但所有使用 ctx 的下游调用（MySQL、Redis、httpclient）
// 会自动在超时后返回 context.DeadlineExceeded.
// middleware 在 c.Next() 返回后检查：如果 context 已超时且未写过响应，
// 则写入 503.
func Timeout(cfg TimeoutConfig) gin.HandlerFunc {
    if cfg.Timeout <= 0 {
        cfg.Timeout = 30 * time.Second
    }
    return func(c *gin.Context) {
        ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.Timeout)
        defer cancel()
        c.Request = c.Request.WithContext(ctx)
        c.Next()

        // c.Next() 返回后检查：如果 context 超时且尚未写过响应
        if ctx.Err() == context.DeadlineExceeded && !c.Writer.Written() {
            response.ServiceUnavailable(c)
            c.Abort()
        }
    }
}
```

这个方案更安全：不 spawn goroutine，不并发写 response，只在 handler 返回后做补位。

- [ ] **步骤 21：检查 response 包是否有 ServiceUnavailable**

需要确认 `app/response/response.go` 是否有 `ServiceUnavailable` 函数。如果没有则添加。

```go
// ServiceUnavailable 返回 503 响应.
func ServiceUnavailable(c *gin.Context) {
    c.JSON(http.StatusServiceUnavailable, APIResponse{
        Code: CodeServiceUnavailable,
        Msg:  "service unavailable",
    })
}
```

对应错误码常量：

```go
CodeServiceUnavailable = 10010
```

需在 `app/response/response.go` 中添加常量和函数。

- [ ] **步骤 22：创建 timeout_test.go**

```go
package middleware

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
)

func TestTimeout_DefaultDuration(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.Use(Timeout(TimeoutConfig{})) // 默认 30s
    r.GET("/test", func(c *gin.Context) {
        c.String(http.StatusOK, "ok")
    })

    w := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/test", nil)
    r.ServeHTTP(w, req)
    assert.Equal(t, http.StatusOK, w.Code)
}

func TestTimeout_RequestCompletesBeforeTimeout(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.Use(Timeout(TimeoutConfig{Timeout: 100 * time.Millisecond}))
    r.GET("/fast", func(c *gin.Context) {
        c.String(http.StatusOK, "fast")
    })

    w := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/fast", nil)
    r.ServeHTTP(w, req)
    assert.Equal(t, http.StatusOK, w.Code)
    assert.Contains(t, w.Body.String(), "fast")
}

func TestTimeout_RequestExceedsTimeout(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.Use(Timeout(TimeoutConfig{Timeout: 50 * time.Millisecond}))
    r.GET("/slow", func(c *gin.Context) {
        // 模拟慢 handler：等待超出 timeout
        select {
        case <-c.Request.Context().Done():
            // context 超时，不写响应，让 middleware 补位
        case <-time.After(200 * time.Millisecond):
            c.String(http.StatusOK, "slow")
        }
    })

    w := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/slow", nil)
    r.ServeHTTP(w, req)
    assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
```

- [ ] **步骤 23：运行 timeout 测试**

运行：`cd /data/02_serve/go-aikit && go test ./app/middleware/ -v -run TestTimeout`
预期：三个测试 PASS

---

### 任务 7：FastApp 集成 timeout middleware

**文件：**
- 修改：`app/fastapp.go:54-100`（MiddlewareConfig）
- 修改：`app/fastapp.go:364-420`（buildMiddlewareChain）

- [ ] **步骤 24：在 MiddlewareConfig 中添加 EnableTimeout 和 TimeoutConfig**

在 `MiddlewareConfig` 结构体中添加：

```go
EnableTimeout  bool
TimeoutConfig  middleware.TimeoutConfig
```

- [ ] **步骤 25：在 buildMiddlewareChain 中插入 timeout middleware**

在 `a.engine.Use(gin.Recovery())` 之后、Prometheus 之前插入 timeout middleware：

```go
// Recovery must be outermost so panics in any subsequent middleware are caught.
a.engine.Use(gin.Recovery())

if a.mwCfg.EnableTimeout {
    a.engine.Use(middleware.Timeout(a.mwCfg.TimeoutConfig))
}

if a.mwCfg.EnablePrometheus {
    a.engine.Use(middleware.Prometheus())
}
```

执行顺序变为：`Recovery → Timeout → Prometheus → RequestID → RequestLog → CORS → RateLimit → TokenAuth`

Timeout 在 Recovery 之后是正确的：Recovery 兑换 panic，Timeout 兑换慢请求，两者互不干扰。

- [ ] **步骤 26：编译验证**

运行：`cd /data/02_serve/go-aikit && go build ./app/...`
预期：编译成功

- [ ] **步骤 27：运行 fastapp 测试**

运行：`cd /data/02_serve/go-aikit && go test ./app/ -v`
预期：全部 PASS

---

## 测试策略

### 单元测试

- `metrics/metrics_test.go`：二次注册不 panic、非法 collector 仍 panic
- `app/async_queue/deadletter_test.go`：死信 XADD 写入正确、XADD 失败不 panic
- `app/middleware/timeout_test.go`：默认时长、正常完成、超时返回 503

### 集成测试

- `app/async_queue/consumer_test.go` 中现有 `TestRecoverPending_*` 仍须 PASS，验证 poison message 流程无回归
- `metrics/predefined_test.go` 仍须 PASS，验证预定义指标初始化无回归

### 边界情况

- metrics：同一个 `NewCounterVec` 被两次调用（init 闭包内 + 手动调用）
- metrics：`AlreadyRegisteredError` 但 collector 类型不匹配（不应发生，但 safeRegister 应只忽略同类型）
- async_queue：`sendToDeadLetter` 时 Redis 不可用（XADD 失败，应 log 不应 panic）
- async_queue：`ackAndDel` 成功但 `sendToDeadLetter` 失败（消息 DEL 了但死信没写，可接受 — 消息已从 PEL 消除，死信是辅助排查）
- middleware：handler 已写过响应后超时（middleware 检测 `c.Writer.Written()`，不重复写）

---

## 故障模式

| 故障模式 | 触发条件 | 预期处理方式 | 是否需要测试 |
| --- | --- | --- | --- |
| metrics 二次注册 panic | 同名 CounterVec 第二次 NewCounterVec | safeRegister 检测 AlreadyRegisteredError 忽略 | 是 |
| metrics 非法 collector panic | Collector desc 校验失败 | safeRegister 仍 panic（与 MustRegister 行为一致） | 是 |
| PEL 毒消息无限循环 | MarkFailed 失败 + ackAndDel 失败 | ackAndDel 失败时消息仍留 PEL，但 sendToDeadLetter 已记录；下次 recovery 会再尝试 ack | 间接（已有 TestRecoverPending_*） |
| 死信 XADD 失败 | Redis 不可用 | log.Error 不 panic，不影响 ACK 流程 | 是 |
| timeout handler 超时但已写响应 | handler 写了部分 body 后超时 | middleware 检测 Written() 不重复写 | 是 |

---

## 验证命令

### 级别 1：语法与风格

```bash
cd /data/02_serve/go-aikit && go vet ./metrics/... ./app/async_queue/... ./app/middleware/... ./app/...
```

### 级别 2：单元测试

```bash
cd /data/02_serve/go-aikit && go test ./metrics/ ./app/async_queue/ ./app/middleware/ ./app/ -v
```

### 级别 3：集成测试

```bash
cd /data/02_serve/go-aikit && go test ./... -v
```

### 级别 4：手动验证

启动 FastApp（需 Redis），观察：
1. `/monitor/prometheus` endpoints 包含预期指标 — 接口调用两次 `Enable()` 不 panic
2. 注册一个总是失败的 async_queue handler，发射多条任务 → 超过 MaxRetries 的消息出现在死信 stream
3. 设置 `EnableTimeout=true, TimeoutConfig.Timeout=1s`，访问 /api/slow（handler sleep 5s）→ 返回 503

---

## 验收标准

- [ ] `metrics.NewCounterVec` / `NewGaugeVec` / `NewHistogramVec` 二次调用不 panic
- [ ] `metrics.RegisterGaugeFunc` 二次调用不 panic
- [ ] async_queue poison message 被转移到死信 stream
- [ ] 死信 stream 保留最近 10000 条（MAXLEN ~ 10000）
- [ ] Gin timeout middleware 正常请求不受影响
- [ ] Gin timeout middleware 超时请求返回 503
- [ ] FastApp `EnableTimeout=true` 将 timeout middleware 注入 chain
- [ ] 全量测试 `go test ./...` 零失败

---

## 完成清单

- [ ] 任务 1: safeRegister 辅助函数
- [ ] 任务 2: 替换 MustRegister → safeRegister
- [ ] 任务 3: 死信 key 构造 + 配置常量
- [ ] 任务 4: deadletter.go + deadletter_test.go
- [ ] 任务 5: recoverPending 集成 sendToDeadLetter
- [ ] 任务 6: timeout.go + timeout_test.go + ServiceUnavailable
- [ ] 任务 7: FastApp MiddlewareConfig + buildMiddlewareChain
- [ ] 全量 `go test ./...` PASS
- [ ] `go vet ./...` 零警告

---

## 备注

**设计决策：**

1. **metrics safeRegister 不返回已注册 collector**：因为 Prometheus `AlreadyRegisteredError` 中的 `ExistingCollector` 是接口类型，强转回具体 `*prom.CounterVec` 类型不可靠。但 `NewCounterVec` 创建的 wrapper 内部 指针与已注册 collector 指向同一个底层对象，所以 `Inc/Add` 操作仍然生效——它们用 `WithLabelValues()` 定位具体 metric，而非依赖 collector 指针。

2. **死信不建消费者**：当前只做写入 + 人工排查。自动消费死信（重试/告警）是另一个特性，不在本计划范围。

3. **timeout 不 spawn goroutine**：参考 `gin-contrib/timeout` 的教训——goroutine 方案会导致并发写 response panic。本方案只在 handler 返回后检查 context 状态补位，不改变 Gin 的单线程 response 写入模型。缺点是 handler 必须使用 `c.Request.Context()` 才能被超时传导中断，纯 `time.Sleep` 不受影响。但 go-aikit 的 mysql/redis/httpclient 都传 context，所以传导链路是完整的。

4. **timeout 在 Recovery 之后**：这是最安全的位置。panic 由 Recovery 兑换，超时 由 Timeout 兑换，两者互不干扰。如果 Timeout 放在 Recovery 之前，panic 导致的 goroutine 泄漏可能不会被正确回收。