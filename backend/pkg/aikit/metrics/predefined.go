package metrics

import (
	"strconv"
	"sync/atomic"
	"time"
)

var serviceFamily atomic.Value

// SetFamily sets the global service family label used by all predefined metrics.
func SetFamily(family string) {
	serviceFamily.Store(family)
}

// ServiceFamily returns the service family label.
func ServiceFamily() string {
	if v := serviceFamily.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// Metric names match Python aikit's flat naming convention (no namespace/subsystem prefix).

// Predefined HTTP request duration buckets
var DefaultDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}

// Predefined async queue handler duration buckets (longer for async tasks)
var DefaultAsyncDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 300.0}

// ================================
// HTTP 请求指标
// ================================

var (
	// httpRequestsTotal 统计 HTTP 请求总数
	httpRequestsTotal = NewCounterVec(&CounterVecOpts{
		Name:   "http_requests_total",
		Help:   "Total HTTP requests",
		Labels: []string{"family", "method", "endpoint", "status"},
	})

	// httpRequestDuration 统计 HTTP 请求延迟
	httpRequestDuration = NewHistogramVec(&HistogramVecOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds",
		Labels:  []string{"family", "method", "endpoint"},
		Buckets: DefaultDurationBuckets,
	})
)

// GetHTTPRequestCounter 获取 HTTP 请求计数指标
func GetHTTPRequestCounter() CounterVec {
	return httpRequestsTotal
}

// GetHTTPRequestDuration 获取 HTTP 请求延迟指标
func GetHTTPRequestDuration() HistogramVec {
	return httpRequestDuration
}

// ================================
// 缓存指标
// ================================

var (
	// cacheHitsTotal 统计缓存命中次数
	cacheHitsTotal = NewCounterVec(&CounterVecOpts{
		Name:   "cache_hits_total",
		Help:   "Total cache hits",
		Labels: []string{"family", "name", "level"}, // level: l1 / l2
	})

	// cacheMissesTotal 统计缓存未命中次数
	cacheMissesTotal = NewCounterVec(&CounterVecOpts{
		Name:   "cache_misses_total",
		Help:   "Total cache misses",
		Labels: []string{"family", "name"},
	})
)

// GetCacheHits 获取缓存命中指标
func GetCacheHits() CounterVec {
	return cacheHitsTotal
}

// GetCacheMisses 获取缓存未命中指标
func GetCacheMisses() CounterVec {
	return cacheMissesTotal
}

// ================================
// 熔断器指标
// ================================

var (
	// circuitBreakerState 统计熔断器状态
	circuitBreakerState = NewGaugeVec(&GaugeVecOpts{
		Name:   "circuit_breaker_state",
		Help:   "Circuit breaker state (0=closed, 1=open, 2=half_open)",
		Labels: []string{"family", "name"},
	})

	// circuitBreakerCallsTotal 统计熔断器调用次数
	circuitBreakerCallsTotal = NewCounterVec(&CounterVecOpts{
		Name:   "circuit_breaker_calls_total",
		Help:   "Total circuit breaker calls",
		Labels: []string{"family", "name", "result"}, // result: success / failure / rejected
	})
)

// GetCircuitBreakerState 获取熔断器状态指标
func GetCircuitBreakerState() GaugeVec {
	return circuitBreakerState
}

// GetCircuitBreakerCalls 获取熔断器调用计数指标
func GetCircuitBreakerCalls() CounterVec {
	return circuitBreakerCallsTotal
}

// ================================
// Async Queue 指标
// ================================

var (
	// asyncQueueEnqueueTotal 统计任务入队总数
	asyncQueueEnqueueTotal = NewCounterVec(&CounterVecOpts{
		Name:   "async_queue_enqueue_total",
		Help:   "Total tasks enqueued (producer)",
		Labels: []string{"family", "endpoint", "result"}, // result: success / failure / duplicate
	})

	// asyncQueueConsumeTotal 统计任务消费总数
	asyncQueueConsumeTotal = NewCounterVec(&CounterVecOpts{
		Name:   "async_queue_consume_total",
		Help:   "Total tasks consumed (consumer terminal state)",
		Labels: []string{"family", "endpoint", "result"}, // result: success / failure / timeout / cancelled
	})

	// asyncQueueHandlerDuration 统计任务处理延迟
	asyncQueueHandlerDuration = NewHistogramVec(&HistogramVecOpts{
		Name:    "async_queue_handler_duration_seconds",
		Help:    "Async queue handler execution latency in seconds",
		Labels:  []string{"family", "endpoint", "result"},
		Buckets: DefaultAsyncDurationBuckets,
	})
)

// GetAsyncQueueEnqueueCounter 获取任务入队计数指标
func GetAsyncQueueEnqueueCounter() CounterVec {
	return asyncQueueEnqueueTotal
}

// GetAsyncQueueConsumeCounter 获取任务消费计数指标
func GetAsyncQueueConsumeCounter() CounterVec {
	return asyncQueueConsumeTotal
}

// GetAsyncQueueHandlerDuration 获取任务处理延迟指标
func GetAsyncQueueHandlerDuration() HistogramVec {
	return asyncQueueHandlerDuration
}

// ================================
// HTTP Client 指标
// ================================

var (
	// httpClientRequestsTotal 统计 HTTP 客户端出站请求总数
	httpClientRequestsTotal = NewCounterVec(&CounterVecOpts{
		Name:   "http_client_requests_total",
		Help:   "Total HTTP client outgoing requests",
		Labels: []string{"family", "client", "method", "endpoint", "status"},
	})

	// httpClientRequestDuration 统计 HTTP 客户端出站请求延迟
	httpClientRequestDuration = NewHistogramVec(&HistogramVecOpts{
		Name:    "http_client_request_duration_seconds",
		Help:    "HTTP client outgoing request latency in seconds",
		Labels:  []string{"family", "client", "method", "endpoint"},
		Buckets: DefaultDurationBuckets,
	})
)

// GetHTTPClientRequestCounter 获取 HTTP 客户端请求计数指标
func GetHTTPClientRequestCounter() CounterVec {
	return httpClientRequestsTotal
}

// GetHTTPClientRequestDuration 获取 HTTP 客户端请求延迟指标
func GetHTTPClientRequestDuration() HistogramVec {
	return httpClientRequestDuration
}

// ================================
// MySQL 指标
// ================================

var (
	// mysqlRequestsTotal 统计 MySQL 查询总数
	mysqlRequestsTotal = NewCounterVec(&CounterVecOpts{
		Name:   "mysql_requests_total",
		Help:   "Total MySQL queries",
		Labels: []string{"family", "datasource", "operation", "success"},
	})

	// mysqlRequestDuration 统计 MySQL 查询延迟
	mysqlRequestDuration = NewHistogramVec(&HistogramVecOpts{
		Name:    "mysql_request_duration_seconds",
		Help:    "MySQL query latency in seconds",
		Labels:  []string{"family", "datasource", "operation"},
		Buckets: DefaultDurationBuckets,
	})
)

// GetMySQLRequestCounter 获取 MySQL 请求计数指标
func GetMySQLRequestCounter() CounterVec {
	return mysqlRequestsTotal
}

// GetMySQLRequestDuration 获取 MySQL 请求延迟指标
func GetMySQLRequestDuration() HistogramVec {
	return mysqlRequestDuration
}

// ================================
// 便捷函数
// ================================

// ObserveHTTPRequest 记录 HTTP 请求
func ObserveHTTPRequest(family, method, endpoint string, status int, duration time.Duration) {
	if httpRequestsTotal != nil {
		httpRequestsTotal.Inc(family, method, endpoint, strconv.Itoa(status))
	}
	if httpRequestDuration != nil {
		httpRequestDuration.Observe(duration.Seconds(), family, method, endpoint)
	}
}

// ObserveCacheHit 记录缓存命中
func ObserveCacheHit(family, name, level string) {
	if cacheHitsTotal != nil {
		cacheHitsTotal.Inc(family, name, level)
	}
}

// ObserveCacheMiss 记录缓存未命中
func ObserveCacheMiss(family, name string) {
	if cacheMissesTotal != nil {
		cacheMissesTotal.Inc(family, name)
	}
}

// ObserveCircuitBreakerState 记录熔断器状态变化
func ObserveCircuitBreakerState(family, name string, state int) {
	if circuitBreakerState != nil {
		circuitBreakerState.Set(float64(state), family, name)
	}
}

// ObserveCircuitBreakerCall 记录熔断器调用
func ObserveCircuitBreakerCall(family, name, result string) {
	if circuitBreakerCallsTotal != nil {
		circuitBreakerCallsTotal.Inc(family, name, result)
	}
}

// ObserveAsyncQueueEnqueue 记录任务入队
func ObserveAsyncQueueEnqueue(family, endpoint, result string) {
	if asyncQueueEnqueueTotal != nil {
		asyncQueueEnqueueTotal.Inc(family, endpoint, result)
	}
}

// ObserveAsyncQueueConsume 记录任务消费
func ObserveAsyncQueueConsume(family, endpoint, result string, duration time.Duration) {
	if asyncQueueConsumeTotal != nil {
		asyncQueueConsumeTotal.Inc(family, endpoint, result)
	}
	if asyncQueueHandlerDuration != nil {
		asyncQueueHandlerDuration.Observe(duration.Seconds(), family, endpoint, result)
	}
}

// ObserveHTTPClientRequest 记录 HTTP 客户端出站请求
func ObserveHTTPClientRequest(family, client, method, endpoint, status string, duration time.Duration) {
	if httpClientRequestsTotal != nil {
		httpClientRequestsTotal.Inc(family, client, method, endpoint, status)
	}
	if httpClientRequestDuration != nil {
		httpClientRequestDuration.Observe(duration.Seconds(), family, client, method, endpoint)
	}
}

// ObserveMySQLQuery 记录 MySQL 查询
func ObserveMySQLQuery(family, datasource, operation string, success bool, duration time.Duration) {
	s := "true"
	if !success {
		s = "false"
	}
	if mysqlRequestsTotal != nil {
		mysqlRequestsTotal.Inc(family, datasource, operation, s)
	}
	if mysqlRequestDuration != nil {
		mysqlRequestDuration.Observe(duration.Seconds(), family, datasource, operation)
	}
}
