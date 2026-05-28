# cache — 多级缓存

基于 jetcache-go 的多级缓存：本地（FreeCache/TinyLFU/LRU）+ 远端（Redis），支持自动刷新、缓存击穿防护、Prometheus 指标。

## 用法

```go
c, err := cache.New(cache.Config{
    Family:       "my-service",
    Name:         "user-cache",
    CacheType:    cache.CacheTypeBoth,  // 本地 + Redis
    LocalType:    cache.LocalCacheTypeFreeCache,
    LocalMaxSize: 100 * 1024 * 1024,    // 100MB
    LocalTTL:     60,                   // 秒
    RemoteTTL:    300,
    RedisConfig:  &dbredis.Config{Addrs: []string{"localhost:6379"}, Type: "standalone"},
})

// 基本操作
c.Set(ctx, "key", value)
val, err := c.Get(ctx, "key")
c.Delete(ctx, "key")
exists := c.Exists(ctx, "key")

// 加载模式（缓存未命中时自动回源）
val, err := c.GetOrLoad(ctx, "user:123", func() (interface{}, error) {
    return db.FindUser(ctx, 123)
})

c.Close()
```

## 缓存类型

| CacheType | 说明 |
|---|---|
| `CacheTypeLocal` | 仅本地缓存 |
| `CacheTypeRemote` | 仅 Redis |
| `CacheTypeBoth` | 本地 + Redis 二级缓存 |

## 指标

| 指标名 | 类型 | Labels |
|---|---|---|
| `cache_hits_total` | counter | `family, name, level`（l1/l2） |
| `cache_misses_total` | counter | `family, name` |
