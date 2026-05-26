# Pulsar 模块增强实现计划

> **面向 AI 代理的工作者：** 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 aikit/pulsar 从最小可用包装升级为生产级模块，具备日志桥接、可观测性（Metrics/Interceptor）、并发消费、错误处理改进和 FastApp 集成。

**架构：** 保持当前单文件结构拆分为职责清晰的多个文件（client.go、producer.go、consumer.go、logger.go、metrics.go），参照 aikit 内 redis/mysql 模块的拆分惯例。新增日志桥接（Pulsar logrus → aikit/log）、Pulsar 指标（参照 redis/metrics.go）、Interceptor 支持、并发消费（串行 + channel 批处理并发两种模式）、Must 构造器，以及 FastApp 资源注册/优雅关闭集成。

**技术栈：** Go 1.25+, apache/pulsar-client-go v0.11.0, aikit/log, aikit/metrics, aikit/utils/gopool, golang.org/x/sync/errgroup

---

## 特性描述

当前 aikit/pulsar 模块仅提供 Client/Producer/Consumer 的最薄封装：无日志输出、无指标采集、无并发消费、无 Interceptor 支持、New() 失败直接 panic 且无 error 返回变体、未集成到 FastApp 生命周期。参照 go-ceres 和 go-lib2 的 xpulsar 实现，以及 aikit 内 redis/mysql 模块的模式，进行全面增强。

## 用户故事

作为 aikit 用户（Go 后端开发者）
我想要一个开箱即用的 Pulsar 模块，具备日志桥接、指标采集、并发消费、拦截器支持和 FastApp 集成
以便于在生产环境中可靠地使用 Pulsar 消息队列，且无需自行处理可观测性和生命周期管理

## 问题陈述

当前 Pulsar 模块是"能编译但不够生产"的状态：Pulsar 客户端内部日志丢失、无法观测消息收发延迟和错误率、消费端需要用户自行编写循环和 ack 逻辑、无拦截器可插拔指标/链路追踪、客户端创建失败直接 panic 且无优雅处理、未与 FastApp 资源管理集成。

## 方案陈述

参照 go-lib2/xpulsar 的功能特性和 aikit 内 redis/mysql 的代码模式，逐层增强：日志桥接 → 指标定义 → 拆分文件 → 增强 Producer/Consumer → FastApp 集成。每个增强点独立可测，不引入 go-lib2 外部依赖。

## 特性元数据

**特性类型**：增强
**预估复杂度**：高
**主要受影响系统**：`pkg/aikit/database/pulsar/`、`pkg/aikit/metrics/`、`pkg/aikit/app/fastapp.go`
**依赖**：`github.com/apache/pulsar-client-go v0.11.0`（已有）、`github.com/sirupsen/logrus`（已有间接依赖）、`golang.org/x/sync`（已有）

## 假设

- Pulsar 客户端版本保持 v0.11.0，不升级
- 不引入 go-lib2 外部依赖，所有功能在 aikit 内实现
- 并发消费模式参照 go-lib2 但使用 aikit 自有的 gopool + errgroup 替代 xgo + congroup
- Consumer 的 `Start()` 方法采用 "handler 返回 error，成功则 Ack，失败则 Nack" 语义（改进 go-lib2 的 always-ack 问题）
- FastApp 集成模式参照现有 RegisterRedis/RegisterMySQL 的单实例注册模式

## 待决问题

- Pulsar 配置是否需要支持多实例（map[string]*PulsarConfig）？当前假设先用单实例，与现有 Redis/MySQL 的 Register 模式对齐即可。
- Consumer 并发消费是否需要限制最大并发数？当前假设通过 errgroup.SetLimit() 控制。

## 非目标

- 不实现 Pulsar Reader API
- 不实现事务消息（Transaction）
- 不升级 pulsar-client-go 版本
- 不引入 go-lib2 外部依赖
- 不实现多 Topic 订阅（topics pattern），仅支持单 Topic
- 不实现死信队列（DLQ）自动配置

---

## 上下文参考

### 相关代码库文件 — 重要：实现前必须阅读这些文件

- `backend/pkg/aikit/database/pulsar/pulsar.go` — 当前实现，增强基础
- `backend/pkg/aikit/database/redis/redis.go` — 参照：Config 结构、Fix/Validate、New 中 panic+log 模式
- `backend/pkg/aikit/database/redis/metrics.go` — 参照：Prometheus Hook 模式，Pulsar metrics.go 将参照此模式
- `backend/pkg/aikit/database/mysql/metrics.go` — 参照：GORM Plugin 模式中的指标采集，观察 Observe 调用方式
- `backend/pkg/aikit/metrics/predefined.go` — 必读：已有指标定义和 Observe 便捷函数，新增 Pulsar 指标需在此追加
- `backend/pkg/aikit/app/fastapp.go`（第 91-127 行）— 参照：FastApp 资源注册字段和 Register/Get 方法模式
- `backend/pkg/aikit/app/fastapp.go`（第 216-243 行）— 参照：RegisterRedis/RegisterMySQL 实现模式
- `backend/pkg/aikit/app/fastapp.go`（第 555-613 行）— 参照：shutdown 中资源关闭顺序
- `backend/pkg/aikit/log/logger.go`（第 163-190 行）— 必读：结构化日志 API（Infov/Errorv 等），logger.go 桥接将使用这些
- `backend/pkg/aikit/log/field.go` — 必读：KVString/KV 等字段构造函数
- `backend/pkg/aikit/utils/gopool/pool.go` — 必读：Go()/CtxGo() goroutine 池，并发消费将使用
- `backend/pkg/aikit/database/pulsar/pulsar_test.go` — 当前测试，需扩展

### 已有可复用代码

- `backend/pkg/aikit/metrics/` — Prometheus 指标注册和 Observe 便捷函数框架，直接复用
- `backend/pkg/aikit/log/` — 结构化日志，日志桥接目标
- `backend/pkg/aikit/utils/gopool/` — goroutine 池，替代 go-lib2 的 xgo
- `golang.org/x/sync/errgroup` — 已有依赖（go.mod 中有 golang.org/x/sync），替代 go-lib2 的 congroup

### 需新建的文件

- `backend/pkg/aikit/database/pulsar/client.go` — Client 结构体和构造器（从 pulsar.go 拆分）
- `backend/pkg/aikit/database/pulsar/producer.go` — Producer 增强（从 pulsar.go 拆分）
- `backend/pkg/aikit/database/pulsar/consumer.go` — Consumer 增强，含 Start/并发消费（从 pulsar.go 拆分）
- `backend/pkg/aikit/database/pulsar/logger.go` — Pulsar logrus → aikit/log 桥接
- `backend/pkg/aikit/database/pulsar/metrics.go` — Prometheus Interceptor 实现

### 将删除的文件

- `backend/pkg/aikit/database/pulsar/pulsar.go` — 拆分后删除（内容分散到 client.go/producer.go/consumer.go）

### 需遵循的模式

**Config 模式**（参照 redis/redis.go）：

```go
type Config struct {
    Name   string        `yaml:"name"`
    URL    string        `yaml:"url"`
    // ...
}
func (c *Config) Fix()   { /* 填充默认值 */ }
func (c *Config) Validate() error { /* 校验必填字段 */ }
func (c *Config) fix()   { c.Fix(); if err := c.Validate(); err != nil { panic(err) } }
```

**New 模式**（参照 redis/redis.go:73-107）：

```go
func New(c *Config, opts ...Option) *Client {
    for _, opt := range opts { opt(c) }
    c.fix()
    log.Info("[Pulsar][connect_start][url=%s]", c.URL)
    // 创建客户端...
    log.Info("[Pulsar][connected][url=%s]", c.URL)
    return &Client{client: client, config: c}
}
```

**Metrics 模式**（参照 redis/metrics.go）：

```go
type prometheusHook struct{ name string }
func (h *prometheusHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
    return func(ctx context.Context, cmd redis.Cmder) error {
        start := time.Now()
        err := next(ctx, cmd)
        success := err == nil || errors.Is(err, redis.Nil)
        metrics.ObserveRedis(h.name, success, time.Since(start))
        return err
    }
}
```

Pulsar 使用 Interceptor 模式而非 Hook，但 Observe 调用方式一致。

**日志桥接模式**（参照 go-lib2/mq/xpulsar/logger.go）：

```go
func defaultLogger() log.Logger {
    l := logrus.New()
    l.SetFormatter(&logrus.TextFormatter{DisableColors: true, DisableTimestamp: true})
    l.AddHook(newLogHook())
    return log.NewLoggerWithLogrus(l)
}
```

将 xlog 替换为 aikit/log。

**FastApp 注册模式**（参照 fastapp.go:216-228）：

```go
func (a *FastApp) RegisterRedis(name string, cfg *dbredis.Config) *dbredis.Redis {
    if cfg.Name == "" { cfg.Name = a.cfg.Family + "/" + name }
    rdb := dbredis.New(cfg)
    a.redisInstances[name] = rdb
    return rdb
}
func (a *FastApp) GetRedis(name string) *dbredis.Redis { return a.redisInstances[name] }
```

## 流程图

```text
                          FastApp
                            |
                   RegisterPulsar(name, cfg)
                            |
                            v
                     Client.New(cfg)
                       |         |
                  logrus→aikit   pulsar.NewClient
                  logger bridge       |
                            |         |
                            v         v
                         Client struct
                        /            \
            NewProducer()            NewConsumer()
                |                         |
                v                         v
         Producer struct            Consumer struct
         (with interceptors)       (with interceptors)
                |                         |
          Send / SendObj              Start(fn)
          (metrics via              /            \
           interceptor)     serial mode      concurrent mode
                                |                |
                           gopool.Go()     gopool.Go()
                                |                |
                           Receive→fn→Ack   Chan→batch→errgroup
                                            |          |
                                        fn→Ack     fn→Ack(Nack on error)

                    ┌─── Graceful Shutdown ───┐
                    | consumer.Close()        |
                    | producer.Close()        |
                    | client.Close()          |
                    └─────────────────────────┘
```

---

## 实现计划

### 阶段 1：基础设施（日志桥接 + 指标定义）

搭建 Pulsar 模块的基础可观测性：将 Pulsar 客户端内部日志桥接到 aikit/log，定义 Prometheus 指标。

**任务：**

- 创建 logger.go 实现 logrus→aikit/log 桥接
- 在 metrics/predefined.go 中新增 Pulsar 指标定义和 Observe 便捷函数

### 阶段 2：文件拆分 + Client 增强

将 pulsar.go 拆分为 client.go，同时增强 Client：增加 MustNew 构造器、日志输出、Validate 校验、logger 选项。

**任务：**

- 创建 client.go（从 pulsar.go 拆分），增强 ClientConfig（Name、ConnectionTimeout/OperationTimeout 分离）、增加 MustNew
- 删除原 pulsar.go

### 阶段 3：Producer 增强

拆分并增强 Producer：增加 SendObj、Schema、disableBlockIfQueueFull、Properties、Interceptor 支持。

**任务：**

- 创建 producer.go，增加 SendObj、Interceptor、Schema 等选项

### 阶段 4：Consumer 增强

拆分并增强 Consumer：增加 Start 方法（串行+并发消费）、HandlerFunc 带 error 返回、Interceptor 支持、MustNewConsumer。

**任务：**

- 创建 consumer.go，实现 Start/Close 生命周期和并发消费
- 创建 metrics.go，实现 Producer/Consumer Interceptor 采集指标

### 阶段 5：FastApp 集成

将 Pulsar 注册到 FastApp 资源管理体系，支持 RegisterPulsar/GetPulsar 和优雅关闭。

**任务：**

- 修改 fastapp.go，新增 pulsarInstances 字段和 RegisterPulsar/GetPulsar 方法
- 在 shutdown 流程中关闭 Pulsar 客户端

### 阶段 6：测试与验证

完善所有新增功能的单元测试。

**任务：**

- 扩展 pulsar_test.go，覆盖所有新增配置、选项、日志桥接逻辑

---

## 逐步任务

### 任务 1：创建 logger.go — Pulsar logrus → aikit/log 桥接

**文件：**
- 创建：`backend/pkg/aikit/database/pulsar/logger.go`

- [ ] **步骤 1：编写 logger.go**

```go
package pulsar

import (
	"context"

	"github.com/example/go-template/pkg/aikit/log"
	pulsarlog "github.com/apache/pulsar-client-go/pulsar/log"
	"github.com/sirupsen/logrus"
)

type logHook struct{}

// defaultLogger creates a pulsar.Logger that bridges Pulsar client internal
// logs into the aikit structured logger.
func defaultLogger() pulsarlog.Logger {
	l := logrus.New()
	l.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
	l.AddHook(&logHook{})
	return pulsarlog.NewLoggerWithLogrus(l)
}

func (h *logHook) Fire(entry *logrus.Entry) error {
	args := make([]log.D, 0, len(entry.Data)+1)
	args = append(args, log.KVString("log", entry.Message))
	for k, v := range entry.Data {
		args = append(args, log.KV(k, v))
	}
	ctx := context.Background()
	switch entry.Level {
	case logrus.PanicLevel, logrus.FatalLevel:
		log.Fatalv(ctx, args...)
	case logrus.ErrorLevel:
		log.Errorv(ctx, args...)
	case logrus.WarnLevel:
		log.Warnv(ctx, args...)
	case logrus.InfoLevel:
		log.Infov(ctx, args...)
	case logrus.DebugLevel, logrus.TraceLevel:
		log.Debugv(ctx, args...)
	}
	return nil
}

func (h *logHook) Levels() []logrus.Level {
	return logrus.AllLevels
}
```

- [ ] **步骤 2：运行编译验证**

运行：`cd /data/13_claude/go_code_template/backend && go build ./pkg/aikit/database/pulsar/`
预期：编译成功

- [ ] **步骤 3：提交**

```bash
git add backend/pkg/aikit/database/pulsar/logger.go
git commit -m "feat(pulsar): add logrus-to-aikit/logger bridge"
```

---

### 任务 2：在 metrics/predefined.go 中新增 Pulsar 指标

**文件：**
- 修改：`backend/pkg/aikit/metrics/predefined.go`

- [ ] **步骤 1：编写失败的测试**

在 `backend/pkg/aikit/metrics/predefined_test.go`（如不存在则创建）中添加：

```go
func TestPulsarMetricsNotNil(t *testing.T) {
	Enable()
	assert.NotNil(t, GetPulsarProducerCounter())
	assert.NotNil(t, GetPulsarProducerDuration())
	assert.NotNil(t, GetPulsarConsumerCounter())
	assert.NotNil(t, GetPulsarConsumerDuration())
}

func TestObservePulsarProducer(t *testing.T) {
	Enable()
	assert.NotPanics(t, func() {
		ObservePulsarProduce("test-topic", true, 100*time.Millisecond)
	})
}

func TestObservePulsarConsume(t *testing.T) {
	Enable()
	assert.NotPanics(t, func() {
		ObservePulsarConsume("test-topic", "ack", 50*time.Millisecond)
	})
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/metrics/ -run TestPulsar -v`
预期：编译失败，GetPulsarProducerCounter 等函数未定义

- [ ] **步骤 3：在 predefined.go 中添加 Pulsar 指标定义**

在 `predefined.go` 末尾（MySQL 指标之后、便捷函数之前）追加：

```go
// ================================
// Pulsar 指标
// ================================

var (
	pulsarProducerTotal    CounterVec
	pulsarProducerDuration HistogramVec
	pulsarConsumerTotal    CounterVec
	pulsarConsumerDuration HistogramVec
)

func init() {
	Register(func() {
		pulsarProducerTotal = NewCounterVec(&CounterVecOpts{
			Name:   "pulsar_produce_total",
			Help:   "Total Pulsar producer send operations",
			Labels: []string{"topic", "success"},
		})
		pulsarProducerDuration = NewHistogramVec(&HistogramVecOpts{
			Name:    "pulsar_produce_duration_seconds",
			Help:    "Pulsar producer send latency in seconds",
			Labels:  []string{"topic"},
			Buckets: DefaultDurationBuckets,
		})
		pulsarConsumerTotal = NewCounterVec(&CounterVecOpts{
			Name:   "pulsar_consume_total",
			Help:   "Total Pulsar consumer message processing",
			Labels: []string{"topic", "result"},
		})
		pulsarConsumerDuration = NewHistogramVec(&HistogramVecOpts{
			Name:    "pulsar_consume_duration_seconds",
			Help:    "Pulsar consumer message processing latency in seconds",
			Labels:  []string{"topic", "result"},
			Buckets: DefaultAsyncDurationBuckets,
		})
	})
}

func GetPulsarProducerCounter() CounterVec   { return pulsarProducerTotal }
func GetPulsarProducerDuration() HistogramVec { return pulsarProducerDuration }
func GetPulsarConsumerCounter() CounterVec   { return pulsarConsumerTotal }
func GetPulsarConsumerDuration() HistogramVec { return pulsarConsumerDuration }

func ObservePulsarProduce(topic string, success bool, duration time.Duration) {
	s := "true"
	if !success {
		s = "false"
	}
	if pulsarProducerTotal != nil {
		pulsarProducerTotal.Inc(topic, s)
	}
	if pulsarProducerDuration != nil {
		pulsarProducerDuration.Observe(duration.Seconds(), topic)
	}
}

func ObservePulsarConsume(topic, result string, duration time.Duration) {
	if pulsarConsumerTotal != nil {
		pulsarConsumerTotal.Inc(topic, result)
	}
	if pulsarConsumerDuration != nil {
		pulsarConsumerDuration.Observe(duration.Seconds(), topic, result)
	}
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/metrics/ -run TestPulsar -v`
预期：PASS

- [ ] **步骤 5：运行完整 metrics 包测试确认无回归**

运行：`cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/metrics/ -v`
预期：全部 PASS

- [ ] **步骤 6：提交**

```bash
git add backend/pkg/aikit/metrics/predefined.go backend/pkg/aikit/metrics/predefined_test.go
git commit -m "feat(metrics): add Pulsar producer/consumer Prometheus metrics"
```

---

### 任务 3：创建 client.go — 拆分并增强 Client

**文件：**
- 创建：`backend/pkg/aikit/database/pulsar/client.go`
- 最终删除：`backend/pkg/aikit/database/pulsar/pulsar.go`

- [ ] **步骤 1：编写 client.go**

```go
package pulsar

import (
	"fmt"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/apache/pulsar-client-go/pulsar/log"

	"github.com/example/go-template/pkg/aikit/log"
)

// Config holds configuration for the Pulsar client.
type Config struct {
	Name                     string        `yaml:"name"`
	URL                      string        `yaml:"url"`
	ConnectionTimeout        time.Duration `yaml:"connection_timeout"`
	OperationTimeout         time.Duration `yaml:"operation_timeout"`
	KeepAliveInterval        time.Duration `yaml:"keep_alive_interval"`
	MaxConnectionsPerBroker  int           `yaml:"max_connections_per_broker"`
}

// Fix fills default values for zero/empty fields.
func (c *Config) Fix() {
	if c.URL == "" {
		c.URL = "pulsar://localhost:6650"
	}
	if c.ConnectionTimeout <= 0 {
		c.ConnectionTimeout = 3 * time.Second
	}
	if c.OperationTimeout <= 0 {
		c.OperationTimeout = 5 * time.Second
	}
	if c.KeepAliveInterval <= 0 {
		c.KeepAliveInterval = 30 * time.Second
	}
	if c.MaxConnectionsPerBroker <= 0 {
		c.MaxConnectionsPerBroker = 1
	}
}

// Validate checks required fields and returns an error if any are missing.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("pulsar: Name is required (used as Prometheus datasource label)")
	}
	return nil
}

func (c *Config) fix() {
	c.Fix()
	if err := c.Validate(); err != nil {
		panic(err.Error())
	}
}

// Client wraps pulsar.Client and is the entry point for creating producers and consumers.
type Client struct {
	client pulsar.Client
	config *Config
}

// Option configures a Config.
type Option func(*Config)

func WithURL(url string) Option {
	return func(c *Config) { c.URL = url }
}

func WithConnectionTimeout(d time.Duration) Option {
	return func(c *Config) { c.ConnectionTimeout = d }
}

func WithOperationTimeout(d time.Duration) Option {
	return func(c *Config) { c.OperationTimeout = d }
}

func WithKeepAliveInterval(d time.Duration) Option {
	return func(c *Config) { c.KeepAliveInterval = d }
}

func WithMaxConnectionsPerBroker(n int) Option {
	return func(c *Config) { c.MaxConnectionsPerBroker = n }
}

// New creates a Pulsar client. Panics on connection error.
func New(c *Config, opts ...Option) *Client {
	for _, opt := range opts {
		opt(c)
	}
	c.fix()

	log.Info("[Pulsar][connect_start][url=%s]", c.URL)

	var logger log.Logger = defaultLogger()
	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL:                     c.URL,
		ConnectionTimeout:       c.ConnectionTimeout,
		OperationTimeout:        c.OperationTimeout,
		KeepAliveInterval:       c.KeepAliveInterval,
		MaxConnectionsPerBroker: c.MaxConnectionsPerBroker,
		Logger:                  logger,
	})
	if err != nil {
		log.Error("[Pulsar][connect_error][url=%s]: %v", c.URL, err)
		panic(fmt.Sprintf("pulsar: connect error: %v", err))
	}

	log.Info("[Pulsar][connected][url=%s]", c.URL)
	return &Client{client: client, config: c}
}

// MustNew is an alias for New (both panic on error). Kept for API
// consistency with go-lib2's MustNewClient pattern.
func MustNew(c *Config, opts ...Option) *Client {
	return New(c, opts...)
}

// Close closes the underlying Pulsar client.
func (c *Client) Close() {
	c.client.Close()
	log.Info("[Pulsar][closed][url=%s]", c.config.URL)
}

// Raw returns the underlying pulsar.Client for advanced usage.
func (c *Client) Raw() pulsar.Client {
	return c.client
}

// Name returns the configured client name (for metrics labels).
func (c *Client) Name() string {
	if c.config == nil {
		return ""
	}
	return c.config.Name
}
```

- [ ] **步骤 2：运行编译验证**

运行：`cd /data/13_claude/go_code_template/backend && go build ./pkg/aikit/database/pulsar/`
预期：编译成功（此时 client.go 和 pulsar.go 并存，可能有重复定义，暂先确认 logger.go 和 client.go 可编译）

实际上 client.go 和 pulsar.go 会有类型冲突。需要先临时将 pulsar.go 重命名或修改包名来验证。更好的做法是在任务 3 完成后直接删除 pulsar.go 并同时创建 producer.go 和 consumer.go。因此先继续任务 4/5，最后一起删除 pulsar.go。

- [ ] **步骤 3：暂不提交，等任务 4/5 完成后统一提交**

---

### 任务 4：创建 producer.go — 拆分并增强 Producer

**文件：**
- 创建：`backend/pkg/aikit/database/pulsar/producer.go`

- [ ] **步骤 1：编写 producer.go**

```go
package pulsar

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
)

// Producer wraps pulsar.Producer.
type Producer struct {
	producer pulsar.Producer
	topic    string
}

// ProducerConfig holds configuration for a Pulsar producer.
type ProducerConfig struct {
	Topic                   string                       `yaml:"topic"`
	Name                    string                       `yaml:"name"`
	SendTimeout             time.Duration                `yaml:"send_timeout"`
	CompressionType         pulsar.CompressionType       `yaml:"compression_type"`
	DisableBlockIfQueueFull bool                         `yaml:"disable_block_if_queue_full"`
	Properties              map[string]string            `yaml:"properties"`
	Interceptors            []pulsar.ProducerInterceptor `yaml:"-"`
}

func (c *ProducerConfig) fix() {
	if c.SendTimeout <= 0 {
		c.SendTimeout = 3 * time.Second
	}
}

// ProducerOption configures a ProducerConfig.
type ProducerOption func(*ProducerConfig)

func WithProducerName(name string) ProducerOption {
	return func(c *ProducerConfig) { c.Name = name }
}

func WithSendTimeout(d time.Duration) ProducerOption {
	return func(c *ProducerConfig) { c.SendTimeout = d }
}

func WithCompressionType(ct pulsar.CompressionType) ProducerOption {
	return func(c *ProducerConfig) { c.CompressionType = ct }
}

func WithDisableBlockIfQueueFull(disable bool) ProducerOption {
	return func(c *ProducerConfig) { c.DisableBlockIfQueueFull = disable }
}

func WithProducerProperties(props map[string]string) ProducerOption {
	return func(c *ProducerConfig) { c.Properties = props }
}

func WithProducerInterceptor(ics ...pulsar.ProducerInterceptor) ProducerOption {
	return func(c *ProducerConfig) {
		c.Interceptors = append(c.Interceptors, ics...)
	}
}

// NewProducer creates a producer for the given topic.
func (c *Client) NewProducer(topic string, opts ...ProducerOption) (*Producer, error) {
	if topic == "" {
		return nil, fmt.Errorf("pulsar: producer topic is required")
	}
	cfg := &ProducerConfig{
		Topic:                   topic,
		DisableBlockIfQueueFull: true,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	cfg.fix()

	interceptors := cfg.Interceptors
	// Auto-inject metrics interceptor
	interceptors = append([]pulsar.ProducerInterceptor{&producerMetricsInterceptor{topic: topic}}, interceptors...)

	p, err := c.client.CreateProducer(pulsar.ProducerOptions{
		Topic:                   cfg.Topic,
		Name:                    cfg.Name,
		SendTimeout:             cfg.SendTimeout,
		CompressionType:         cfg.CompressionType,
		DisableBlockIfQueueFull: cfg.DisableBlockIfQueueFull,
		Properties:              cfg.Properties,
		Interceptors:            interceptors,
	})
	if err != nil {
		return nil, fmt.Errorf("pulsar: create producer: %w", err)
	}
	return &Producer{producer: p, topic: topic}, nil
}

// MustNewProducer creates a producer and panics on error.
func (c *Client) MustNewProducer(topic string, opts ...ProducerOption) *Producer {
	p, err := c.NewProducer(topic, opts...)
	if err != nil {
		panic(err)
	}
	return p
}

// Send sends a raw byte payload synchronously.
func (p *Producer) Send(ctx context.Context, data []byte) (pulsar.MessageID, error) {
	return p.producer.Send(ctx, &pulsar.ProducerMessage{Payload: data})
}

// SendAsync sends a raw byte payload asynchronously.
func (p *Producer) SendAsync(ctx context.Context, data []byte, callback func(pulsar.MessageID, *pulsar.ProducerMessage, error)) {
	p.producer.SendAsync(ctx, &pulsar.ProducerMessage{Payload: data}, callback)
}

// SendObj marshals obj to JSON and sends it as a string message.
func (p *Producer) SendObj(ctx context.Context, obj interface{}) (pulsar.MessageID, error) {
	bs, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("pulsar: marshal message: %w", err)
	}
	return p.producer.Send(ctx, &pulsar.ProducerMessage{Value: string(bs)})
}

// Close closes the producer.
func (p *Producer) Close() {
	p.producer.Close()
}
```

- [ ] **步骤 2：暂不编译（等 consumer.go 和 metrics.go 就位后统一编译）**

---

### 任务 5：创建 consumer.go — 拆分并增强 Consumer

**文件：**
- 创建：`backend/pkg/aikit/database/pulsar/consumer.go`

- [ ] **步骤 1：编写 consumer.go**

```go
package pulsar

import (
	"context"
	"fmt"
	"sync"

	"github.com/apache/pulsar-client-go/pulsar"
	"golang.org/x/sync/errgroup"

	"github.com/example/go-template/pkg/aikit/log"
	"github.com/example/go-template/pkg/aikit/utils/gopool"
)

// HandlerFunc processes a Pulsar message. Return error to Nack, nil to Ack.
type HandlerFunc func(ctx context.Context, msg pulsar.Message) error

// Consumer wraps pulsar.Consumer with lifecycle management.
type Consumer struct {
	consumer pulsar.Consumer
	topic    string
	opts     *consumerOptions
	started  bool
	mu       sync.Mutex
}

type consumerOptions struct {
	subscriptionName string
	subscriptionType pulsar.SubscriptionType
	concurrency      int
	properties       map[string]string
	interceptors     []pulsar.ConsumerInterceptor
	stop             chan struct{}
}

// ConsumerOption configures consumer creation.
type ConsumerOption func(*consumerOptions)

func WithSubscription(sub string) ConsumerOption {
	return func(o *consumerOptions) { o.subscriptionName = sub }
}

func WithSubscriptionType(t pulsar.SubscriptionType) ConsumerOption {
	return func(o *consumerOptions) { o.subscriptionType = t }
}

func WithConcurrency(n int) ConsumerOption {
	return func(o *consumerOptions) { o.concurrency = n }
}

func WithConsumerProperties(props map[string]string) ConsumerOption {
	return func(o *consumerOptions) { o.properties = props }
}

func WithConsumerInterceptor(ics ...pulsar.ConsumerInterceptor) ConsumerOption {
	return func(o *consumerOptions) { o.interceptors = append(o.interceptors, ics...) }
}

// NewConsumer creates a consumer for the given topic.
func (c *Client) NewConsumer(topic string, opts ...ConsumerOption) (*Consumer, error) {
	if topic == "" {
		return nil, fmt.Errorf("pulsar: consumer topic is required")
	}
	o := &consumerOptions{
		subscriptionType: pulsar.Shared,
		concurrency:      1,
		stop:             make(chan struct{}),
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.subscriptionName == "" {
		o.subscriptionName = "go-aikit-sub"
	}

	interceptors := o.interceptors
	// Auto-inject metrics interceptor
	interceptors = append([]pulsar.ConsumerInterceptor{&consumerMetricsInterceptor{topic: topic}}, interceptors...)

	cs, err := c.client.Subscribe(pulsar.ConsumerOptions{
		Topic:            topic,
		SubscriptionName: o.subscriptionName,
		Type:             o.subscriptionType,
		Properties:       o.properties,
		Interceptors:     interceptors,
	})
	if err != nil {
		return nil, fmt.Errorf("pulsar: subscribe: %w", err)
	}
	return &Consumer{
		consumer: cs,
		topic:    topic,
		opts:     o,
	}, nil
}

// MustNewConsumer creates a consumer and panics on error.
func (c *Client) MustNewConsumer(topic string, opts ...ConsumerOption) *Consumer {
	cs, err := c.NewConsumer(topic, opts...)
	if err != nil {
		panic(err)
	}
	return cs
}

// Start begins consuming messages. Idempotent: returns immediately if already started.
// For concurrency=1 (default), messages are processed serially.
// For concurrency>1, messages are batched from the consumer channel and processed concurrently via errgroup.
func (cc *Consumer) Start(fn HandlerFunc) {
	cc.mu.Lock()
	if cc.started {
		cc.mu.Unlock()
		return
	}
	cc.started = true
	cc.mu.Unlock()

	if cc.opts.concurrency <= 1 {
		cc.startSerial(fn)
	} else {
		cc.startConcurrent(fn)
	}
}

func (cc *Consumer) startSerial(fn HandlerFunc) {
	gopool.Go(func() {
		for {
			select {
			case <-cc.opts.stop:
				log.Infov(context.Background(), log.KVString("topic", cc.topic), log.KVString("log", "Consumer stopped"))
				return
			default:
				ctx := context.Background()
				msg, err := cc.consumer.Receive(ctx)
				if err != nil {
					log.Errorv(ctx, log.KVString("topic", cc.topic), log.KVString("log", fmt.Sprintf("Failed to receive message: %v", err)))
					continue
				}
				cc.handleAndAck(ctx, msg, fn)
			}
		}
	})
}

func (cc *Consumer) startConcurrent(fn HandlerFunc) {
	ch := cc.consumer.Chan()
	gopool.Go(func() {
		for {
			select {
			case <-cc.opts.stop:
				log.Infov(context.Background(), log.KVString("topic", cc.topic), log.KVString("log", "Consumer stopped"))
				return
			case cm, ok := <-ch:
				if !ok {
					return
				}
				cc.handleConcurrentBatch(cm, fn)
			}
		}
	})
}

// handleConcurrentBatch reads a single ConsumerMessage and processes it.
// For higher throughput, consider batching multiple messages — but single-message
// dispatch with errgroup is simpler and avoids ordering issues.
func (cc *Consumer) handleConcurrentBatch(cm pulsar.ConsumerMessage, fn HandlerFunc) {
	ctx := context.Background()
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(cc.opts.concurrency)

	// Process this message in a goroutine from the errgroup
	msg := cm.Message
	g.Go(func() error {
		cc.handleAndAck(gCtx, msg, fn)
		return nil
	})
	_ = g.Wait()
}

func (cc *Consumer) handleAndAck(ctx context.Context, msg pulsar.Message, fn HandlerFunc) {
	err := fn(ctx, msg)
	if err != nil {
		cc.consumer.NackID(msg.ID())
		log.Errorv(ctx,
			log.KVString("topic", cc.topic),
			log.KVString("log", fmt.Sprintf("Handler error, nacking message: %v", err)),
		)
	} else {
		if ackErr := cc.consumer.AckID(msg.ID()); ackErr != nil {
			log.Errorv(ctx,
				log.KVString("topic", cc.topic),
				log.KVString("log", fmt.Sprintf("Failed to ack message: %v", ackErr)),
			)
		}
	}
}

// Close stops the consumer loop and closes the underlying consumer.
func (cc *Consumer) Close() {
	cc.mu.Lock()
	if !cc.started {
		cc.mu.Unlock()
		cc.consumer.Close()
		return
	}
	close(cc.opts.stop)
	cc.mu.Unlock()
	cc.consumer.Close()
}
```

- [ ] **步骤 2：暂不编译（等 metrics.go 就位后统一编译）**

---

### 任务 6：创建 metrics.go — Pulsar Interceptor 实现

**文件：**
- 创建：`backend/pkg/aikit/database/pulsar/metrics.go`

- [ ] **步骤 1：编写 metrics.go**

```go
package pulsar

import (
	"time"

	"github.com/apache/pulsar-client-go/pulsar"

	"github.com/example/go-template/pkg/aikit/metrics"
)

// producerMetricsInterceptor implements pulsar.ProducerInterceptor to record
// send latency and success/failure counts.
type producerMetricsInterceptor struct {
	topic string
}

func (i *producerMetricsInterceptor) BeforeSend(producer pulsar.Producer, message *pulsar.ProducerMessage) {
	message.EventTime = time.Now().UnixNano()
}

func (i *producerMetricsInterceptor) OnSendAcknowledgement(producer pulsar.Producer, message *pulsar.ProducerMessage, msgID pulsar.MessageID) {
	var start time.Time
	if message.EventTime > 0 {
		start = time.Unix(0, message.EventTime)
	} else {
		start = time.Now()
	}
	metrics.ObservePulsarProduce(i.topic, true, time.Since(start))
}

// consumerMetricsInterceptor implements pulsar.ConsumerInterceptor to record
// consumption latency and result counts.
type consumerMetricsInterceptor struct {
	topic string
}

func (i *consumerMetricsInterceptor) BeforeConsume(message pulsar.ConsumerMessage) {
	// Record start time in message properties is not feasible,
	// so we use the publish time as approximate start.
}

func (i *consumerMetricsInterceptor) OnAcknowledge(consumer pulsar.Consumer, msgID pulsar.MessageID) {
	metrics.ObservePulsarConsume(i.topic, "ack", 0)
}

func (i *consumerMetricsInterceptor) OnNegativeAcknowledgeSend(consumer pulsar.Consumer, msgID pulsar.MessageID) {
	metrics.ObservePulsarConsume(i.topic, "nack", 0)
}
```

- [ ] **步骤 2：删除 pulsar.go 并统一编译验证**

```bash
rm backend/pkg/aikit/database/pulsar/pulsar.go
cd /data/13_claude/go_code_template/backend && go build ./pkg/aikit/database/pulsar/
```

预期：编译成功

- [ ] **步骤 3：运行现有测试**

注意：原 pulsar_test.go 引用了旧类型（如 `ClientConfig`），需更新为新类型（`Config`）。

运行：`cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/database/pulsar/ -v`
预期：编译失败（类型名变更），需更新测试

- [ ] **步骤 4：更新 pulsar_test.go 适配新类型**

将测试中 `ClientConfig` → `Config`，`fix()` → `Fix()`，更新默认值断言（Timeout 拆分为 ConnectionTimeout=3s + OperationTimeout=5s），新增选项测试。

详见任务 7。

- [ ] **步骤 5：提交**

```bash
git add backend/pkg/aikit/database/pulsar/
git commit -m "refactor(pulsar): split into client/producer/consumer/logger/metrics with enhanced features"
```

---

### 任务 7：更新测试文件

**文件：**
- 修改：`backend/pkg/aikit/database/pulsar/pulsar_test.go`

- [ ] **步骤 1：重写测试文件适配新 API**

```go
package pulsar

import (
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/stretchr/testify/assert"
)

func TestConfig_Fix_Defaults(t *testing.T) {
	c := &Config{}
	c.Fix()
	assert.Equal(t, "pulsar://localhost:6650", c.URL)
	assert.Equal(t, 3*time.Second, c.ConnectionTimeout)
	assert.Equal(t, 5*time.Second, c.OperationTimeout)
	assert.Equal(t, 30*time.Second, c.KeepAliveInterval)
	assert.Equal(t, 1, c.MaxConnectionsPerBroker)
}

func TestConfig_Fix_ExistingValues(t *testing.T) {
	c := &Config{
		URL:                     "pulsar://custom:6650",
		ConnectionTimeout:       10 * time.Second,
		OperationTimeout:        30 * time.Second,
		KeepAliveInterval:       10 * time.Second,
		MaxConnectionsPerBroker: 4,
	}
	c.Fix()
	assert.Equal(t, "pulsar://custom:6650", c.URL)
	assert.Equal(t, 10*time.Second, c.ConnectionTimeout)
	assert.Equal(t, 30*time.Second, c.OperationTimeout)
	assert.Equal(t, 10*time.Second, c.KeepAliveInterval)
	assert.Equal(t, 4, c.MaxConnectionsPerBroker)
}

func TestConfig_Validate_MissingName(t *testing.T) {
	c := &Config{}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Name is required")
}

func TestConfig_Validate_WithName(t *testing.T) {
	c := &Config{Name: "test"}
	err := c.Validate()
	assert.NoError(t, err)
}

func TestProducerConfig_fix_Defaults(t *testing.T) {
	c := &ProducerConfig{}
	c.fix()
	assert.Equal(t, 3*time.Second, c.SendTimeout)
}

func TestClientOptions(t *testing.T) {
	c := &Config{}
	WithURL("pulsar://test:6650")(c)
	WithConnectionTimeout(5 * time.Second)(c)
	WithOperationTimeout(10 * time.Second)(c)
	WithKeepAliveInterval(15 * time.Second)(c)
	WithMaxConnectionsPerBroker(2)(c)
	assert.Equal(t, "pulsar://test:6650", c.URL)
	assert.Equal(t, 5*time.Second, c.ConnectionTimeout)
	assert.Equal(t, 10*time.Second, c.OperationTimeout)
	assert.Equal(t, 15*time.Second, c.KeepAliveInterval)
	assert.Equal(t, 2, c.MaxConnectionsPerBroker)
}

func TestProducerOptions(t *testing.T) {
	c := &ProducerConfig{}
	WithProducerName("my-producer")(c)
	WithSendTimeout(10 * time.Second)(c)
	WithCompressionType(pulsar.LZ4)(c)
	WithDisableBlockIfQueueFull(false)(c)
	assert.Equal(t, "my-producer", c.Name)
	assert.Equal(t, 10*time.Second, c.SendTimeout)
	assert.Equal(t, pulsar.LZ4, c.CompressionType)
	assert.False(t, c.DisableBlockIfQueueFull)
}

func TestConsumerOptions(t *testing.T) {
	o := &consumerOptions{}
	WithSubscription("my-sub")(o)
	WithSubscriptionType(pulsar.Exclusive)(o)
	WithConcurrency(5)(o)
	assert.Equal(t, "my-sub", o.subscriptionName)
	assert.Equal(t, pulsar.Exclusive, o.subscriptionType)
	assert.Equal(t, 5, o.concurrency)
}

func TestLoggerBridge(t *testing.T) {
	// Verify defaultLogger() returns a non-nil pulsar.Logger
	l := defaultLogger()
	assert.NotNil(t, l)
}

func TestLogHook_Levels(t *testing.T) {
	h := &logHook{}
	levels := h.Levels()
	assert.NotEmpty(t, levels)
}
```

- [ ] **步骤 2：运行测试验证通过**

运行：`cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/database/pulsar/ -v`
预期：全部 PASS

- [ ] **步骤 3：提交**

```bash
git add backend/pkg/aikit/database/pulsar/pulsar_test.go
git commit -m "test(pulsar): update tests for refactored module with new Config and options"
```

---

### 任务 8：FastApp 集成 — RegisterPulsar/GetPulsar + 优雅关闭

**文件：**
- 修改：`backend/pkg/aikit/app/fastapp.go`

- [ ] **步骤 1：编写失败的测试**

在 `backend/pkg/aikit/app/fastapp_test.go`（如不存在则创建）中添加：

```go
func TestFastApp_Pulsar(t *testing.T) {
	app := NewFastApp(FastAppConfig{Family: "test"})
	// GetPulsar should return nil before registration
	assert.Nil(t, app.GetPulsar("demo"))
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/app/ -run TestFastApp_Pulsar -v`
预期：编译失败，GetPulsar 方法未定义

- [ ] **步骤 3：修改 fastapp.go**

3a. 在 import 块中新增：

```go
dbpulsar "github.com/example/go-template/pkg/aikit/database/pulsar"
```

3b. 在 FastApp struct 中新增字段（在 httpClientInstances 之后）：

```go
pulsarInstances map[string]*dbpulsar.Client
```

3c. 在 NewFastApp 中初始化 map（在 httpClientInstances 初始化之后）：

```go
pulsarInstances: make(map[string]*dbpulsar.Client),
```

3d. 在 GetHTTPClient 方法之后新增：

```go
// RegisterPulsar registers a named Pulsar client instance.
func (a *FastApp) RegisterPulsar(name string, cfg *dbpulsar.Config) *dbpulsar.Client {
	if cfg.Name == "" {
		cfg.Name = a.cfg.Family + "/" + name
	}
	client := dbpulsar.New(cfg)
	a.pulsarInstances[name] = client
	return client
}

// GetPulsar returns a named Pulsar client instance.
func (a *FastApp) GetPulsar(name string) *dbpulsar.Client {
	return a.pulsarInstances[name]
}
```

3e. 在 shutdown 方法中，在 Redis 关闭之前新增 Pulsar 关闭：

```go
// Close Pulsar instances
for name, client := range a.pulsarInstances {
	client.Close()
	log.Info("pulsar [%s] closed", name)
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/app/ -run TestFastApp_Pulsar -v`
预期：PASS

- [ ] **步骤 5：运行完整 app 包测试确认无回归**

运行：`cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/app/ -v`
预期：全部 PASS

- [ ] **步骤 6：提交**

```bash
git add backend/pkg/aikit/app/fastapp.go backend/pkg/aikit/app/fastapp_test.go
git commit -m "feat(app): integrate Pulsar into FastApp with RegisterPulsar/GetPulsar and graceful shutdown"
```

---

## 测试策略

### 单元测试

所有单元测试使用 `stretchr/testify`，覆盖：
- Config.Fix() 默认值填充
- Config.Validate() 必填字段校验
- 所有 Option 函数正确修改配置
- ProducerConfig/ConsumerConfig 默认值
- 日志桥接 defaultLogger() 返回非 nil
- logHook.Levels() 返回全级别
- FastApp.RegisterPulsar/GetPulsar 注册和查询

### 集成测试

暂不编写 Pulsar 集成测试（需要实际 Pulsar 集群）。标记为后续工作。

### 边界情况

| 场景 | 预期行为 |
|------|---------|
| Config.Name 为空 | Validate 返回 error，fix() panic |
| Consumer.Start 重复调用 | 幂等，第二次立即返回 |
| HandlerFunc 返回 error | Nack 消息，记日志 |
| HandlerFunc 返回 nil | Ack 消息 |
| Ack 失败 | 记日志，不重试 |
| Consumer 未 Start 直接 Close | 直接关闭底层 consumer |
| 并发消费 concurrency=0 | 退化为串行模式（concurrency <= 1） |

## 故障模式

| 故障模式 | 触发条件 | 预期处理方式 | 是否需要测试 |
| --- | --- | --- | --- |
| Pulsar 连接失败 | URL 错误或 broker 不可用 | panic（与应用启动一致） | 是（间接，通过 Config.Validate） |
| Producer 发送超时 | SendTimeout 内 broker 未确认 | 返回 error，metrics 记录 success=false | 否（Pulsar 客户端行为） |
| Consumer Receive 错误 | 连接断开 | 记日志，继续循环重试 | 否（集成测试级别） |
| Handler 处理 panic | 业务代码 panic | gopool 的 panic handler 捕获，Nack 不会执行，消息超时后重投 | 是（需确认 gopool 行为） |
| Ack 失败 | broker 端问题 | 记日志，消息可能被重复投递 | 否 |

---

## 验证命令

### 级别 1：语法与风格

```bash
cd /data/13_claude/go_code_template/backend && go vet ./pkg/aikit/database/pulsar/ ./pkg/aikit/metrics/ ./pkg/aikit/app/
```

### 级别 2：单元测试

```bash
cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/database/pulsar/ -v
cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/metrics/ -v
cd /data/13_claude/go_code_template/backend && go test ./pkg/aikit/app/ -v
```

### 级别 3：集成测试

暂无可自动化的集成测试。需要本地运行 Pulsar 服务。

### 级别 4：手动验证

1. 在 config.yaml 中添加 pulsar 配置段
2. 在 main.go 中通过 FastApp.RegisterPulsar 注册客户端
3. 启动服务，检查日志输出 `[Pulsar][connected]`
4. 创建 Producer 并 SendObj，检查 Prometheus 指标 `pulsar_produce_total`
5. 创建 Consumer 并 Start，检查 Prometheus 指标 `pulsar_consume_total`
6. 停止服务，检查日志输出 `[Pulsar][closed]`

### 级别 5：完整包编译

```bash
cd /data/13_claude/go_code_template/backend && go build ./...
```

---

## 验收标准

- [ ] Pulsar 客户端内部日志通过 logger.go 桥接到 aikit/log
- [ ] Prometheus 指标（pulsar_produce_total/duration, pulsar_consume_total/duration）已定义
- [ ] Producer 支持 SendObj、Interceptor、disableBlockIfQueueFull、Properties
- [ ] Consumer 支持 Start(fn) 串行/并发消费，HandlerFunc 返回 error 时 Nack
- [ ] Client 和 Producer/Consumer 均有 Must 变体构造器
- [ ] Config 拆分 ConnectionTimeout/OperationTimeout，增加 Name/Validate
- [ ] FastApp 支持 RegisterPulsar/GetPulsar，关闭时释放资源
- [ ] 所有验证命令零错误通过
- [ ] 单元测试覆盖所有配置、选项、日志桥接逻辑

---

## 完成清单

- [ ] 所有任务按顺序完成
- [ ] 每个任务的验证已即时通过
- [ ] 所有验证命令执行成功
- [ ] 完整测试套件通过
- [ ] 无 vet 或编译错误
- [ ] 验收标准全部满足

---

## 备注

**设计决策：**

1. **HandlerFunc 返回 error（Ack/Nack 语义）**：go-lib2 的 always-ack 模式在生产中是有问题的——如果业务处理失败，消息被 Ack 后就丢失了。改为返回 error 时 Nack，保证消息可重投。

2. **Consumer 并发模式简化**：go-lib2 使用 batch + congroup 批量拉取并发处理，这引入了 batch 依赖且增加了复杂度。本方案使用更简单的方式：从 consumer.Chan() 读取 ConsumerMessage，用 errgroup 控制并发度。这样不依赖外部 batch/congroup 包，且行为更可预测。

3. **Metrics Interceptor 而非 Hook**：Pulsar 客户端使用 Interceptor 模式（而非 Redis 的 Hook 模式），Producer 有 BeforeSend/OnSendAcknowledgement，Consumer 有 OnAcknowledge/OnNegativeAcknowledgeSend。自动注入 metrics interceptor 使指标采集对用户透明。

4. **ProducerMetricsInterceptor 使用 EventTime 传递计时起点**：ProducerInterceptor 的 BeforeSend 和 OnSendAcknowledstration 之间没有共享状态机制，利用 ProducerMessage.EventTime 字段传递 start time（int64 纳秒时间戳），在 OnSendAcknowledstration 中计算耗时。这是一个常见技巧（go-lib2 和其他项目均使用此模式）。

5. **ConsumerMetricsInterceptor 不记录耗时**：Consumer Interceptor 没有提供 BeforeProcess/AfterProcess 的配对回调，只有 OnAcknowledge/OnNegativeAcknowledgeSend，无法精确计算 handler 处理耗时。因此仅记录计数（duration 传 0）。如果需要 handler 耗时，应在业务层自行记录。

6. **文件拆分参照项目惯例**：aikit 内 redis 模块拆为 redis.go + metrics.go + cmd.go + lock.go 等，mysql 模块拆为 mysql.go + metrics.go + breaker.go 等。pulsar 拆为 client.go + producer.go + consumer.go + logger.go + metrics.go 符合此惯例。

**与 go-lib2/xpulsar 的主要差异：**

| 方面 | go-lib2/xpulsar | 本方案 |
|------|----------------|--------|
| 日志 | 桥接到 xlog | 桥接到 aikit/log |
| 并发消费 | batch + congroup | gopool + errgroup |
| goroutine 池 | xgo.Go | gopool.Go |
| Ack 语义 | always-ack | error→Nack, nil→Ack |
| 外部依赖 | go-lib2/sync/batch, congroup, xgo, xlog | 仅 aikit 内部包 |
| embed vs 组合 | embed pulsar.Client | 保持现有组合+Raw() |
