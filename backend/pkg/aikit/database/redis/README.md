# redis — go-redis/v9 客户端

go-redis/v9 封装，支持 Standalone / Sentinel / Cluster 三种模式，内置 Prometheus 指标、分布式锁、扩展命令。

## 用法

```go
rdb := dbredis.New(&dbredis.Config{
    Addrs: []string{"localhost:6379"},
    Type:  "standalone",  // standalone | sentinel | cluster
    DB:    0,
})
defer rdb.Close()

// 基本操作
rdb.SetEx(ctx, "key", "value", time.Minute)
val, err := rdb.Get(ctx, "key")
rdb.Del(ctx, "key")
rdb.Exists(ctx, "key")
rdb.Incr(ctx, "counter")

// Hash
rdb.HSet(ctx, "hash", "field", "value")
rdb.HGet(ctx, "hash", "field")
rdb.HGetAll(ctx, "hash")

// Pub/Sub
rdb.Publish(ctx, "channel", "message")
```

## 分布式锁

```go
lock := rdb.NewLock(ctx, "my-lock", 30*time.Second)
ok, err := lock.TryLock()
if ok {
    defer lock.Unlock()
    // 临界区
}
lock.Refresh() // 手动续期
```

### Watchdog 自动续期

长任务场景下，锁可能在任务完成前过期。启用 watchdog 后，后台 goroutine 会以 `expire/3` 间隔自动续期，直到 `Unlock()` 或 context 取消：

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

lock := rdb.NewLock(ctx, "long-task-lock", 30*time.Second, dbredis.WithWatchdog(ctx))
ok, err := lock.TryLock()
if ok {
    defer lock.Unlock() // 自动停止 watchdog
    doLongRunningTask() // 锁会被自动续期，不会过期
}
```

## 配置

```yaml
addrs: ["localhost:6379"]
type: standalone
db: 0
pool_size: 10
max_retries: 3
key_prefix: "myapp:"
```

## 指标

| 指标名 | 类型 | Labels | Buckets |
|---|---|---|---|
| `redis_requests_total` | counter | `datasource, success` | — |
| `redis_request_duration_seconds` | histogram | `datasource, success` | `0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0` |
