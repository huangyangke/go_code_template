// Package cache 多级缓存（本地 + Redis）.
package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	jcache "github.com/mgtv-tech/jetcache-go"
	"github.com/mgtv-tech/jetcache-go/encoding"
	"github.com/mgtv-tech/jetcache-go/local"
	"github.com/mgtv-tech/jetcache-go/remote"
	"github.com/mgtv-tech/jetcache-go/stats"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/huangyangke/go-aikit/log"
	"github.com/huangyangke/go-aikit/metrics"
)

// CacheType 缓存模式类型.
type CacheType string

const (
	// CacheTypeLocal 仅使用本地缓存.
	CacheTypeLocal CacheType = "local"
	// CacheTypeRemote 仅使用 Redis 缓存.
	CacheTypeRemote CacheType = "remote"
	// CacheTypeBoth 同时使用本地和 Redis 缓存.
	CacheTypeBoth CacheType = "both"
)

// LocalCacheType 本地缓存后端类型.
type LocalCacheType string

const (
	// LocalCacheTypeFreeCache 按内存大小限制（如 256MB）.
	LocalCacheTypeFreeCache LocalCacheType = "freecache"
	// LocalCacheTypeTinyLFU 按条目数量限制（如 1024 条）.
	LocalCacheTypeTinyLFU LocalCacheType = "tinylfu"
)

// ErrCacheNotFound 缓存命中"未找到"占位符时返回的错误.
var ErrCacheNotFound = errors.New("cache: not found")

// Config 缓存配置.
type Config struct {
	Family             string         `yaml:"family"`
	Name               string         `yaml:"name"`
	CacheType          CacheType      `yaml:"cache_type"`
	LocalType          LocalCacheType `yaml:"local_type"`
	LocalTTL           int            `yaml:"local_ttl"`      // seconds
	RemoteTTL          int            `yaml:"remote_ttl"`     // seconds
	NullValueTTL       int            `yaml:"null_value_ttl"` // seconds
	RedisCmdable       redis.Cmdable  `yaml:"-"`
	LocalMemSize       string         `yaml:"local_mem_size"`                 // FreeCache memory limit, e.g., "256MB"
	LocalItemSize      int            `yaml:"local_item_size"`                // TinyLFU max items
	RefreshDuration    int            `yaml:"refresh_duration"`               // seconds, async refresh interval (>0 enables)
	StopRefreshAfter   int            `yaml:"stop_refresh_after_last_access"` // seconds, stop refresh after idle
	RefreshConcurrency int            `yaml:"refresh_concurrency"`            // max concurrent refreshes
	Codec              string         `yaml:"codec"`                          // "msgpack"|"json"
	SourceID           string         `yaml:"-"`                              // auto-generated instance ID
	SyncLocal          bool           `yaml:"sync_local"`                     // cross-instance L1 invalidation via jetcache-go
	EventChBufSize     int            `yaml:"event_ch_buf_size"`              // SyncLocal event channel buffer size
}

// 全局缓存注册表.
var (
	cacheRegistry   = make(map[string]*MultiLevelCache)
	cacheRegistryMu sync.RWMutex
)

// MultiLevelCache 多级缓存实现（L1 本地 + L2 Redis），基于 jetcache-go.
type MultiLevelCache struct {
	config        Config
	cache         jcache.Cache
	redisCmdable  redis.Cmdable
	stopCh        chan struct{}
	closeOnce     sync.Once
	localCache    *trackedLocal
	unregTaskSize func()
}

type trackedLocal struct {
	backend local.Local
	mu      sync.Mutex
	keys    map[string]struct{}
}

func newTrackedLocal(backend local.Local) *trackedLocal {
	return &trackedLocal{
		backend: backend,
		keys:    make(map[string]struct{}),
	}
}

func (l *trackedLocal) Set(key string, data []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.keys[key] = struct{}{}
	l.backend.Set(key, data)
}

func (l *trackedLocal) Get(key string) ([]byte, bool) {
	return l.backend.Get(key)
}

func (l *trackedLocal) Del(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.backend.Del(key)
	delete(l.keys, key)
}

func (l *trackedLocal) Clear() {
	l.mu.Lock()
	for key := range l.keys {
		l.backend.Del(key)
	}
	l.keys = make(map[string]struct{})
	l.mu.Unlock()
}

// Fix 填充零值/空值字段的默认值.
func (c *Config) Fix() {
	if c.CacheType == "" {
		c.CacheType = CacheTypeBoth
	}
	if c.LocalType == "" {
		c.LocalType = LocalCacheTypeFreeCache
	}
	if c.LocalTTL <= 0 {
		c.LocalTTL = 60
	}
	if c.RemoteTTL <= 0 {
		c.RemoteTTL = 3600
	}
	if c.NullValueTTL <= 0 {
		c.NullValueTTL = 60
	}
	if c.LocalMemSize == "" {
		c.LocalMemSize = "256MB"
	}
	if c.LocalItemSize <= 0 {
		c.LocalItemSize = 1000
	}
	if c.Codec == "" {
		c.Codec = "msgpack"
	}
	if c.RefreshConcurrency <= 0 {
		c.RefreshConcurrency = 4
	}
	if c.SourceID == "" {
		c.SourceID = generateInstanceID()
	}
	if c.EventChBufSize <= 0 {
		c.EventChBufSize = 100
	}
	if c.StopRefreshAfter <= 0 && c.RefreshDuration > 0 {
		c.StopRefreshAfter = c.RefreshDuration + 1
	}
}

// Validate 校验必填字段，缺失时返回错误.
// 返回值：err - 校验失败时的错误.
func (c *Config) Validate() error {
	if c.Family == "" {
		return &ConfigError{Msg: "family required"}
	}
	if c.Name == "" {
		return &ConfigError{Msg: "name required"}
	}
	if c.SyncLocal && c.CacheType != CacheTypeBoth {
		return &ConfigError{Msg: "sync_local requires cache_type=both"}
	}
	return nil
}

// New 创建新的多级缓存实例.
// 参数：cfg - 缓存配置.
// 返回值：m - 多级缓存实例, err - 创建失败时的错误.
func New(cfg Config) (*MultiLevelCache, error) {
	cfg.Fix()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	m := &MultiLevelCache{
		config: cfg,
		stopCh: make(chan struct{}),
	}

	opts := []jcache.Option{
		jcache.WithName(cacheName(cfg.Family, cfg.Name)),
		jcache.WithCodec(cfg.Codec),
		jcache.WithRemoteExpiry(time.Duration(cfg.RemoteTTL) * time.Second),
		jcache.WithNotFoundExpiry(time.Duration(cfg.NullValueTTL) * time.Second),
		jcache.WithErrNotFound(ErrCacheNotFound),
		jcache.WithRefreshDuration(time.Duration(cfg.RefreshDuration) * time.Second),
		jcache.WithRefreshConcurrency(cfg.RefreshConcurrency),
		jcache.WithSourceId(cfg.SourceID),
		jcache.WithEventChBufSize(cfg.EventChBufSize),
		jcache.WithStatsHandler(stats.NewHandles(false,
			stats.NewStatsLogger(cacheName(cfg.Family, cfg.Name)),
			newPromStats(cfg.Family, cfg.Name),
		)),
	}

	if cfg.StopRefreshAfter > 0 {
		opts = append(opts, jcache.WithStopRefreshAfterLastAccess(
			time.Duration(cfg.StopRefreshAfter)*time.Second,
		))
	}

	// trackedLocal（key map）仅在 CacheTypeLocal 模式下需要，因为本地缓存无法用 SCAN 扫描
	// 来枚举键以执行 Clear(). 对于 CacheTypeBoth，Clear() 使用 Redis SCAN + DeleteFromLocalCache.
	switch cfg.CacheType {
	case CacheTypeLocal:
		m.localCache = newTrackedLocal(m.newLocalCache())
		opts = append(opts, jcache.WithLocal(m.localCache))
	case CacheTypeBoth:
		opts = append(opts, jcache.WithLocal(m.newLocalCache()))
	}

	if cfg.CacheType == CacheTypeRemote || cfg.CacheType == CacheTypeBoth {
		if cfg.RedisCmdable != nil {
			m.redisCmdable = cfg.RedisCmdable
			opts = append(opts, jcache.WithRemote(
				remote.NewGoRedisV9Adapter(cfg.RedisCmdable),
			))
		}
	}

	if cfg.SyncLocal && cfg.CacheType == CacheTypeBoth {
		opts = append(opts, jcache.WithSyncLocal(true))
	}

	m.cache = jcache.New(opts...)

	log.Info("[Cache][%s/%s][created][type=%s][local=%s][codec=%s]",
		cfg.Family, cfg.Name, cfg.CacheType, cfg.LocalType, cfg.Codec)

	m.unregTaskSize = metrics.RegisterGaugeFunc(
		"cache_refresh_task_size",
		"Number of in-flight async cache refresh tasks",
		prom.Labels{"name": cacheName(cfg.Family, cfg.Name)},
		func() float64 { return float64(m.cache.TaskSize()) },
	)

	return m, nil
}

// newLocalCache 根据配置中的 LocalType 创建本地缓存后端.
func (m *MultiLevelCache) newLocalCache() local.Local {
	localTTL := time.Duration(m.config.LocalTTL) * time.Second
	switch m.config.LocalType {
	case LocalCacheTypeFreeCache:
		size, err := ParseSize(m.config.LocalMemSize)
		if err != nil {
			log.Warn("[Cache][%s/%s][parse_size_error][mem=%s]: %v, using 256MB",
				m.config.Family, m.config.Name, m.config.LocalMemSize, err)
			size, _ = ParseSize("256MB")
		}
		return local.NewFreeCache(size, localTTL)
	default:
		return local.NewTinyLFU(m.config.LocalItemSize, localTTL)
	}
}

// buildKey 构建本地和远程通用的完整缓存键.
func (m *MultiLevelCache) buildKey(key string) string {
	return "aikit:cache:" + m.config.Family + ":" + m.config.Name + ":" + key
}

// codecEncode 通过配置的 Codec 编码值.
// jetcache-go 的 Marshal 对 string/[]byte 有快速路径类型分支，
// 会绕过 Codec 直接写入，导致用 *interface{} 解码时失败.
func (m *MultiLevelCache) codecEncode(value interface{}) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return encoding.GetCodec(m.config.Codec).Marshal(value)
}

// codecDecode 通过配置的 Codec 解码字节.
func (m *MultiLevelCache) codecDecode(data []byte, result interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return encoding.GetCodec(m.config.Codec).Unmarshal(data, result)
}

// Get 从缓存中获取值.
// 参数：ctx - 上下文, key - 缓存键.
// 返回值：value - 缓存值（未命中时为 nil）, err - 获取失败时的错误.
func (m *MultiLevelCache) Get(ctx context.Context, key string) (interface{}, error) {
	var raw []byte
	err := m.cache.Get(ctx, m.buildKey(key), &raw)
	if err != nil {
		if errors.Is(err, jcache.ErrCacheMiss) || errors.Is(err, ErrCacheNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	var result interface{}
	if err := m.codecDecode(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Set 将值存入缓存.
// 先经 Codec 编码再以 []byte 存储，避免 jetcache-go 的快速路径类型分支导致 Get 无法解码.
// 参数：ctx - 上下文, key - 缓存键, value - 待缓存值.
// 返回值：err - 存储或编码失败时的错误.
func (m *MultiLevelCache) Set(ctx context.Context, key string, value interface{}) error {
	encoded, err := m.codecEncode(value)
	if err != nil {
		return err
	}
	return m.cache.Set(ctx, m.buildKey(key), jcache.Value(encoded))
}

// GetOrLoad 从缓存获取值，未命中时调用 fetchFunc 加载.
// 参数：ctx - 上下文, key - 缓存键, fetchFunc - 缓存未命中时的数据加载函数.
// 返回值：value - 最终值（未命中且加载失败时为 nil）, err - 加载或解码失败时的错误.
func (m *MultiLevelCache) GetOrLoad(
	ctx context.Context,
	key string,
	fetchFunc func(context.Context) (interface{}, error),
) (interface{}, error) {
	var raw []byte
	err := m.cache.Once(ctx, m.buildKey(key),
		jcache.Value(&raw),
		jcache.Do(func(ctx context.Context) (any, error) {
			val, err := fetchFunc(ctx)
			if err != nil {
				return nil, err
			}
			return m.codecEncode(val)
		}),
		jcache.Refresh(m.config.RefreshDuration > 0),
	)
	if err != nil {
		if errors.Is(err, ErrCacheNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	var result interface{}
	if err := m.codecDecode(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Delete 从缓存中删除指定键.
// 参数：ctx - 上下文, key - 缓存键.
// 返回值：err - 删除失败时的错误.
func (m *MultiLevelCache) Delete(ctx context.Context, key string) error {
	return m.cache.Delete(ctx, m.buildKey(key))
}

// Exists 检查缓存键是否存在.
// 参数：ctx - 上下文, key - 缓存键.
// 返回值：exists - 键存在时为 true.
func (m *MultiLevelCache) Exists(ctx context.Context, key string) bool {
	return m.cache.Exists(ctx, m.buildKey(key))
}

// Inner 返回底层 jetcache-go Cache.
// 可用 jcache.NewT[K, V](mc.Inner()) 实现类型安全的泛型访问.
func (m *MultiLevelCache) Inner() jcache.Cache { return m.cache }

// Clear 清除所有缓存条目.
// 参数：ctx - 上下文.
// 返回值：err - Redis 清除操作失败时的错误.
func (m *MultiLevelCache) Clear(ctx context.Context) error {
	var remoteErr error
	if m.redisCmdable != nil {
		pattern := "aikit:cache:" + m.config.Family + ":" + m.config.Name + ":*"
		var cursor uint64
		for {
			keys, newCursor, err := m.redisCmdable.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				log.Warn("[Cache][%s/%s][clear_scan_error]: %v", m.config.Family, m.config.Name, err)
				remoteErr = err
				break
			}
			if len(keys) > 0 {
				if err := m.redisCmdable.Del(ctx, keys...).Err(); err != nil {
					log.Warn("[Cache][%s/%s][clear_delete_error][count=%d]: %v",
						m.config.Family, m.config.Name, len(keys), err)
					remoteErr = err
					break
				}
				for _, k := range keys {
					m.cache.DeleteFromLocalCache(k)
				}
			}
			if newCursor == 0 {
				break
			}
			cursor = newCursor
		}
	}

	if m.localCache != nil {
		m.localCache.Clear()
	}
	log.Info("[Cache][%s/%s][cleared]", m.config.Family, m.config.Name)
	return remoteErr
}

// Close 停止缓存并释放资源.
// 返回值：err - 关闭失败时的错误.
func (m *MultiLevelCache) Close() error {
	m.closeOnce.Do(func() {
		close(m.stopCh)
	})
	// Unregister gauge before closing cache so a concurrent Prometheus scrape
	// can't invoke TaskSize() on a closed jetcache-go instance.
	m.unregTaskSize()
	m.cache.Close()
	log.Info("[Cache][%s/%s][closed]", m.config.Family, m.config.Name)
	return nil
}

// generateInstanceID 生成唯一实例 ID.
func generateInstanceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// cacheName 由 family 和 name 组合缓存名.
func cacheName(family, name string) string {
	return family + ":" + name
}

// ============================================================================
// 全局注册表相关公开 API
// ============================================================================.

// GetCache 获取或创建全局缓存实例.
// 参数：name - 缓存名称, family - 缓存族名, cacheType - 缓存模式, localItemSize - TinyLFU 最大条目数, localTTL - 本地缓存 TTL（秒）, remoteTTL - 远程缓存 TTL（秒）, redisCmdable - Redis 客户端.
// 返回值：cache - 缓存实例, err - 创建失败时的错误.
func GetCache(
	name string,
	family string,
	cacheType CacheType,
	localItemSize int,
	localTTL int,
	remoteTTL int,
	redisCmdable redis.Cmdable,
) (*MultiLevelCache, error) {
	if family == "" {
		return nil, ErrFamilyRequired
	}

	cacheKey := family + ":" + name

	cacheRegistryMu.RLock()
	if c, ok := cacheRegistry[cacheKey]; ok {
		cacheRegistryMu.RUnlock()
		return c, nil
	}
	cacheRegistryMu.RUnlock()

	cacheRegistryMu.Lock()
	defer cacheRegistryMu.Unlock()

	if c, ok := cacheRegistry[cacheKey]; ok {
		return c, nil
	}

	cfg := Config{
		Name:          name,
		Family:        family,
		CacheType:     cacheType,
		LocalItemSize: localItemSize,
		LocalTTL:      localTTL,
		RemoteTTL:     remoteTTL,
		RedisCmdable:  redisCmdable,
	}

	c, err := New(cfg)
	if err != nil {
		return nil, err
	}

	cacheRegistry[cacheKey] = c
	return c, nil
}

// CloseCache 关闭指定缓存实例.
// 参数：name - 缓存名称, family - 缓存族名.
// 返回值：err - 关闭失败时的错误.
func CloseCache(name string, family string) error {
	if family == "" {
		return ErrFamilyRequired
	}

	cacheKey := family + ":" + name

	cacheRegistryMu.Lock()
	c, ok := cacheRegistry[cacheKey]
	if ok {
		delete(cacheRegistry, cacheKey)
	}
	cacheRegistryMu.Unlock()

	if ok {
		return c.Close()
	}
	return nil
}

// CloseAllCaches 关闭所有缓存实例.
// 返回值：err - 任一实例关闭失败时的错误.
func CloseAllCaches() error {
	cacheRegistryMu.Lock()
	cachesToClose := make([]*MultiLevelCache, 0, len(cacheRegistry))
	for _, c := range cacheRegistry {
		cachesToClose = append(cachesToClose, c)
	}
	cacheRegistry = make(map[string]*MultiLevelCache)
	cacheRegistryMu.Unlock()

	for _, c := range cachesToClose {
		_ = c.Close()
	}
	return nil
}

// ErrFamilyRequired 表示 family 名称是必填项.
var ErrFamilyRequired = &ConfigError{Msg: "family name is required"}

// ConfigError 配置错误.
type ConfigError struct {
	Msg string
}

func (e *ConfigError) Error() string {
	return e.Msg
}

// 确保 metrics 包被引用（stats_handler.go 使用了它，但保持 import 有效）.
var _ = metrics.ObserveCache
