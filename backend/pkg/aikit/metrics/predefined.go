package metrics

import (
	"strconv"
	"time"
)

var DefaultDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}
var DefaultAsyncDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 300.0}

// ================================
// HTTP 请求指标
// ================================

var (
	httpRequestsTotal   CounterVec
	httpRequestDuration HistogramVec
)

func init() {
	Register(func() {
		httpRequestsTotal = NewCounterVec(&CounterVecOpts{
			Name:   "http_requests_total",
			Help:   "Total HTTP requests",
			Labels: []string{"method", "endpoint", "status"},
		})
		httpRequestDuration = NewHistogramVec(&HistogramVecOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Labels:  []string{"method", "endpoint"},
			Buckets: DefaultDurationBuckets,
		})
	})
}

func GetHTTPRequestCounter() CounterVec   { return httpRequestsTotal }
func GetHTTPRequestDuration() HistogramVec { return httpRequestDuration }

// ================================
// 缓存指标
// ================================

var cacheRequestsTotal CounterVec

func init() {
	Register(func() {
		cacheRequestsTotal = NewCounterVec(&CounterVecOpts{
			Name:   "cache_requests_total",
			Help:   "Total cache requests",
			Labels: []string{"name", "level", "result"},
		})
	})
}

func GetCacheRequests() CounterVec { return cacheRequestsTotal }

// ================================
// 熔断器指标
// ================================

var (
	circuitBreakerState      GaugeVec
	circuitBreakerCallsTotal CounterVec
)

func init() {
	Register(func() {
		circuitBreakerState = NewGaugeVec(&GaugeVecOpts{
			Name:   "circuit_breaker_state",
			Help:   "Circuit breaker state (0=closed, 1=open, 2=half_open)",
			Labels: []string{"name"},
		})
		circuitBreakerCallsTotal = NewCounterVec(&CounterVecOpts{
			Name:   "circuit_breaker_calls_total",
			Help:   "Total circuit breaker calls",
			Labels: []string{"name", "result"},
		})
	})
}

func GetCircuitBreakerState() GaugeVec   { return circuitBreakerState }
func GetCircuitBreakerCalls() CounterVec { return circuitBreakerCallsTotal }

// ================================
// Async Queue 指标
// ================================

var (
	asyncQueueEnqueueTotal    CounterVec
	asyncQueueConsumeTotal    CounterVec
	asyncQueueHandlerDuration HistogramVec
)

func init() {
	Register(func() {
		asyncQueueEnqueueTotal = NewCounterVec(&CounterVecOpts{
			Name:   "async_queue_enqueue_total",
			Help:   "Total tasks enqueued (producer)",
			Labels: []string{"endpoint", "result"},
		})
		asyncQueueConsumeTotal = NewCounterVec(&CounterVecOpts{
			Name:   "async_queue_consume_total",
			Help:   "Total tasks consumed (consumer terminal state)",
			Labels: []string{"endpoint", "result"},
		})
		asyncQueueHandlerDuration = NewHistogramVec(&HistogramVecOpts{
			Name:    "async_queue_handler_duration_seconds",
			Help:    "Async queue handler execution latency in seconds",
			Labels:  []string{"endpoint", "result"},
			Buckets: DefaultAsyncDurationBuckets,
		})
	})
}

func GetAsyncQueueEnqueueCounter() CounterVec   { return asyncQueueEnqueueTotal }
func GetAsyncQueueConsumeCounter() CounterVec   { return asyncQueueConsumeTotal }
func GetAsyncQueueHandlerDuration() HistogramVec { return asyncQueueHandlerDuration }

// ================================
// HTTP Client 指标
// ================================

var (
	httpClientRequestsTotal   CounterVec
	httpClientRequestDuration HistogramVec
)

func init() {
	Register(func() {
		httpClientRequestsTotal = NewCounterVec(&CounterVecOpts{
			Name:   "http_client_requests_total",
			Help:   "Total HTTP client outgoing requests",
			Labels: []string{"client", "method", "endpoint", "status"},
		})
		httpClientRequestDuration = NewHistogramVec(&HistogramVecOpts{
			Name:    "http_client_request_duration_seconds",
			Help:    "HTTP client outgoing request latency in seconds",
			Labels:  []string{"client", "method", "endpoint"},
			Buckets: DefaultDurationBuckets,
		})
	})
}

func GetHTTPClientRequestCounter() CounterVec   { return httpClientRequestsTotal }
func GetHTTPClientRequestDuration() HistogramVec { return httpClientRequestDuration }

// ================================
// Redis 指标
// ================================

var (
	redisRequestsTotal           CounterVec
	redisRequestDuration         HistogramVec
	redisPipelineRequestsTotal   CounterVec
	redisPipelineRequestDuration HistogramVec
)

func init() {
	Register(func() {
		redisRequestsTotal = NewCounterVec(&CounterVecOpts{
			Name:   "redis_requests_total",
			Help:   "Total Redis requests (single command)",
			Labels: []string{"datasource", "success"},
		})
		redisRequestDuration = NewHistogramVec(&HistogramVecOpts{
			Name:    "redis_request_duration_seconds",
			Help:    "Redis request latency in seconds (single command)",
			Labels:  []string{"datasource"},
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		})
		redisPipelineRequestsTotal = NewCounterVec(&CounterVecOpts{
			Name:   "redis_pipeline_requests_total",
			Help:   "Total Redis pipeline executions (one batch = one observation)",
			Labels: []string{"datasource", "success"},
		})
		redisPipelineRequestDuration = NewHistogramVec(&HistogramVecOpts{
			Name:    "redis_pipeline_duration_seconds",
			Help:    "Redis pipeline batch latency in seconds",
			Labels:  []string{"datasource"},
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		})
	})
}

func GetRedisRequestCounter() CounterVec           { return redisRequestsTotal }
func GetRedisRequestDuration() HistogramVec        { return redisRequestDuration }
func GetRedisPipelineRequestCounter() CounterVec   { return redisPipelineRequestsTotal }
func GetRedisPipelineRequestDuration() HistogramVec { return redisPipelineRequestDuration }

// ================================
// MySQL 指标
// ================================

var (
	mysqlRequestsTotal   CounterVec
	mysqlRequestDuration HistogramVec
)

func init() {
	Register(func() {
		mysqlRequestsTotal = NewCounterVec(&CounterVecOpts{
			Name:   "mysql_requests_total",
			Help:   "Total MySQL queries",
			Labels: []string{"datasource", "table", "operation", "success"},
		})
		mysqlRequestDuration = NewHistogramVec(&HistogramVecOpts{
			Name:    "mysql_request_duration_seconds",
			Help:    "MySQL query latency in seconds",
			Labels:  []string{"datasource", "table", "operation"},
			Buckets: DefaultDurationBuckets,
		})
	})
}

func GetMySQLRequestCounter() CounterVec   { return mysqlRequestsTotal }
func GetMySQLRequestDuration() HistogramVec { return mysqlRequestDuration }

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

func GetPulsarProducerCounter() CounterVec    { return pulsarProducerTotal }
func GetPulsarProducerDuration() HistogramVec { return pulsarProducerDuration }
func GetPulsarConsumerCounter() CounterVec    { return pulsarConsumerTotal }
func GetPulsarConsumerDuration() HistogramVec { return pulsarConsumerDuration }

// ================================
// 便捷函数
// ================================

func ObserveHTTPRequest(method, endpoint string, status int, duration time.Duration) {
	if httpRequestsTotal != nil {
		httpRequestsTotal.Inc(method, endpoint, strconv.Itoa(status))
	}
	if httpRequestDuration != nil {
		httpRequestDuration.Observe(duration.Seconds(), method, endpoint)
	}
}

func ObserveCache(name, level, result string) {
	if cacheRequestsTotal != nil {
		cacheRequestsTotal.Inc(name, level, result)
	}
}

func ObserveCircuitBreakerState(name string, state int) {
	if circuitBreakerState != nil {
		circuitBreakerState.Set(float64(state), name)
	}
}

func ObserveCircuitBreakerCall(name, result string) {
	if circuitBreakerCallsTotal != nil {
		circuitBreakerCallsTotal.Inc(name, result)
	}
}

func ObserveAsyncQueueEnqueue(endpoint, result string) {
	if asyncQueueEnqueueTotal != nil {
		asyncQueueEnqueueTotal.Inc(endpoint, result)
	}
}

func ObserveAsyncQueueConsume(endpoint, result string, duration time.Duration) {
	if asyncQueueConsumeTotal != nil {
		asyncQueueConsumeTotal.Inc(endpoint, result)
	}
	if asyncQueueHandlerDuration != nil {
		asyncQueueHandlerDuration.Observe(duration.Seconds(), endpoint, result)
	}
}

func ObserveHTTPClientRequest(client, method, endpoint, status string, duration time.Duration) {
	if httpClientRequestsTotal != nil {
		httpClientRequestsTotal.Inc(client, method, endpoint, status)
	}
	if httpClientRequestDuration != nil {
		httpClientRequestDuration.Observe(duration.Seconds(), client, method, endpoint)
	}
}

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

func ObserveRedis(datasource string, success bool, duration time.Duration) {
	s := "1"
	if !success {
		s = "0"
	}
	if redisRequestsTotal != nil {
		redisRequestsTotal.Inc(datasource, s)
	}
	if redisRequestDuration != nil {
		redisRequestDuration.Observe(duration.Seconds(), datasource)
	}
}

// ObserveRedisPipeline records a pipeline batch execution. A batch maps to N
// underlying commands and has no per-command "hit" semantic, so it is recorded
// against separate metrics from the single-command path.
func ObserveRedisPipeline(datasource string, success bool, duration time.Duration) {
	s := "1"
	if !success {
		s = "0"
	}
	if redisPipelineRequestsTotal != nil {
		redisPipelineRequestsTotal.Inc(datasource, s)
	}
	if redisPipelineRequestDuration != nil {
		redisPipelineRequestDuration.Observe(duration.Seconds(), datasource)
	}
}
