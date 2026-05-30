# TODO

## 高优先级

### log/slog 兼容
Go 1.21+ 的 `log/slog` 已是标准日志接口。gorm、go-redis 等第三方库默认用 slog 输出，当前无法汇入自定义 logger，日志格式割裂。
- 为 `log/` 包实现 `slog.Handler` 接口，使其可作为 slog 后端使用

### OpenTelemetry / 分布式追踪
当前只有轻量的 `task_id` 概念，缺少 W3C trace-context 传播和 span 上报。需要在以下层统一接入 OTel SDK：
- `app/middleware` — HTTP server 入口 span
- `app/httpclient` — 出站请求 span 传播
- `database/mysql` — SQL 查询 span
- `database/redis` — Redis 命令 span
- `cache/` — 缓存操作 span

## 中优先级

### async_queue poison message 进死信
`consumer.go` 识别了 poisoned 消息并计 metrics，但只是跳过，消息一直留在 PEL，下次 PEL recovery 重复扫描，造成无限循环。
- 明确 XACK 该消息 + 写死信（另一个 stream 或专用 Redis key），避免 PEL 无限膨胀

### gopool 队列满时无界增长
`utils/gopool/pool.go` 的 `CtxGo` 在 worker 数达到 cap 时继续入队（无界 queue），并发量爆炸时内存无界增长。
- 支持可选的队列容量上限，超限返回 `ErrPoolFull` 或触发 backpressure

### Pulsar 自动重连
`database/pulsar/consumer.go` 只有 `time.Sleep(1s)` 硬编码退避，Broker 重启时会卡死。Producer 侧也无重连逻辑。
- 实现指数退避 + 上限，补充延迟消息支持、批量发送、优雅关闭协调

### config 热重载竞态
`config.go` 的 `fsnotify` 回调里直接写 map，未见显式 `sync.RWMutex` 保护，高频文件变更下可能有读写竞态。
- 用 `-race` 跑压测确认，必要时加锁

### 统一错误包
各包各自定义 sentinel error，缺少统一的业务错误码体系。建议新增 `errors/` 包，提供：
- 结构化错误码（error code + HTTP status 映射）
- 可序列化的错误类型（便于 API 返回）
- 与 `app/response` 打通，service 层到 handler 层错误自动转换

### health check 区分 Liveness / Readiness
`/healthz` 同时做 liveness 和 readiness，K8s 滚动发布时容易被误杀。
- 拆成 `/healthz/live`（进程活着即 OK）和 `/healthz/ready`（依赖全通才 OK）

### httpclient 出站限流
`app/httpclient` 有 breaker 和 retry，但缺少 client-side rate limiter（令牌桶/漏桶）。调用第三方 API 时容易触发对方限流。
- 新增 `middleware_ratelimit.go`，支持按 host 配置 QPS 上限

### 补充 benchmark 测试和 log 测试
`internal/testutil` 已建立，但以下测试仍缺失：
- 零 benchmark 测试（`log/`、`cache/`、`utils/gopool/` 等性能敏感包）
- `log/` 包测试覆盖薄弱（文件轮转、context fields、filter 均未测试）

### Makefile / CI 工具链
缺少标准化构建流程，`.golangci.yml` 已有，补充：
- `Makefile`（build / test / lint / fmt 目标）
- `Dockerfile`（生产镜像构建）

## 低优先级

### gRPC server/client wrapper
`go.mod` 已间接依赖 `google.golang.org/grpc`。如果团队有 gRPC 服务，可以像 `app/httpclient` 一样提供带 middleware chain 的 gRPC server/client wrapper，统一拦截器（metrics、tracing、recovery、auth）。
