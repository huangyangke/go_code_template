# resilience — 熔断器 + 重试

提供熔断器和重试两个弹性模式，被 httpclient 和 mysql 等模块内部使用，也可独立使用。

## 熔断器

```go
breaker := resilience.New(&resilience.Config{
    Name:                   "my-breaker",
    MaxRequests:            1,    // 半开状态允许通过的最大请求数
    Interval:               10 * time.Second, // 统计窗口（default 10s）
    RequestVolumeThreshold: 20,
    SleepWindow:            5 * time.Second,
    ErrorPercentThreshold:  50,
})

err := breaker.Do(func() error {
    return callService()
}, func(err error) error {
    return fallbackResult // 熔断打开时的降级逻辑
})
```

## 重试

```go
// 简单重试（默认 3 次）
err := resilience.Retry(func() error {
    return callService()
}, resilience.WithTimes(5))

// 指数退避重试
backoff := resilience.NewBackoff(
    resilience.WithBackoffBase(100 * time.Millisecond),
    resilience.WithBackoffMax(5 * time.Second),
    resilience.WithBackoffFactor(2.0),
    resilience.WithBackoffJitter(0.1),
)
err := resilience.RetryWithBackoff(func() error {
    return callService()
}, backoff, resilience.WithTimes(5))

// 自定义可接受错误（不重试的错误）
err := resilience.Retry(fn, resilience.WithAcceptable(func(err error) bool {
    return errors.Is(err, ErrNotFound) // NotFound 不重试
}))
```

## 配置

```yaml
name: my-breaker
max_requests: 1                   # 半开状态允许通过的最大请求数（默认 1）
interval: 10s                     # 统计窗口，0 表示每次状态改变后重置（默认 10s）
request_volume_threshold: 20      # 触发统计的最小请求量（默认 5）
sleep_window: 5s                  # 熔断恢复等待（默认 5s）
error_percent_threshold: 50       # 错误百分比阈值（默认 50）
```
