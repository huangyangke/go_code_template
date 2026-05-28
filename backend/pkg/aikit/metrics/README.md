# metrics — Prometheus 指标

Prometheus 指标工厂，提供类型安全的 Counter/Gauge/Histogram 向量创建，以及 go-aikit 内部预定义指标。

## 自定义指标

```go
// Counter
reqCounter := metrics.NewCounterVec(&metrics.CounterVecOpts{
    Namespace: "myapp",
    Name:      "requests_total",
    Help:      "Total requests",
    Labels:    []string{"method", "path"},
})
reqCounter.Inc("GET", "/api")
reqCounter.Add(5, "POST", "/api")

// Histogram
latency := metrics.NewHistogramVec(&metrics.HistogramVecOpts{
    Namespace: "myapp",
    Name:      "request_duration_seconds",
    Help:      "Request latency",
    Labels:    []string{"method"},
    Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
})
latency.Observe(0.123, "GET")

// Gauge
conns := metrics.NewGaugeVec(&metrics.GaugeVecOpts{
    Namespace: "myapp",
    Name:      "active_connections",
    Help:      "Active connections",
    Labels:    []string{"pool"},
})
conns.Set(42, "main")
conns.Inc("main")
```

## 预定义指标

go-aikit 各模块使用的内置指标，通过 `metrics.Observe*` 函数自动记录：

| 指标名 | 模块 | Labels |
|---|---|---|
| `http_client_requests_total` | httpclient | family, client, method, endpoint, status |
| `http_client_request_duration_seconds` | httpclient | family, client, method, endpoint |
| `mysql_requests_total` | mysql | family, datasource, operation, success |
| `mysql_request_duration_seconds` | mysql | family, datasource, operation |
| `redis_requests_total` | redis | datasource, success |
| `redis_request_duration_seconds` | redis | datasource, success |
| `cache_hits_total` | cache | family, name, level |
| `cache_misses_total` | cache | family, name |
| `async_queue_enqueue_total` | async_queue | family, endpoint, result |
| `async_queue_consume_total` | async_queue | family, endpoint, result |
| `async_queue_handler_duration_seconds` | async_queue | family, endpoint, result |
| `pulsar_produce_total` | pulsar | topic, success |
| `pulsar_produce_duration_seconds` | pulsar | topic, success |
| `pulsar_consume_total` | pulsar | topic, result |
| `pulsar_consume_duration_seconds` | pulsar | topic, result |

`ServiceFamily()` 返回通过 `metrics.SetFamily()` 设置的 family 值。FastApp 在 `Run()` 时自动调用 `metrics.SetFamily(cfg.Family)`。
