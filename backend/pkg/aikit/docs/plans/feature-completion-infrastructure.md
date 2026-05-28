# Feature: go-aikit Infrastructure Layer Completion

The following plan should be complete, but it's important that you validate documentation and codebase patterns and task sanity before you start implementing.

Pay special attention to naming of existing utils types and models. Import from the right files etc.

## Feature Description

补全 go-aikit 的基础设施层（metrics、log、config、database/redis、database/mysql、cache、resilience），使其能独立支撑生产级 Go AI 服务。所有模块从 go-lib2/go-ceres 源码 copy 并替换私有依赖为公开包，保持 go-aikit 自包含。

## User Story

As a Go AI service developer  
I want to use go-aikit as a standalone infrastructure library  
So that I don't need to import private go-lib2/go-ceres and can rely on a self-contained, production-ready toolkit

## Problem Statement

go-aikit 目前仅有 async_queue 核心模块、middleware/request_id 和 response 三个包。Python aikit 有完整的日志、指标、缓存、熔断、中间件等基础设施，Go 端缺失这些能力，使用 go-aikit 的服务需自行解决横切关注点。

## Solution Statement

自底向上实现 7 个新包：metrics → log → config → database/redis + database/mysql → cache → resilience → middleware，采用薄封装 + copy 源码策略，替换私有库依赖为公开包，保持 go-aikit 自包含可独立发布。

## Feature Metadata

**Feature Type**: New Capability  
**Estimated Complexity**: High  
**Primary Systems Affected**: metrics/, log/, config/, database/redis/, database/mysql/, cache/, resilience/, app/middleware/  
**Dependencies**: prometheus/client_golang, redis/go-redis/v9, spf13/viper, gorm.io/gorm, mgtv-tech/jetcache-go, sony/gobreaker

## Assumptions

- go-redis/v8 → v9 升级的 API 变更可接受（async_queue 已使用 v8，需统一升级）
- afex/hystrix-go 虽已废弃但仍可用；disc doc 指定使用它，后续按 roadmap 迁移至 sony/gobreaker
- GORM 使用 v2（gorm.io/gorm），不沿用 go-lib2 的 v1（jinzhu/gorm）
- log/ 直接 copy go-lib2/xlog 的自定义零分配实现（非 zap/logrus）

## Open Questions

- go-redis/v8 → v9 升级后，async_queue 模块中 `redis.Cmdable`/`redis.Client`/`redis.Nil` 等类型和方法的兼容性是否需要逐文件检查？（TODO：验证 async_queue 所有 Redis 调用点）
- disc doc 使用 afex/hystrix-go，但该库已废弃无维护。是否提前切换到 sony/gobreaker？当前按 disc doc 执行，roadmap 中备注后续迁移。

## Non-goals

- cloud_serve（TTS/Chat/Embedding/Dify）：按需后续实现
- auth / admin/task_monitor：Go 服务通常由网关负责
- deploy/、nlp/rag、cryptor：Python ML 生态专属
- SkyWalking tracer 集成（tracer.go 直接删除，不在范围内）
- Nacos config center 集成（超出 disc doc 范围）

---

## CONTEXT REFERENCES

### Relevant Codebase Files — IMPORTANT: YOU MUST READ THESE FILES BEFORE IMPLEMENTING!

- `app/async_queue/consumer.go` (全文) — Why: 核心 Redis 客户端使用模式（redis.Cmdable、*redis.Client、Lua scripts、Pipeline）
- `app/async_queue/concurrency_limiter.go` (全文) — Why: Redis Lua script 模式、redis.Nil 处理、fail-open 模式
- `app/async_queue/status.go` (全文) — Why: Redis Hash 操作模式（HSet/HGet/HGetAll）
- `app/async_queue/config.go` (全文) — Why: 配置结构体命名和默认值模式
- `app/async_queue/producer.go` (全文) — Why: Gin handler 和 response 使用模式
- `app/middleware/request_id.go` (全文) — Why: 现有 middleware 模式（gin.HandlerFunc closure）
- `app/response/response.go` (全文) — Why: 错误码常量和统一响应模式
- `go.mod` (全文) — Why: 当前依赖列表，需升级和新增

### Source Code to Copy/Adapt (go-lib2)

- `/data/13_claude/go-lib2/xlog/log.go` — Why: log/ 包主入口，Config/Init/函数签名
- `/data/13_claude/go-lib2/xlog/field.go` — Why: D 类型别名和 KV* 构造器
- `/data/13_claude/go-lib2/xlog/level.go` — Why: Level 类型定义
- `/data/13_claude/go-lib2/xlog/handler.go` — Why: Handler 接口，Handlers bundle 模式
- `/data/13_claude/go-lib2/xlog/stdout.go` — Why: StdoutHandler 实现
- `/data/13_claude/go-lib2/xlog/file.go` — Why: FileHandler 实现
- `/data/13_claude/go-lib2/xlog/dsn.go` — Why: parseDSN 实现（需替换 xtime.Duration）
- `/data/13_claude/go-lib2/xlog/pattern.go` — Why: Render 接口
- `/data/13_claude/go-lib2/xlog/util.go` — Why: toMap 辅助函数
- `/data/13_claude/go-lib2/xlog/agent_linux.go` — Why: AgentHandler (UDP sink) Linux 实现
- `/data/13_claude/go-lib2/xlog/agent_darwin.go` — Why: AgentHandler Darwin 实现（暂可跳过，仅 Linux）
- `/data/13_claude/go-lib2/xlog/internal/core/` (全部文件) — Why: 零分配 encoder/field/buffer/pool 实现（需替换 xtime/xlog import）
- `/data/13_claude/go-lib2/xlog/internal/filewriter/` (全部文件) — Why: 旋转文件写入器
- `/data/13_claude/go-lib2/metric/metric.go` — Why: VectorOpts 基础配置
- `/data/13_claude/go-lib2/metric/counter.go` — Why: CounterVec 接口和实现
- `/data/13_claude/go-lib2/metric/gauge.go` — Why: GaugeVec 接口和实现
- `/data/13_claude/go-lib2/metric/histogram.go` — Why: HistogramVec 接口和实现
- `/data/13_claude/go-lib2/metric/timer.go` — Why: Timer 接口（可选，disc doc 未要求）
- `/data/13_claude/go-lib2/cache/redis/redis.go` — Why: Redis 三模式客户端核心实现
- `/data/13_claude/go-lib2/cache/redis/cmd.go` — Why: Redis 命令封装
- `/data/13_claude/go-lib2/cache/redis/redislock.go` — Why: 分布式锁（可选，disc doc 未明确要求，暂不 copy）
- `/data/13_claude/go-lib2/cache/redis/metrics.go` — Why: Prometheus hook（需改用 go-aikit 自身 metrics/）
- `/data/13_claude/go-lib2/cache/redis/tracer.go` — Why: 直接删除（SkyWalking 不在范围内）
- `/data/13_claude/go-lib2/database/orm/orm.go` — Why: GORM 封装参考（但需用 GORM v2 重写）

### Source Code to Copy/Adapt (go-ceres)

- `/data/13_claude/go-ceres/pkg/breaker/breaker.go` — Why: 熔断器封装模式参考
- `/data/13_claude/go-ceres/pkg/cache/cache/cache.go` — Why: jetcache-go 集成模式参考
- `/data/13_claude/go-ceres/pkg/config/` (全部文件) — Why: 配置结构和 Fix() 模式参考
- `/data/13_claude/go-ceres/pkg/net/http/gin.go` — Why: Gin response helper 模式参考

### What Already Exists

- `app/async_queue/` — 7 个文件，完整的异步队列实现（使用 go-redis/v8，需升级到 v9）
- `app/middleware/request_id.go` — 现有 middleware 模式，新增 middleware 需对齐
- `app/response/response.go` — 错误码和统一响应，RateLimit/TokenAuth 需复用

### New Files to Create

```
metrics/
  metrics.go          — VectorOpts 基础配置
  counter.go          — CounterVec 接口+实现
  gauge.go            — GaugeVec 接口+实现
  histogram.go        — HistogramVec 接口+实现

log/
  logger.go           — Config, Init, Debug/Info/Warn/Error/Fatal 函数
  level.go            — Level 类型
  field.go            — D 类型别名, KV* 构造器
  handler.go          — Handler 接口, Handlers bundle
  stdout.go           — StdoutHandler
  file.go             — FileHandler
  dsn.go              — parseDSN（替换 xtime.Duration）
  pattern.go          — Render 接口
  util.go             — toMap 辅助
  agent_linux.go      — AgentHandler (UDP sink) — 仅 Linux
  internal/
    core/
      buffer.go
      bufferpool.go
      encoder.go
      field.go        — Field struct, FieldType（替换 xtime）
      json_encoder.go
      pool.go
    filewriter/
      filewriter.go
      option.go

config/
  config.go           — Loader struct, New, Scan, Watch

database/
  redis/
    redis.go          — Config, Redis struct, New, 三模式访问器, 命令封装
    metrics.go        — Prometheus hook（改用 go-aikit metrics/）
  mysql/
    mysql.go          — Config, New（GORM v2）

cache/
  cache.go            — Config, New, 返回 cache.Cache

resilience/
  breaker.go          — Config, Breaker interface, New

app/
  middleware/
    request_id.go     — 已有
    prometheus.go     — NEW
    request_log.go    — NEW
    rate_limit.go     — NEW
    token_auth.go     — NEW
```

### Relevant Documentation — YOU SHOULD READ THESE BEFORE IMPLEMENTING!

- [go-redis v9 Migration Guide](https://redis.uptrace.dev/) — v8→v9 import path 变更（`github.com/go-redis/redis/v8` → `github.com/redis/go-redis/v9`）
- [jetcache-go README](https://github.com/mgtv-tech/jetcache-go) — WithName/WithRemote/WithLocal 配置模式，GoRedisV9Adapter 用法
- [GORM v2 Release Notes](https://gorm.io/docs/v2_release_note.html) — v1→v2 API 变更（gorm.Open 参数、Session、Plugin）
- [viper Documentation](https://github.com/spf13/viper) — YAML/TOML/env 加载，Watch 配置热重载
- [prometheus/client_golang](https://github.com/prometheus/client_golang) — prometheus.MustRegister, promauto, CounterVec/HistogramVec/GaugeVec API
- [sony/gobreaker](https://github.com/sony/gobreaker) — CircuitBreaker.Execute API（备用，后续迁移目标）
- [afex/hystrix-go](https://github.com/afex/hystrix-go) — hystrix.Do/Go, Configure, ErrCircuitOpen（当前指定使用的熔断库）

### Patterns to Follow

**Naming Conventions:**
- 包名：小写，下划线分隔（`async_queue`、`request_id`）
- 导出类型：PascalCase（`EndpointConfig`、`RedisConfig`、`CounterVecOpts`）
- 未导出类型：camelCase（`taskContext`、`priorityHeap`）
- 常量：PascalCase（`TaskStatusQueued`、`CodeSuccess`）
- 时间默认值：PascalCase `var` 非 `const`（`DefaultPullBlock`）

**Error Handling:**
- `New()` / `Init()` 配置非法时 **panic**（启动时快速失败）
- 运行时错误返回 `error`，不 panic
- 非关键错误用 `_ = err` 忽略（PEL recovery、status store）
- Redis 失败 fail-open（并发限流器模式）

**Import Ordering:**
- 标准库 → 空行 → 第三方库 → 空行 → 内部项目包
- 参考 `app/async_queue/producer.go` 的现有 import 分组

**Struct Construction:**
- 简单 `New*()` 构造器返回指针
- 功能选项模式：`Option func(*Config)` / `ConsumerOption func(*Consumer)`
- 默认配置：`default*Config()` 函数返回零值+默认值结构体

**Redis Patterns (from async_queue):**
- `redis.Cmdable` 接口用于通用操作（测试友好）
- `*redis.Client` 仅用于 Subscribe/PubSub
- Lua scripts via `redis.NewScript` 原子操作
- Pipeline 批量操作（ACK+DEL）
- Key namespace: `prefix:namespace:identifier`

**Gin Middleware Pattern (from request_id.go):**
- `gin.HandlerFunc` closure 模式
- `c.Next()` 调用链继续
- `c.Set()` / `c.Get()` 传递上下文

---

## FLOW DIAGRAM

```text
[Application main.go]
   |
   v
config.New("config.yaml") → *Loader
   |
   ├─ log.Init(&cfg.Log)
   ├─ redis.New(&cfg.Redis) → *Redis ──→ redis.Cmdable (for middleware/async_queue)
   ├─ mysql.New(&cfg.MySQL) → *gorm.DB
   ├─ cache.New(&cfg.Cache, rdb) → cache.Cache
   └─ resilience.New(&cfg.Breaker) → Breaker

[gin.Engine Setup]
   |
   ├─ middleware.RequestID()
   ├─ middleware.RequestLog()        ← depends on log/
   ├─ middleware.Prometheus()        ← depends on metrics/
   ├─ middleware.TokenAuth(verify)   ← standalone
   └─ middleware.RateLimit(rdb, cfg) ← depends on redis.Cmdable
```

## SYSTEM BOUNDARIES

| Boundary | Input Type | Required Validation |
| --- | --- | --- |
| config YAML/TOML file | file path string | 文件存在、格式合法、必填字段非空 |
| config.Env override | env vars | viper 自动处理，无需额外验证 |
| log UDP sink DSN | string `udp://host:port?chan=N&timeout=Xms` | parseDSN 校验格式、host/port 合法 |
| Redis connection config | struct: Addrs, Type, MasterName | Type ∈ {cluster,sentinel,standalone}; sentinel 时 MasterName 非空 |
| MySQL DSN | string | DSN 格式合法、连接成功（否则 panic） |
| cache.Config | struct: Name, LocalMaxSize, RedisClient | Name 非空; LocalMaxSize ≥ 0 |
| resilience.Config | struct: Name, Timeout, thresholds | Name 非空; Timeout > 0; RequestVolumeThreshold > 0 |
| middleware.TokenAuth whitelist | []string path prefix | 路径格式合法 |
| middleware.RateLimit config | struct: Limit, Window, KeyFunc | Limit > 0; Window > 0 |

---

## IMPLEMENTATION PLAN

### Phase 1: Foundation — Dependency Upgrade + metrics/

先升级 go-redis v8→v9（所有后续模块都依赖 v9），再实现 metrics/ 包（最底层，无内部依赖）。

**Tasks:**
- 升级 go-redis v8 → v9，更新 async_queue 所有 Redis API
- 实现 metrics/ 包（copy from go-lib2/metric，替换 import）

### Phase 2: Core Infrastructure — log/ + config/

**Tasks:**
- 实现 log/ 包（copy from go-lib2/xlog，替换所有私有依赖）
- 实现 config/ 包（viper 封装）

### Phase 3: Data Layer — database/redis/ + database/mysql/

**Tasks:**
- 实现 database/redis/ 包（copy from go-lib2/cache/redis，删除 tracer.go，改用自身 metrics/）
- 实现 database/mysql/ 包（GORM v2 封装，不复用 go-lib2 的 v1 实现）

### Phase 4: Cache + Resilience

**Tasks:**
- 实现 cache/ 包（jetcache-go 薄封装）
- 实现 resilience/ 包（afex/hystrix-go 薄封装）

### Phase 5: Middleware + Integration

**Tasks:**
- 实现 4 个新 middleware（prometheus, request_log, rate_limit, token_auth）
- 验证初始化数据流：config → log → redis → mysql → cache → resilience → gin middleware chain

---

## STEP-BY-STEP TASKS

### Task 1: UPDATE go.mod — 升级 go-redis v8 → v9

- **IMPLEMENT**: 将 `github.com/go-redis/redis/v8` 替换为 `github.com/redis/go-redis/v9`，更新所有间接依赖
- **IMPLEMENT**: 新增以下直接依赖：
  ```
  github.com/prometheus/client_golang v1.23.2
  github.com/redis/go-redis/v9 v9.19.0
  github.com/mgtv-tech/jetcache-go v1.2.6
  github.com/spf13/viper v1.21.0
  github.com/go-viper/mapstructure/v2 v2.x
  gorm.io/gorm v1.31.1
  gorm.io/driver/mysql v1.6.0
  github.com/afex/hystrix-go v0.0.0-20180502004556-fa1f5e0a9c6e
  github.com/coocood/freecache v1.2.7
  github.com/stretchr/testify v1.11.1
  github.com/alicebob/miniredis/v2 (test only)
  github.com/DATA-DOG/go-sqlmock (test only)
  ```
- **PATTERN**: 参考 go.mod 当前格式，direct 依赖无 `// indirect` 标注
- **GOTCHA**: v9 import path 是 `github.com/redis/go-redis/v9`（不是 `github.com/go-redis/redis/v9`）
- **GOTCHA**: viper v1.21+ 需要 `github.com/go-viper/mapstructure/v2` 而非旧版 `github.com/mitchellh/mapstructure`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go mod tidy && go build ./...`

### Task 2: UPDATE app/async_queue/ — 适配 go-redis v9 API

- **IMPLEMENT**: 遍历 async_queue 7 个文件，替换所有 `github.com/go-redis/redis/v8` import 为 `github.com/redis/go-redis/v9`
- **IMPLEMENT**: 检查 v8→v9 API 差异，关键变更点：
  - `redis.Nil` → 仍在 v9 中为 `redis.Nil`，不变
  - `redis.Cmdable` → 仍在 v9 中，不变
  - `redis.Client` → 仍在 v9 中，不变
  - `redis.NewScript` → 仍在 v9 中，不变
  - `redis.Pipeliner` → 仍在 v9 中，不变
  - `redis.Z` → 仍在 v9 中，不变
  - `redis.Message` → 仍在 v9 中，不变
  - **主要变化**: v9 中部分 Cmd.Val() 返回类型从 `interface{}` 变为具体类型；但 async_queue 主要用 Cmdable 接口方法，影响较小
- **IMPLEMENT**: 特别检查 `consumer.go` 中 `XReadGroup`/`XPendingExt`/`XClaim` 等 Stream 命令的调用签名是否变化
- **IMPLEMENT**: 更新 `config.go` 中 `RedisConfig` 的 json tag 和使用方式
- **PATTERN**: 现有 async_queue import 分组模式
- **GOTCHA**: v9 的 `redis.Options`/`redis.ClusterOptions`/`redis.FailoverOptions` struct 字段可能有微小变化（如新增 TLS config），需确认
- **GOTCHA**: miniredis 用于测试，也需升级到与 v9 兼容的版本
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./app/async_queue/...`

### Task 3: CREATE metrics/ 包

- **IMPLEMENT**: copy go-lib2/metric/ 的 metric.go + counter.go + gauge.go + histogram.go 到 `metrics/`
- **IMPLEMENT**: 替换所有 `git.imgo.tv/ft/go-lib2/metric` 内部自引用 import 为 `git.imgo.tv/dm/ai/go-aikit/metrics`
- **IMPLEMENT**: 替换 `git.imgo.tv/ft/go-lib2/xlog` import（在 log_handler.go 和 pusher.go 中）→ 暂时跳过这两个文件（disc doc 未要求 Timer/PromHandler/Pusher），仅保留核心 Counter/Gauge/Histogram
- **IMPLEMENT**: 不 copy timer.go、pusher.go、log_handler.go（超出 disc doc 范围）
- **IMPLEMENT**: 确保 `CounterVec`/`HistogramVec`/`GaugeVec` 是接口类型（返回接口而非指针到接口）
- **IMPLEMENT**: 方法签名对齐 disc doc：`Inc(labels ...string)`、`Add(v float64, labels ...string)`、`Observe(v int64, labels ...string)`、`Set(v float64, labels ...string)`
- **PATTERN**: 参考 go-lib2/metric/counter.go 的 `counterVec` struct + `prometheus.CounterVec` 嵌入模式
- **IMPORTS**: `github.com/prometheus/client_golang/prometheus`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./metrics/...`

### Task 4: CREATE log/ 包 — 核心 + internal/core

- **IMPLEMENT**: copy go-lib2/xlog/ 的核心文件到 `log/`，保持内部 `internal/core/` 子包结构
- **IMPLEMENT**: 替换所有私有依赖：
  | 原依赖 | 替换方式 |
  |---|---|
  | `git.imgo.tv/ft/go-lib2/env` | 内联 `os.Hostname()` 调用（仅 `env.Hostname()` 一行） |
  | `git.imgo.tv/ft/go-lib2/util` | 内联 `runtime.Caller(skip)` + `runtime.FuncForPC` 获取函数名（替代 `util.FuncName(skip)`） |
  | `git.imgo.tv/ft/go-lib2/xtime` | `xtime.Duration` → `time.Duration`（DSN timeout 解析改用标准库）；`xtime.Time` → `time.Time`（field.go assertAddTo 测试用） |
  | `git.imgo.tv/ft/go-lib2/xlog/internal/core` | 改为 `git.imgo.tv/dm/ai/go-aikit/log/internal/core` |
  | `git.imgo.tv/ft/go-lib2/xlog/internal/filewriter` | 改为 `git.imgo.tv/dm/ai/go-aikit/log/internal/filewriter` |
- **IMPLEMENT**: 对齐 disc doc 公开 API：
  ```go
  type Config struct {
      Family  string  // 服务标识（UDP tag）
      Dir     string  // 日志目录
      Agent   string  // UDP DSN: udp://host:port?chan=1024&timeout=100ms
      Stdout  bool
      Level   string  // debug | info | error | fatal
  }
  func Init(c *Config)
  func Debug(format string, v ...any)
  func Info(format string, v ...any)
  func Warn(format string, v ...any)
  func Error(format string, v ...any)
  func Fatal(format string, v ...any)
  ```
- **IMPLEMENT**: 跳过 `prom.go`（注释中已禁用，disc doc 不要求）
- **IMPLEMENT**: UDP sink 实现：copy `agent_linux.go`，适配 v9（无影响，UDP 是独立网络层）
- **IMPLEMENT**: `agent_darwin.go` 和 `agent_windows.go` 可 copy 但 Darwin/Windows 暂非主要目标，保留文件以兼容
- **PATTERN**: 现有 xlog 的零分配实现（buffer pool、JSON encoder、field 类型系统）
- **GOTCHA**: `parseDSN` 中 `xtime.Duration` 的 `UnmarshalText` 方法需替换为手动解析 `"100ms"` → `time.Duration`（xtime.Duration 本质是 time.Duration + 文本解析，直接用 `time.ParseDuration`）
- **GOTCHA**: `util.FuncName(skip)` 实现：`runtime.Caller(skip)` → `runtime.FuncForPC(pc).Name()`，但需裁剪前缀（去掉主函数名前的包路径），参考原 util.FuncName 实现
- **GOTCHA**: 不引入 `github.com/pkg/errors`（xlog 使用的 errors.WithStack/WithMessage）。Init/config 阶段 panic 快速失败，运行时错误用标准 `fmt.Errorf`。文件 handler Close 用 `errors.Join` 或逐个收集。
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./log/...`

### Task 5: CREATE config/ 包

- **IMPLEMENT**: 创建 `config/config.go`，使用 `spf13/viper` 封装
- **IMPLEMENT**: 对齐 disc doc API：
  ```go
  type Loader struct {
      v *viper.Viper
  }
  func New(path string) (*Loader, error)
  func (l *Loader) Scan(key string, v any) error
  func (l *Loader) Watch(key string, fn func()) error
  ```
- **IMPLEMENT**: `New(path)` — viper.SetConfigFile(path)，ReadInConfig，返回 Loader
- **IMPLEMENT**: `Scan(key, v)` — viper.UnmarshalKey(key, v)，使用 mapstructure v2 反序列化
- **IMPLEMENT**: `Watch(key, fn)` — viper.WatchConfig() + fsnotify，OnConfigChange 回调触发 fn。setup 失败返回 error
- **IMPLEMENT**: 每个 Loader 持有独立 `*viper.Viper` 实例（避免全局污染）
- **IMPLEMENT**: 支持 yaml/toml/json，环境变量覆盖（viper 默认行为）
- **PATTERN**: 参考 go-ceres/pkg/config/ 的 Fix()+Validate() 模式概念，但简化为单文件 Loader
- **GOTCHA**: viper v1.21 用 `go-viper/mapstructure/v2`。Scan 内部 viper.UnmarshalKey 已自动使用 mapstructure，无需手动 import mapstructure
- **GOTCHA**: viper.OnConfigChange 只检测文件修改事件，不保证 key 级别变更。Watch 回调在任意文件变更时触发 fn，调用方需自行判断是否 key 变了
- **IMPORTS**: `github.com/spf13/viper`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./config/...`

### Task 6: CREATE database/redis/ 包

- **IMPLEMENT**: copy go-lib2/cache/redis/ 的 redis.go + cmd.go 到 `database/redis/`
- **IMPLEMENT**: **删除 tracer.go**（SkyWalking 不在范围内）
- **IMPLEMENT**: **改写 metrics.go** — 将 `git.imgo.tv/ft/go-lib2/metric` import 替换为 `git.imgo.tv/dm/ai/go-aikit/metrics`，改用 go-aikit 自身的 CounterVec/GaugeVec/Timer
- **IMPLEMENT**: **修正 standalone 模式** — Config.Type json tag options 显式包含 `"standalone"`（源码中遗漏）
- **IMPLEMENT**: 替换 `xtime.Duration` → `time.Duration`（所有超时配置字段）
- **IMPLEMENT**: 替换 `xstr.RandN` → 内联随机字符串生成（用 `crypto/rand` 或 `math/rand`）
- **IMPLEMENT**: **暂不 copy redislock.go**（分布式锁不在 disc doc 范围内，后续可按需添加）
- **IMPLEMENT**: go-redis import 改为 `github.com/redis/go-redis/v9`，API 适配
- **IMPLEMENT**: 对齐 disc doc 公开 API：
  ```go
  type Config struct {
      KeyPrefix    string
      Addrs        []string
      Type         string        // cluster | sentinel | standalone
      MasterName   string        // sentinel 模式必填
      PoolSize     int
      MaxRetries   int
      // 超时配置...
  }
  func New(c *Config, opts ...Option) *Redis
  ```
- **IMPLEMENT**: `*Redis` 暴露命令方法 + `Cluster()`/`Sentinel()`/`Standalone()` 返回 `redis.Cmdable`
- **PATTERN**: go-lib2/cache/redis/ 的 key 前缀模式、panic-on-connect-fail 模式
- **GOTCHA**: go-redis v9 的 `redis.Options`/`redis.ClusterOptions`/`redis.FailoverOptions` 中 `PoolSize` 默认已变化，需确认。v9 默认 `runtime.GOMAXPROCS * 10`，v8 默认 10*CPU+2
- **GOTCHA**: v9 `redis.Hook` 接口方法签名可能从 `BeforeProcess(ctx, cmd)` 变为包含更多参数——需对照 v9 Hook 接口确认 metrics.go 的 Prometheus hook 实现
- **GOTCHA**: validate `redis.Nil` 在 v9 中仍为标准 "not found" 错误
- **IMPORTS**: `github.com/redis/go-redis/v9`, `git.imgo.tv/dm/ai/go-aikit/metrics`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./database/redis/...`

### Task 7: CREATE database/mysql/ 包

- **IMPLEMENT**: 创建 `database/mysql/mysql.go`，使用 GORM v2（不复用 go-lib2 的 v1 实现）
- **IMPLEMENT**: 对齐 disc doc API：
  ```go
  type Config struct {
      DSN          string
      MaxOpenConns int
      MaxIdleConns int
      MaxLifetime  time.Duration
  }
  func New(c *Config) *gorm.DB
  ```
- **IMPLEMENT**: `New(c)` — `gorm.Open(mysql.Open(c.DSN), &gorm.Config{...})`，配置连接池参数，panic on error
- **IMPLEMENT**: 连接池配置通过 `sql.DB` 取回后设置：
  ```go
  db, err := gorm.Open(mysql.Open(c.DSN), &gorm.Config{})
  sqlDB, _ := db.DB()
  sqlDB.SetMaxOpenConns(c.MaxOpenConns)
  sqlDB.SetMaxIdleConns(c.MaxIdleConns)
  sqlDB.SetConnMaxLifetime(c.MaxLifetime)
  ```
- **PATTERN**: disc doc 简化版 API（不含 interceptor/breaker/tracer，仅基础连接）
- **GOTCHA**: GORM v2 import 路径：`gorm.io/gorm` + `gorm.io/driver/mysql`（非 `github.com/jinzhu/gorm`）
- **GOTCHA**: `gorm.Open` v2 返回 `(*gorm.DB, error)`，v1 返回 `(db *gorm.DB, err error)` — 签名相同，但 Open 参数完全不同
- **IMPORTS**: `gorm.io/gorm`, `gorm.io/driver/mysql`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./database/mysql/...`

### Task 8: CREATE cache/ 包

- **IMPLEMENT**: 创建 `cache/cache.go`
- **IMPLEMENT**: 对齐 disc doc API：
  ```go
  type Config struct {
      Name           string
      LocalMaxSize   int64         // 本地缓存最大字节（0 = 不启用本地缓存）
      LocalTTL       time.Duration
      RedisClient    *database_redis.Redis  // nil = 仅本地模式
      RemoteExpiry   time.Duration
      NotFoundExpiry time.Duration
      StatsDisabled  bool
  }
  func New(c *Config) cache.Cache
  ```
- **IMPLEMENT**: `New(c)` 内部逻辑：
  - `LocalMaxSize > 0` → `local.NewFreeCache(local.Size(c.LocalMaxSize), c.LocalTTL)`
  - `c.RedisClient != nil` → `remote.NewGoRedisV9Adapter(c.RedisClient.Cluster())`（或根据 Redis 模式选择对应 Cmdable）
  - 两者条件组合决定模式：local-only / remote-only / both
  - 调用 `cache.New(...)` 返回 `cache.Cache`
- **IMPLEMENT**: Config 不暴露 jetcache-go 内部类型（local.Local/remote.Remote），调用方无需直接 import jetcache-go 子包
- **PATTERN**: 参考 go-ceres/pkg/cache/cache/cache.go 的模式，但简化 Config
- **GOTCHA**: jetcache-go v1.2.6 使用 `remote.NewGoRedisV9Adapter`（不是 V8），传入 `redis.Cmdable`
- **GOTCHA**: `cache.Cache` 接口来自 jetcache-go 顶层包，是不可避免的直接依赖
- **GOTCHA**: `local.Size` 类型是 `local.MB`/`local.KB`/`local.GB` 常量（int64），`LocalMaxSize` 传入时需转为 `local.Size` 类型
- **GOTCHA**: `New()` 内部需从 `*Redis` 取正确模式的 Cmdable：cluster 用 `.Cluster()`，standalone 用 `.Standalone()`，sentinel 用 `.Sentinel()`。简化版：全部用 `.Cluster()` 行不通（standalone/sentinel 返回的是 `*redis.Client` 而非 `*redis.ClusterClient`）。解决方案：**在 Config 上增加 RedisMode 字段**，或者 **用 `Redis.Cmdable()` 方法**（目前 go-lib2 Redis struct 没有统一的 Cmdable 访问器，只有分模式的）。需要在 Redis struct 上添加一个 `Cmdable()` 方法返回底层 `redis.Cmdable`（NewRedis 时已存储在 `r.client` 字段）。
- **IMPORTS**: `github.com/mgtv-tech/jetcache-go`, `github.com/mgtv-tech/jetcache-go/local`, `github.com/mgtv-tech/jetcache-go/remote`, `git.imgo.tv/dm/ai/go-aikit/database/redis`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./cache/...`

### Task 9: CREATE resilience/ 包

- **IMPLEMENT**: 创建 `resilience/breaker.go`
- **IMPLEMENT**: 对齐 disc doc API：
  ```go
  type Config struct {
      Name                   string
      Timeout                time.Duration
      MaxConcurrentRequests  int
      RequestVolumeThreshold int
      SleepWindow            time.Duration
      ErrorPercentThreshold  int
  }
  type Breaker interface {
      Do(run func() error, fallback func(error) error) error
  }
  func New(c *Config) Breaker
  ```
- **IMPLEMENT**: 基于 `github.com/afex/hystrix-go` 封装：
  - `New(c)` → `hystrix.ConfigureCommand(c.Name, hystrix.CommandConfig{...})` + 返回 `&hystrixBreaker{name: c.Name}`
  - `Do(run, fallback)` → `hystrix.Do(c.Name, run, fallback)`
- **IMPLEMENT**: fallback 为 nil 时不做降级，直接返回原始错误（hystrix.Do 传 nil fallback 即此行为）
- **IMPLEMENT**: 熔断时 Do 返回 `hystrix.ErrCircuitOpen`
- **IMPLEMENT**: **不**在接口上暴露 `ctx`，避免语义误导（hystrix 超时由 Config.Timeout 控制）
- **PATTERN**: 参考 go-ceres/pkg/breaker/breaker.go 的封装模式
- **GOTCHA**: afex/hystrix-go 最后 commit 2018，无维护但 feature-complete。按 disc doc 指定使用，后续 roadmap 迁移至 sony/gobreaker
- **GOTCHA**: hystrix.ConfigureCommand 是全局配置（按 command name 注册）。如果同一 name 多次调用 New，后者覆盖前者配置。这不影响单实例场景，但需注意。
- **IMPORTS**: `github.com/afex/hystrix-go/hystrix`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./resilience/...`

### Task 10: CREATE app/middleware/prometheus.go

- **IMPLEMENT**: `Prometheus() gin.HandlerFunc` — 拦截请求，记录 method/path/status/耗时 到 metrics
- **IMPLEMENT**: 使用 metrics.NewCounterVec + metrics.NewHistogramVec 记录：
  - Counter: `http_requests_total{method, path, status}`
  - Histogram: `http_request_duration_seconds{method, path}`
- **IMPLEMENT**: 在 handler 中 `c.Next()`，之后取 `c.Writer.Status()` 和耗时
- **PATTERN**: 参考 go-ceres/pkg/net/http/server.go 的 ServerPrometheusMiddleware
- **IMPORTS**: `github.com/gin-gonic/gin`, `git.imgo.tv/dm/ai/go-aikit/metrics`, `time`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./app/middleware/...`

### Task 11: CREATE app/middleware/request_log.go

- **IMPLEMENT**: `RequestLog() gin.HandlerFunc` — 记录 method/path/status/耗时
- **IMPLEMENT**: 使用 `log.Info` 输出请求日志，格式：`method path status duration`
- **IMPLEMENT**: 从 context 取 RequestID（`middleware.GetTaskID`）附加值
- **IMPLEMENT**: 在 handler 中 `c.Next()`，之后取 status 和耗时
- **PATTERN**: 参考 go-ceres ServerAccessLogMiddleware，但简化为 log.Info 格式化输出
- **IMPORTS**: `github.com/gin-gonic/gin`, `git.imgo.tv/dm/ai/go-aikit/log`, `git.imgo.tv/dm/ai/go-aikit/app/middleware`, `time`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./app/middleware/...`

### Task 12: CREATE app/middleware/rate_limit.go

- **IMPLEMENT**: 滑动窗口限流，接受 `redis.Cmdable`（非 *Redis）
- **IMPLEMENT**: 对齐 disc doc API：
  ```go
  type RateLimitConfig struct {
      Limit   int
      Window  time.Duration
      KeyFunc func(*gin.Context) string  // 默认按 RemoteIP
  }
  func RateLimit(rdb redis.Cmdable, cfg RateLimitConfig) gin.HandlerFunc
  ```
- **IMPLEMENT**: 滑动窗口算法：
  1. 当前时间窗口 key: `rate_limit:{keyFunc(c)}:{now/window}`
  2. INCR 当前窗口计数
  3. EXPIRE 当前窗口 key
  4. 如果 count > Limit → 返回 429 (`response.RateLimited`)
  5. 否则 c.Next()
- **IMPLEMENT**: 默认 KeyFunc = `func(c *gin.Context) string { return c.ClientIP() }`
- **PATTERN**: 参考 Python aikit rate_limit 概念，用 Redis INCR+EXPIRE 实现滑动窗口
- **GOTCHA**: `redis.Cmdable` 来自 `github.com/redis/go-redis/v9`（已升级），调用方通过 `rdb.Cluster()`/`rdb.Standalone()` 传入
- **GOTCHA**: 限流 429 直接写 HTTP 响应，不传递给下游
- **IMPORTS**: `github.com/gin-gonic/gin`, `github.com/redis/go-redis/v9`, `git.imgo.tv/dm/ai/go-aikit/app/response`, `time`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./app/middleware/...`

### Task 13: CREATE app/middleware/token_auth.go

- **IMPLEMENT**: Bearer token 认证中间件
- **IMPLEMENT**: 对齐 disc doc API：
  ```go
  type VerifyFunc func(ctx context.Context, token string) (bool, error)
  func TokenAuth(verify VerifyFunc, whitelist ...string) gin.HandlerFunc
  ```
- **IMPLEMENT**: 检查 Authorization header，提取 Bearer token，调用 verify
- **IMPLEMENT**: whitelist 路径前缀匹配 → 跳过认证
- **IMPLEMENT**: verify 返回 false → 返回 401 (`response.Unauthorized`)
- **IMPLEMENT**: verify 返回 error → 返回 401
- **PATTERN**: 纯标准库 + gin，无第三方认证库依赖
- **GOTCHA**: whitelist 匹配用 `strings.HasPrefix(c.Request.URL.Path, wlItem)`
- **IMPORTS**: `github.com/gin-gonic/gin`, `context`, `strings`, `git.imgo.tv/dm/ai/go-aikit/app/response`
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./app/middleware/...`

### Task 14: ADD Redis Cmdable() 方法 + 全量构建验证

- **IMPLEMENT**: 在 database/redis/Redis struct 上添加 `Cmdable()` 方法，返回底层 `redis.Cmdable`（用于 cache/ 和 middleware/rate_limit 直接传入）
- **IMPLEMENT**: 确保所有包能全量构建：`go build ./...`
- **IMPLEMENT**: 确认初始化数据流可串联：config → log → redis → mysql → cache → resilience → middleware
- **VALIDATE**: `cd /data/02_serve/go-aikit && go build ./... && go vet ./...`

---

## TESTING STRATEGY

### Unit Tests

每个新模块需有单元测试，遵循 go-lib2 的 `testify/assert` 模式：

| 模块 | 测试文件 | 测试方式 |
|---|---|---|
| metrics/ | `counter_test.go`, `gauge_test.go`, `histogram_test.go` | 注册并验证 label 和值（用 `testutil.ToFloat64`） |
| log/ | `log_test.go`, `level_test.go`, `dsn_test.go` | Init 不 panic，DSN 解析正确，UDP 输出格式 |
| config/ | `config_test.go` | testdata/ yaml，Scan 反序列化，多 Loader 互不干扰 |
| database/redis/ | `redis_test.go` | miniredis mock，验证三模式连接和基本命令 |
| database/mysql/ | `mysql_test.go` | go-sqlmock，验证 GORM 初始化和连接池配置 |
| cache/ | `cache_test.go` | 仅 local 模式验证 hit/miss（FreeCache 不需 Redis） |
| resilience/ | `breaker_test.go` | 构造失败函数，验证熔断触发和恢复 |
| app/middleware/ | `*_test.go` | httptest.NewRecorder + gin.SetMode(gin.TestMode) |

### Integration Tests

可选，需外部服务：
- Redis: miniredis 内存模拟
- MySQL: go-sqlmock

### Edge Cases

- metrics: label 数量不匹配 → prometheus panic（设计如此，启动时快速失败）
- log: UDP agent DSN 格式错误 → panic（Init 快速失败）
- log: UDP channel full → 日志丢弃（非阻塞 select/default）
- config: 配置文件不存在 → New 返回 error
- config: Watch fsnotify 初始化失败 → 返回 error
- redis: 连接失败 → panic（启动快速失败）
- redis: nil key → Key() 返回前缀化空 key
- mysql: DSN 格式错误 → panic
- cache: LocalMaxSize=0 → 仅远程模式；RedisClient=nil → 仅本地模式
- resilience: fallback=nil → 直接返回原始错误
- rate_limit: Redis 不可用 → **fail-open**（允许请求通过，不因限流 Redis 故障阻断服务）
- token_auth: 无 Authorization header → 401

---

## FAILURE MODES

| Failure Mode | Trigger | Expected Handling | Test Required |
| --- | --- | --- | --- |
| Redis 连接失败 | 配置错误/服务不可用 | panic 启动时快速失败 | yes (redis_test) |
| MySQL 连接失败 | DSN 错误/服务不可用 | panic 启动时快速失败 | yes (mysql_test) |
| UDP agent 不可达 | 网络故障/agent 下线 | 非阻塞丢弃日志，不影响服务 | yes (log_test) |
| 配置文件不存在 | path 错误 | New 返回 error | yes (config_test) |
| 熔断触发 | 错误率超阈值 | Do 返回 hystrix.ErrCircuitOpen | yes (breaker_test) |
| 限流 Redis 失败 | Redis 临时不可用 | fail-open，允许请求通过 | yes (rate_limit_test) |
| 认证 verify 失败 | verify 函数返回 error | 401 Unauthorized | yes (token_auth_test) |
| 缓存远程写入失败 | Redis 不可用 | jetcache 自动降级到本地缓存 | no (jetcache 内部处理) |

---

## VALIDATION COMMANDS

### Level 1: Syntax & Style

```bash
cd /data/02_serve/go-aikit
go build ./...
go vet ./...
```

### Level 2: Unit Tests

```bash
cd /data/02_serve/go-aikit
go test ./metrics/... -v
go test ./log/... -v
go test ./config/... -v
go test ./database/redis/... -v
go test ./database/mysql/... -v
go test ./cache/... -v
go test ./resilience/... -v
go test ./app/middleware/... -v
go test ./app/async_queue/... -v
```

### Level 3: Full Suite

```bash
cd /data/02_serve/go-aikit
go test ./... -v -count=1
```

### Level 4: Manual Validation

创建临时 example/main.go 验证初始化数据流：

```go
package main

import (
    "git.imgo.tv/dm/ai/go-aikit/config"
    "git.imgo.tv/dm/ai/go-aikit/log"
    "git.imgo.tv/dm/ai/go-aikit/database/redis"
    "git.imgo.tv/dm/ai/go-aikit/database/mysql"
    "git.imgo.tv/dm/ai/go-aikit/cache"
    "git.imgo.tv/dm/ai/go-aikit/resilience"
    "git.imgo.tv/dm/ai/go-aikit/app/middleware"
    "github.com/gin-gonic/gin"
)

func main() {
    ldr, err := config.New("config.yaml")
    if err != nil {
        panic(err)
    }

    var cfg struct {
        Log     log.Config
        Redis   database_redis.Config
        MySQL   database_mysql.Config
        Cache   cache.Config
        Breaker resilience.Config
    }
    if err := ldr.Scan("", &cfg); err != nil {
        panic(err)
    }

    log.Init(&cfg.Log)
    rdb := database_redis.New(&cfg.Redis)
    db := database_mysql.New(&cfg.MySQL)
    c := cache.New(&cfg.Cache) // with RedisClient set
    brk := resilience.New(&cfg.Breaker)

    log.Info("all infrastructure initialized")

    // Use breaker
    err = brk.Do(func() error {
        return nil // success
    }, nil)

    r := gin.Default()
    r.Use(middleware.RequestID())
    r.Use(middleware.RequestLog())
    r.Use(middleware.Prometheus())
    r.Use(middleware.RateLimit(rdb.Cmdable(), middleware.RateLimitConfig{Limit: 100, Window: time.Minute}))
    r.Run(":8080")
}
```

验证可编译运行（需 mock Redis 或使用 miniredis）

---

## ACCEPTANCE CRITERIA

- [ ] 所有 7 个新包 + 4 个 middleware 创建完毕，API 对齐 disc doc
- [ ] go-redis v8 → v9 升级完成，async_queue 全部适配
- [ ] `go build ./...` 无错误
- [ ] `go vet ./...` 无错误
- [ ] `go test ./...` 全绿
- [ ] 所有私有库依赖（go-lib2/go-ceres）已替换为公开包或内联
- [ ] 初始化数据流可串联：config → log → redis → mysql → cache → resilience → middleware
- [ ] 中间件错误处理符合 disc doc：429 限流、401 认证直接写响应
- [ ] 无 SkyWalking/Nacos 等超出范围的依赖引入

---

## COMPLETION CHECKLIST

- [ ] 所有 14 个 task 按顺序完成
- [ ] 每个 task 的 VALIDATE 命令通过
- [ ] 全量 `go build ./...` && `go vet ./...` && `go test ./...` 通过
- [ ] 无 go-lib2/go-ceres 私有 import 残留
- [ ] 手动验证初始化数据流 example 可编译

---

## NOTES

1. **go-redis v9 升级是前提**：jetcache-go v1.2.6 强依赖 `redis/go-redis/v9`，必须先升级 async_queue 的 Redis API。这是当前项目最大的兼容性风险点。

2. **hystrix-go 维护状态**：afex/hystrix-go 已废弃，仍可正常编译使用但无后续修复。disc doc 明确指定使用它，项目 roadmap 备注后续迁移至 sony/gobreaker。建议在 resilience 包接口设计上保持简单（仅 Config + Breaker interface），便于后续替换底层实现。

3. **GORM v2 vs v1**：go-lib2 使用 jinzhu/gorm (v1)，disc doc 需 GORM 封装。go-aikit 使用 gorm.io/gorm (v2)，API 完全不同，不能直接 copy go-lib2 的 orm.go，需重新编写简化的封装。

4. **xlog copy 复杂度**：xlog 有 35 个文件、内部子包、多种操作系统实现。核心替换点集中（env→os.Hostname、xtime→time.Duration、util→runtime.Caller），但文件数量多需逐一处理。建议先 copy 全部文件再批量替换。

5. **Redis Cmdable() 方法**：disc doc 中 RateLimit 接受 `redis.Cmdable`，调用方通过 `.Cluster()`/`.Standalone()` 取。但 cache 包需要根据 Redis 模式传入正确的 Cmdable 到 jetcache-go。建议在 Redis struct 上添加统一的 `Cmdable()` 方法返回底层 client（NewRedis 时存的 `r.client` 字段）。

6. **实现顺序严格自底向上**：metrics(log 不依赖 metrics) → log → config → redis + mysql → cache(依赖 redis) → resilience(无依赖) → middleware(依赖 metrics/log/redis)。每层完成后立即验证构建，不跳步。