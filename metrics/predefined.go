// Package metrics Prometheus 指标采集与管理.
package metrics

import (
	"strconv"
	"time"
)

// DefaultDurationBuckets 常规请求耗时的默认分桶.
var DefaultDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}

// DefaultAsyncDurationBuckets 异步任务耗时的默认分桶，上限扩展到 5 分钟.
var DefaultAsyncDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 300.0}

const ns = "aikit"

func boolLabel(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ================================
// HTTP 请求指标
// ================================.

var (
	httpRequestsTotal   CounterVec
	httpRequestDuration HistogramVec
)

func init() {
	Register(func() {
		httpRequestsTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "http_requests_total",
			Help:      "Total HTTP requests",
			Labels:    []string{"method", "endpoint", "status"},
		})
		httpRequestDuration = NewHistogramVec(&HistogramVecOpts{
			Namespace: ns,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency in seconds",
			Labels:    []string{"method", "endpoint"},
			Buckets:   DefaultDurationBuckets,
		})
	})
}

// GetHTTPRequestCounter 获取 HTTP 请求计数器.
// 参数：无.
// 返回值：CounterVec - HTTP 请求计数器向量.
func GetHTTPRequestCounter() CounterVec { return httpRequestsTotal }

// GetHTTPRequestDuration 获取 HTTP 请求耗时直方图.
// 参数：无.
// 返回值：HistogramVec - HTTP 请求耗时直方图向量.
func GetHTTPRequestDuration() HistogramVec { return httpRequestDuration }

// ================================
// 缓存指标
// ================================.

var cacheRequestsTotal CounterVec

func init() {
	Register(func() {
		cacheRequestsTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "cache_requests_total",
			Help:      "Total cache requests",
			Labels:    []string{"name", "level", "result"},
		})
	})
}

// GetCacheRequests 获取缓存请求计数器.
// 参数：无.
// 返回值：CounterVec - 缓存请求计数器向量.
func GetCacheRequests() CounterVec { return cacheRequestsTotal }

// ================================
// 熔断器指标
// ================================.

var (
	circuitBreakerState      GaugeVec
	circuitBreakerCallsTotal CounterVec
)

func init() {
	Register(func() {
		circuitBreakerState = NewGaugeVec(&GaugeVecOpts{
			Namespace: ns,
			Name:      "circuit_breaker_state",
			Help:      "Circuit breaker state (0=closed, 1=open, 2=half_open)",
			Labels:    []string{"name"},
		})
		circuitBreakerCallsTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "circuit_breaker_calls_total",
			Help:      "Total circuit breaker calls",
			Labels:    []string{"name", "result"},
		})
	})
}

// GetCircuitBreakerState 获取熔断器状态仪表盘.
// 参数：无.
// 返回值：GaugeVec - 熔断器状态仪表盘向量.
func GetCircuitBreakerState() GaugeVec { return circuitBreakerState }

// GetCircuitBreakerCalls 获取熔断器调用计数器.
// 参数：无.
// 返回值：CounterVec - 熔断器调用计数器向量.
func GetCircuitBreakerCalls() CounterVec { return circuitBreakerCallsTotal }

// ================================
// Async Queue 指标
// ================================.

var (
	asyncQueueEnqueueTotal    CounterVec
	asyncQueueConsumeTotal    CounterVec
	asyncQueueHandlerDuration HistogramVec
)

func init() {
	Register(func() {
		asyncQueueEnqueueTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "async_queue_enqueue_total",
			Help:      "Total tasks enqueued (producer)",
			Labels:    []string{"endpoint", "result"},
		})
		asyncQueueConsumeTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "async_queue_consume_total",
			Help:      "Total tasks consumed (consumer terminal state)",
			Labels:    []string{"endpoint", "result"},
		})
		asyncQueueHandlerDuration = NewHistogramVec(&HistogramVecOpts{
			Namespace: ns,
			Name:      "async_queue_handler_duration_seconds",
			Help:      "Async queue handler execution latency in seconds",
			Labels:    []string{"endpoint", "result"},
			Buckets:   DefaultAsyncDurationBuckets,
		})
	})
}

// GetAsyncQueueEnqueueCounter 获取异步队列入队计数器.
// 参数：无.
// 返回值：CounterVec - 异步队列入队计数器向量.
func GetAsyncQueueEnqueueCounter() CounterVec { return asyncQueueEnqueueTotal }

// GetAsyncQueueConsumeCounter 获取异步队列消费计数器.
// 参数：无.
// 返回值：CounterVec - 异步队列消费计数器向量.
func GetAsyncQueueConsumeCounter() CounterVec { return asyncQueueConsumeTotal }

// GetAsyncQueueHandlerDuration 获取异步队列处理耗时直方图.
// 参数：无.
// 返回值：HistogramVec - 异步队列处理耗时直方图向量.
func GetAsyncQueueHandlerDuration() HistogramVec { return asyncQueueHandlerDuration }

// ================================
// HTTP Client 指标
// ================================.

var (
	httpClientRequestsTotal   CounterVec
	httpClientRequestDuration HistogramVec
)

func init() {
	Register(func() {
		httpClientRequestsTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "http_client_requests_total",
			Help:      "Total HTTP client outgoing requests",
			Labels:    []string{"client", "method", "endpoint", "status"},
		})
		httpClientRequestDuration = NewHistogramVec(&HistogramVecOpts{
			Namespace: ns,
			Name:      "http_client_request_duration_seconds",
			Help:      "HTTP client outgoing request latency in seconds",
			Labels:    []string{"client", "method", "endpoint"},
			Buckets:   DefaultDurationBuckets,
		})
	})
}

// GetHTTPClientRequestCounter 获取 HTTP 客户端请求计数器.
// 参数：无.
// 返回值：CounterVec - HTTP 客户端请求计数器向量.
func GetHTTPClientRequestCounter() CounterVec { return httpClientRequestsTotal }

// GetHTTPClientRequestDuration 获取 HTTP 客户端请求耗时直方图.
// 参数：无.
// 返回值：HistogramVec - HTTP 客户端请求耗时直方图向量.
func GetHTTPClientRequestDuration() HistogramVec { return httpClientRequestDuration }

// ================================
// Redis 指标
// ================================.

var (
	redisRequestsTotal           CounterVec
	redisRequestDuration         HistogramVec
	redisPipelineRequestsTotal   CounterVec
	redisPipelineRequestDuration HistogramVec
)

func init() {
	Register(func() {
		redisRequestsTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "redis_requests_total",
			Help:      "Total Redis requests (single command)",
			Labels:    []string{"datasource", "success"},
		})
		redisRequestDuration = NewHistogramVec(&HistogramVecOpts{
			Namespace: ns,
			Name:      "redis_request_duration_seconds",
			Help:      "Redis request latency in seconds (single command)",
			Labels:    []string{"datasource"},
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		})
		redisPipelineRequestsTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "redis_pipeline_requests_total",
			Help:      "Total Redis pipeline executions (one batch = one observation)",
			Labels:    []string{"datasource", "success"},
		})
		redisPipelineRequestDuration = NewHistogramVec(&HistogramVecOpts{
			Namespace: ns,
			Name:      "redis_pipeline_duration_seconds",
			Help:      "Redis pipeline batch latency in seconds",
			Labels:    []string{"datasource"},
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		})
	})
}

// GetRedisRequestCounter 获取 Redis 单命令请求计数器.
// 参数：无.
// 返回值：CounterVec - Redis 单命令请求计数器向量.
func GetRedisRequestCounter() CounterVec { return redisRequestsTotal }

// GetRedisRequestDuration 获取 Redis 单命令请求耗时直方图.
// 参数：无.
// 返回值：HistogramVec - Redis 单命令请求耗时直方图向量.
func GetRedisRequestDuration() HistogramVec { return redisRequestDuration }

// GetRedisPipelineRequestCounter 获取 Redis 管道请求计数器.
// 参数：无.
// 返回值：CounterVec - Redis 管道请求计数器向量.
func GetRedisPipelineRequestCounter() CounterVec { return redisPipelineRequestsTotal }

// GetRedisPipelineRequestDuration 获取 Redis 管道请求耗时直方图.
// 参数：无.
// 返回值：HistogramVec - Redis 管道请求耗时直方图向量.
func GetRedisPipelineRequestDuration() HistogramVec { return redisPipelineRequestDuration }

// ================================
// MySQL 指标
// ================================.

var (
	mysqlRequestsTotal   CounterVec
	mysqlRequestDuration HistogramVec
)

func init() {
	Register(func() {
		mysqlRequestsTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "mysql_requests_total",
			Help:      "Total MySQL queries",
			Labels:    []string{"datasource", "table", "operation", "success"},
		})
		mysqlRequestDuration = NewHistogramVec(&HistogramVecOpts{
			Namespace: ns,
			Name:      "mysql_request_duration_seconds",
			Help:      "MySQL query latency in seconds",
			Labels:    []string{"datasource", "table", "operation"},
			Buckets:   DefaultDurationBuckets,
		})
	})
}

// GetMySQLRequestCounter 获取 MySQL 请求计数器.
// 参数：无.
// 返回值：CounterVec - MySQL 请求计数器向量.
func GetMySQLRequestCounter() CounterVec { return mysqlRequestsTotal }

// GetMySQLRequestDuration 获取 MySQL 请求耗时直方图.
// 参数：无.
// 返回值：HistogramVec - MySQL 请求耗时直方图向量.
func GetMySQLRequestDuration() HistogramVec { return mysqlRequestDuration }

// ================================
// Pulsar 指标
// ================================.

var (
	pulsarProducerTotal    CounterVec
	pulsarProducerDuration HistogramVec
	pulsarConsumerTotal    CounterVec
	pulsarConsumerDuration HistogramVec
)

func init() {
	Register(func() {
		pulsarProducerTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "pulsar_produce_total",
			Help:      "Total Pulsar producer send operations",
			Labels:    []string{"topic", "success"},
		})
		pulsarProducerDuration = NewHistogramVec(&HistogramVecOpts{
			Namespace: ns,
			Name:      "pulsar_produce_duration_seconds",
			Help:      "Pulsar producer send latency in seconds",
			Labels:    []string{"topic"},
			Buckets:   DefaultDurationBuckets,
		})
		pulsarConsumerTotal = NewCounterVec(&CounterVecOpts{
			Namespace: ns,
			Name:      "pulsar_consume_total",
			Help:      "Total Pulsar consumer message processing",
			Labels:    []string{"topic", "result"},
		})
		pulsarConsumerDuration = NewHistogramVec(&HistogramVecOpts{
			Namespace: ns,
			Name:      "pulsar_consume_duration_seconds",
			Help:      "Pulsar consumer message processing latency in seconds",
			Labels:    []string{"topic", "result"},
			Buckets:   DefaultAsyncDurationBuckets,
		})
	})
}

// GetPulsarProducerCounter 获取 Pulsar 生产者计数器.
// 参数：无.
// 返回值：CounterVec - Pulsar 生产者计数器向量.
func GetPulsarProducerCounter() CounterVec { return pulsarProducerTotal }

// GetPulsarProducerDuration 获取 Pulsar 生产者耗时直方图.
// 参数：无.
// 返回值：HistogramVec - Pulsar 生产者耗时直方图向量.
func GetPulsarProducerDuration() HistogramVec { return pulsarProducerDuration }

// GetPulsarConsumerCounter 获取 Pulsar 消费者计数器.
// 参数：无.
// 返回值：CounterVec - Pulsar 消费者计数器向量.
func GetPulsarConsumerCounter() CounterVec { return pulsarConsumerTotal }

// GetPulsarConsumerDuration 获取 Pulsar 消费者耗时直方图.
// 参数：无.
// 返回值：HistogramVec - Pulsar 消费者耗时直方图向量.
func GetPulsarConsumerDuration() HistogramVec { return pulsarConsumerDuration }

// ================================
// 便捷函数
// ================================.

// ObserveHTTPRequest 记录 HTTP 请求指标.
// 参数：method - 请求方法, endpoint - 请求路径, status - 响应状态码, duration - 请求耗时.
// 返回值：无.
func ObserveHTTPRequest(method, endpoint string, status int, duration time.Duration) {
	if httpRequestsTotal != nil {
		httpRequestsTotal.Inc(method, endpoint, strconv.Itoa(status))
	}
	if httpRequestDuration != nil {
		httpRequestDuration.Observe(duration.Seconds(), method, endpoint)
	}
}

// ObserveCache 记录缓存请求指标.
// 参数：name - 缓存名称, level - 缓存层级, result - 访问结果.
// 返回值：无.
func ObserveCache(name, level, result string) {
	if cacheRequestsTotal != nil {
		cacheRequestsTotal.Inc(name, level, result)
	}
}

// ObserveCircuitBreakerState 设置熔断器状态指标.
// 参数：name - 熔断器名称, state - 状态值 (0=closed, 1=open, 2=half_open).
// 返回值：无.
func ObserveCircuitBreakerState(name string, state int) {
	if circuitBreakerState != nil {
		circuitBreakerState.Set(float64(state), name)
	}
}

// ObserveCircuitBreakerCall 记录熔断器调用指标.
// 参数：name - 熔断器名称, result - 调用结果.
// 返回值：无.
func ObserveCircuitBreakerCall(name, result string) {
	if circuitBreakerCallsTotal != nil {
		circuitBreakerCallsTotal.Inc(name, result)
	}
}

// ObserveAsyncQueueEnqueue 记录异步队列入队指标.
// 参数：endpoint - 任务端点, result - 入队结果.
// 返回值：无.
func ObserveAsyncQueueEnqueue(endpoint, result string) {
	if asyncQueueEnqueueTotal != nil {
		asyncQueueEnqueueTotal.Inc(endpoint, result)
	}
}

// ObserveAsyncQueueConsume 记录异步队列消费终态事件.
// 参数：endpoint - 任务端点, result - 消费终态结果, duration - handler 实际执行耗时, 未执行 handler 的路径传 0 跳过 histogram.
// 返回值：无.
func ObserveAsyncQueueConsume(endpoint, result string, duration time.Duration) {
	if asyncQueueConsumeTotal != nil {
		asyncQueueConsumeTotal.Inc(endpoint, result)
	}
	if asyncQueueHandlerDuration != nil && duration > 0 {
		asyncQueueHandlerDuration.Observe(duration.Seconds(), endpoint, result)
	}
}

// ObserveHTTPClientRequest 记录 HTTP 客户端出站请求指标.
// 参数：client - 客户端名称, method - 请求方法, endpoint - 请求路径, status - 响应状态, duration - 请求耗时.
// 返回值：无.
func ObserveHTTPClientRequest(client, method, endpoint, status string, duration time.Duration) {
	if httpClientRequestsTotal != nil {
		httpClientRequestsTotal.Inc(client, method, endpoint, status)
	}
	if httpClientRequestDuration != nil {
		httpClientRequestDuration.Observe(duration.Seconds(), client, method, endpoint)
	}
}

// ObserveMySQLQuery 记录 MySQL 查询指标.
// 参数：datasource - 数据源名称, table - 表名, operation - 操作类型, success - 是否成功, duration - 查询耗时.
// 返回值：无.
func ObserveMySQLQuery(datasource, table, operation string, success bool, duration time.Duration) {
	s := "true"
	if !success {
		s = "false"
	}
	if mysqlRequestsTotal != nil {
		mysqlRequestsTotal.Inc(datasource, table, operation, s)
	}
	if mysqlRequestDuration != nil {
		mysqlRequestDuration.Observe(duration.Seconds(), datasource, table, operation)
	}
}

// ObservePulsarProduce 记录 Pulsar 生产者发送指标.
// 参数：topic - 主题名称, success - 是否成功, duration - 发送耗时.
// 返回值：无.
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

// ObservePulsarConsume 记录 Pulsar 消费者处理指标.
// 参数：topic - 主题名称, result - 处理结果, duration - 处理耗时.
// 返回值：无.
func ObservePulsarConsume(topic, result string, duration time.Duration) {
	if pulsarConsumerTotal != nil {
		pulsarConsumerTotal.Inc(topic, result)
	}
	if pulsarConsumerDuration != nil {
		pulsarConsumerDuration.Observe(duration.Seconds(), topic, result)
	}
}

// ObserveRedis 记录 Redis 单命令请求指标.
// 参数：datasource - 数据源名称, success - 是否成功, duration - 请求耗时.
// 返回值：无.
func ObserveRedis(datasource string, success bool, duration time.Duration) {
	if redisRequestsTotal != nil {
		redisRequestsTotal.Inc(datasource, boolLabel(success))
	}
	if redisRequestDuration != nil {
		redisRequestDuration.Observe(duration.Seconds(), datasource)
	}
}

// ObserveRedisPipeline 记录 Redis 管道批量执行指标.
// 一个管道对应 N 条底层命令，无逐条命中语义，因此使用独立指标与单命令路径分开.
// 参数：datasource - 数据源名称, success - 是否成功, duration - 批量执行耗时.
// 返回值：无.
func ObserveRedisPipeline(datasource string, success bool, duration time.Duration) {
	if redisPipelineRequestsTotal != nil {
		redisPipelineRequestsTotal.Inc(datasource, boolLabel(success))
	}
	if redisPipelineRequestDuration != nil {
		redisPipelineRequestDuration.Observe(duration.Seconds(), datasource)
	}
}
