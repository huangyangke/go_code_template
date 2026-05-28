# go-aikit 功能补全 — Disc Doc

**Date:** 2026-05-18  
**Status:** Approved

---

## 背景

go-aikit 是 Python 版 aikit 的 Go 实现，目前只有 `async_queue` 核心模块、`middleware/request_id` 和 `response` 三个包。Python aikit 有完整的基础设施层（日志、指标、缓存、熔断、中间件等），Go 端缺失这些能力，导致使用 go-aikit 的服务需要自行解决这些横切关注点。

## 目标

补全 go-aikit 的基础设施层，使其能独立支撑生产级 Go AI 服务，对标 Python aikit 的可移植模块（跳过 Python 特有的 deploy/onnx、deploy/triton、nlp/rag、cryptor、auth/admin 等）。

## 不在范围内

- `cloud_serve`（TTS/Chat/Embedding/Dify）：按需后续实现
- `auth` / `admin/task_monitor`：Go 服务通常由网关或外部系统负责
- `deploy/`、`nlp/rag`、`cryptor`：Python ML 生态专属，Go 端不适合

## 方案选择

**薄封装 + copy 源码**：不 import go-lib2/go-ceres 私有库，将所需代码直接 copy 进 go-aikit，底层依赖替换为公开包。

**理由：**
- go-aikit 自包含，无私有库依赖链，可独立发布和使用
- 复用内部已验证的实现（Redis 三模式、jetcache-go 多级缓存、xlog UDP sink）
- 与内部技术栈对齐（go-redis/v8、prometheus/client_golang）

## 目录结构

```
go-aikit/
├── app/
│   ├── async_queue/        # 已有
│   ├── middleware/
│   │   ├── request_id.go   # 已有
│   │   ├── prometheus.go   # NEW
│   │   ├── request_log.go  # NEW
│   │   ├── rate_limit.go   # NEW
│   │   └── token_auth.go   # NEW
│   └── response/           # 已有
├── metrics/
│   └── metrics.go          # NEW — counter/histogram/gauge 工厂，全局注册
├── log/
│   ├── logger.go           # NEW — copy from go-lib2/xlog（自定义零分配实现，非 logrus）
│   └── udp_sink.go         # NEW — UDP JSON sink（copy from go-lib2/xlog）
├── config/
│   └── config.go           # NEW — yaml/toml/env 加载（viper 封装）
├── database/
│   ├── redis/
│   │   └── redis.go        # NEW — cluster/sentinel/standalone（copy from go-lib2/cache/redis/）
│   └── mysql/
│       └── mysql.go        # NEW — GORM 封装（copy from go-lib2/database/orm/）
├── cache/
│   └── cache.go            # NEW — jetcache-go 薄封装 + RedisAdaptor
└── resilience/
    └── breaker.go          # NEW — hystrix-go 薄封装（copy from go-lib2，替换 import 为 afex/hystrix-go）
```

## 模块接口

### metrics/
```go
// 返回接口类型，非指针到接口
func NewCounterVec(opts *CounterVecOpts) CounterVec
func NewHistogramVec(opts *HistogramVecOpts) HistogramVec
func NewGaugeVec(opts *GaugeVecOpts) GaugeVec
```
Counter/Histogram/GaugeVec 是接口类型，提供 `Inc`/`Add`/`Observe`/`Set` 方法，label 通过 varargs 传入。直接 copy go-lib2/metric/ 包，依赖 `prometheus/client_golang`。

### log/
```go
type Config struct {
    Family  string  // 服务标识（UDP tag）
    Dir     string  // 日志目录（空则不写文件）
    Agent   string  // UDP DSN，完整格式：udp://host:port?chan=1024&timeout=100ms
    Stdout  bool
    Level   string  // debug | info | error | fatal（xlog 原生 tag 不含 warn，warn 运行时可用但配置 Level 建议用四个标准值）
}
func Init(c *Config)
func Debug(format string, v ...any)
func Info(format string, v ...any)
func Warn(format string, v ...any)
func Error(format string, v ...any)
func Fatal(format string, v ...any)
```
**实现说明：** copy go-lib2/xlog/ 的自定义零分配实现（非 logrus），需剔除/内联以下内部依赖：
- `git.imgo.tv/ft/go-lib2/env`：仅 `os.Hostname()` 封装，直接内联替换
- `git.imgo.tv/ft/go-lib2/xtime`：`Duration` 类型用于 AgentConfig.Timeout，替换为标准 `time.Duration`
- `git.imgo.tv/ft/go-lib2/trace`（xlog/prom.go 注释中已禁用）：直接跳过
- xlog 不依赖 logrus，无需引入该包

### config/
```go
type Loader struct {
    v *viper.Viper  // 每个实例持有独立 viper，支持多实例并发安全
}
func New(path string) (*Loader, error)
func (l *Loader) Scan(key string, v any) error   // 反序列化到结构体
func (l *Loader) Watch(key string, fn func()) error  // 文件变更热重载，setup 失败返回 error
```
支持 yaml/toml/json，读取环境变量覆盖（viper 默认行为）。每个 `Loader` 持有独立 `*viper.Viper` 实例，避免全局状态污染。

### database/redis/
**源码位置：** copy from go-lib2/`cache/redis/`（注意：源路径是 cache/redis，非 database/redis）。  
standalone 模式在源码中实现但 Config.Type json tag 有误（options 未包含 standalone），copy 时显式修正。

**copy 时需剔除的文件：**
- `tracer.go`：依赖 `github.com/SkyAPM/go2sky` + `skywalking.apache.org/repo/goapi`，APM 追踪不在范围内，直接删除
- `metrics.go`：依赖 `git.imgo.tv/ft/go-lib2/metric`，替换为依赖 go-aikit 自身的 `metrics/` 包（或直接用 prometheus/client_golang 重写）

```go
type Config struct {
    KeyPrefix    string
    Addrs        []string
    Type         string        // cluster | sentinel | standalone
    MasterName   string        // sentinel 模式必填
    PoolSize     int
    MaxRetries   int
    // ... 超时配置（DialTimeout/ReadTimeout/WriteTimeout/IdleTimeout/PingTimeout）
}
func New(c *Config, opts ...Option) *Redis
```
`*Redis` 暴露 `Get/Set/Del/Pipeline/ScriptRun` 等方法，以及 `Cluster()`/`Sentinel()`/`Standalone()` 取原生 go-redis 客户端（返回 `redis.Cmdable`）。

### database/mysql/
```go
type Config struct {
    DSN          string
    MaxOpenConns int
    MaxIdleConns int
    MaxLifetime  time.Duration
}
func New(c *Config) *gorm.DB
```

### cache/
```go
type Config struct {
    Name           string
    LocalMaxSize   int64         // 本地缓存最大字节数（0 表示不启用本地缓存）
    LocalTTL       time.Duration // 本地缓存 TTL
    RedisClient    *database_redis.Redis // nil 则仅本地模式；non-nil 则内部构建 RedisAdaptor
    RemoteExpiry   time.Duration
    NotFoundExpiry time.Duration
    StatsDisabled  bool
}
func New(c *Config) cache.Cache
```
**说明：** Config 不暴露 `local.Local`/`remote.Remote` jetcache-go 内部类型。`New()` 内部根据 `LocalMaxSize` 构建 `local.NewFreeCache`，根据 `RedisClient` 是否为 nil 决定模式。调用方无需直接 import jetcache-go 子包即可完成配置。`cache.Cache` 接口本身来自 jetcache-go 顶层包，是不可避免的直接依赖。

### resilience/
**依赖：** `github.com/afex/hystrix-go`（公开包，go-lib2 私有 fork 的上游来源）。  
**维护状态：** afex/hystrix-go 最后 commit 为 2018 年，属于 feature-complete 但无活跃维护。对于熔断场景够用，后续可按需迁移至 `sony/gobreaker`（已在 roadmap 备注）。

`afex/hystrix-go` 的 `Execute` 不支持 `context.Context`（内部用独立 goroutine + 超时控制），接口设计对齐 hystrix 实际能力：

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
    // fallback 为 nil 时不做降级，直接返回原始错误
    Do(run func() error, fallback func(error) error) error
}
func New(c *Config) Breaker
```
熔断时 `Do` 返回 `hystrix.ErrCircuitOpen`。**不**在接口上暴露 `ctx`，避免语义误导（hystrix 超时由 Config.Timeout 控制，非 ctx 取消）。

### app/middleware/
```go
// prometheus.go — 依赖 metrics/
func Prometheus() gin.HandlerFunc

// request_log.go — 依赖 log/，记录 method/path/status/耗时
func RequestLog() gin.HandlerFunc

// rate_limit.go — 滑动窗口，接受 redis.Cmdable（调用方从 *redis.Redis 取原生客户端）
type RateLimitConfig struct {
    Limit   int
    Window  time.Duration
    KeyFunc func(*gin.Context) string  // 默认按 RemoteIP
}
func RateLimit(rdb redis.Cmdable, cfg RateLimitConfig) gin.HandlerFunc

// token_auth.go — Bearer token，纯标准库
type VerifyFunc func(ctx context.Context, token string) (bool, error)
func TokenAuth(verify VerifyFunc, whitelist ...string) gin.HandlerFunc
```

**RateLimit 与 Redis 客户端的关系：**  
`RateLimit` 接受 `redis.Cmdable`（go-redis/v8 接口）而非 `*redis.Redis`，方便直接传入任意 go-redis 客户端。使用 `database/redis.New()` 时，调用方通过 `.Cluster()` / `.Standalone()` 取 `redis.Cmdable`：

```go
rdb := redis.New(&cfg.Redis)
r.Use(middleware.RateLimit(rdb.Cluster(), rateCfg))  // cluster 模式
// 或
r.Use(middleware.RateLimit(rdb.Standalone(), rateCfg))  // standalone 模式
```

## 初始化数据流

```
config.New("config.yaml")
  ├─ log.Init(&cfg.Log)
  ├─ database/redis.New(&cfg.Redis)          → *redis.Redis
  ├─ database/mysql.New(&cfg.MySQL)          → *gorm.DB
  ├─ cache.New(&cfg.Cache)                   → cache.Cache（传入 NewRedisAdaptor(rdb)）
  └─ resilience.New(&cfg.Breaker)            → Breaker

gin.Engine
  ├─ middleware.RequestID()
  ├─ middleware.RequestLog()
  ├─ middleware.Prometheus()
  ├─ middleware.TokenAuth(fn)
  └─ middleware.RateLimit(rdb.Cluster(), cfg)
```

## 错误处理原则

- `New()` / `Init()` 配置非法时 **panic**（启动时快速失败）
- `config.Loader.Watch()` setup 失败返回 `error`（fsnotify 初始化失败属运行时可恢复错误）
- 运行时错误返回 `error`，不 panic
- `Breaker.Do()` 熔断时返回 `hystrix.ErrCircuitOpen`，调用方处理 fallback
- middleware 错误（限流 429、认证 401）直接写 HTTP 响应，不传递给下游

## 测试策略

| 层 | 测试方式 |
|---|---|
| `metrics/` | 单元测试：注册并验证 counter/histogram label 和值 |
| `log/` | 单元测试：Init 不 panic，UDP sink 输出格式正确 |
| `config/` | 单元测试：testdata/ yaml/toml，验证 Scan 反序列化；多 Loader 实例互不干扰 |
| `database/redis/` | 集成测试（可选）：miniredis mock |
| `database/mysql/` | 集成测试（可选）：go-sqlmock |
| `cache/` | 单元测试：仅 local 模式，验证 hit/miss |
| `resilience/` | 单元测试：构造失败函数，验证熔断触发和恢复 |
| `app/middleware/` | 单元测试：httptest.NewRecorder + gin test mode |

## 新增第三方依赖

| 包 | 用途 | 备注 |
|---|---|---|
| `github.com/prometheus/client_golang` | metrics/ | — |
| `github.com/afex/hystrix-go` | resilience/ | 基本稳定，后续可迁移 gobreaker |
| `github.com/mgtv-tech/jetcache-go` | cache/ | — |
| `github.com/spf13/viper` | config/ | — |
| `gorm.io/gorm` + `gorm.io/driver/mysql` | database/mysql/ | — |
| `github.com/go-redis/redis/v8` | database/redis/ + middleware/rate_limit | 已在 go.mod 为 indirect，提升为 direct |

**注：** `log/` 依赖 xlog copy 的内部实现（自定义，无额外第三方包），不引入 logrus。

## 实现顺序

自底向上：
1. `metrics/`
2. `log/`
3. `config/`
4. `database/redis/` + `database/mysql/`
5. `cache/`
6. `resilience/`
7. `app/middleware/`（prometheus → request_log → rate_limit → token_auth）
