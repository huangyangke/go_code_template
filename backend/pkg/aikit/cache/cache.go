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

	"github.com/example/go-template/pkg/aikit/log"
	"github.com/example/go-template/pkg/aikit/metrics"
)

// CacheType defines cache mode
type CacheType string

const (
	CacheTypeLocal  CacheType = "local"  // Only local cache
	CacheTypeRemote CacheType = "remote" // Only Redis cache
	CacheTypeBoth   CacheType = "both"   // Both local and Redis cache
)

// LocalCacheType defines local cache backend
type LocalCacheType string

const (
	LocalCacheTypeFreeCache LocalCacheType = "freecache" // Memory-bounded (e.g., 256MB)
	LocalCacheTypeTinyLFU   LocalCacheType = "tinylfu"   // Item-count-bounded (e.g., 1024 entries)
)

// ErrCacheNotFound is returned when a cached "not found" placeholder is hit.
var ErrCacheNotFound = errors.New("cache: not found")

// Config defines cache configuration
type Config struct {
	Family             string         `yaml:"family"`
	Name               string         `yaml:"name"`
	CacheType          CacheType      `yaml:"cache_type"`
	LocalType          LocalCacheType `yaml:"local_type"`
	LocalTTL           int            `yaml:"local_ttl"`                      // seconds
	RemoteTTL          int            `yaml:"remote_ttl"`                     // seconds
	NullValueTTL       int            `yaml:"null_value_ttl"`                 // seconds
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

// Global cache registry
var (
	cacheRegistry   = make(map[string]*MultiLevelCache)
	cacheRegistryMu sync.RWMutex
)

// MultiLevelCache implements a multi-level cache (L1 local + L2 Redis) backed by jetcache-go.
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

// Fix fills default values for zero/empty fields
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

// Validate checks required fields and returns an error if any are missing
func (c *Config) Validate() error {
	if c.Name == "" {
		return &ConfigError{Msg: "name required"}
	}
	if c.SyncLocal && c.CacheType != CacheTypeBoth {
		return &ConfigError{Msg: "sync_local requires cache_type=both"}
	}
	return nil
}

// New creates a new MultiLevelCache instance
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

	// trackedLocal (key map) is only needed for CacheTypeLocal where Redis SCAN is unavailable
	// to enumerate keys on Clear(). For CacheTypeBoth, Clear() uses Redis SCAN + DeleteFromLocalCache.
	if cfg.CacheType == CacheTypeLocal {
		m.localCache = newTrackedLocal(m.newLocalCache())
		opts = append(opts, jcache.WithLocal(m.localCache))
	} else if cfg.CacheType == CacheTypeBoth {
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

// newLocalCache creates the local cache backend based on LocalType.
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

// buildKey constructs the full cache key used for both local and remote.
func (m *MultiLevelCache) buildKey(key string) string {
	if m.config.Family == "" {
		return "aikit:cache:" + m.config.Name + ":" + key
	}
	return "aikit:cache:" + m.config.Family + ":" + m.config.Name + ":" + key
}

// codecEncode encodes a value through the configured codec.
// jetcache-go's Marshal has fast-path type switches for string/[]byte
// that bypass the codec, causing decode failures when reading into *interface{}.
func (m *MultiLevelCache) codecEncode(value interface{}) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return encoding.GetCodec(m.config.Codec).Marshal(value)
}

// codecDecode decodes bytes through the configured codec.
func (m *MultiLevelCache) codecDecode(data []byte, result interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return encoding.GetCodec(m.config.Codec).Unmarshal(data, result)
}

// Get retrieves a value from cache
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

// Set stores a value in cache.
// Encodes through the codec first, then stores as []byte to avoid
// jetcache-go's fast-path type switches that break Get with interface{}.
func (m *MultiLevelCache) Set(ctx context.Context, key string, value interface{}) error {
	encoded, err := m.codecEncode(value)
	if err != nil {
		return err
	}
	return m.cache.Set(ctx, m.buildKey(key), jcache.Value(encoded))
}

// GetOrLoad retrieves cache value, loads with fetchFunc if miss
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

// Delete removes a value from cache
func (m *MultiLevelCache) Delete(ctx context.Context, key string) error {
	return m.cache.Delete(ctx, m.buildKey(key))
}

// Exists checks if a key exists in cache
func (m *MultiLevelCache) Exists(ctx context.Context, key string) bool {
	return m.cache.Exists(ctx, m.buildKey(key))
}

// Inner returns the underlying jetcache-go Cache.
// Use jcache.NewT[K, V](mc.Inner()) for type-safe generic access.
func (m *MultiLevelCache) Inner() jcache.Cache { return m.cache }

// Clear removes all cache entries
func (m *MultiLevelCache) Clear(ctx context.Context) error {
	var remoteErr error
	if m.redisCmdable != nil {
		pattern := "aikit:cache:" + m.config.Family + ":" + m.config.Name + ":*"
		if m.config.Family == "" {
			pattern = "aikit:cache:" + m.config.Name + ":*"
		}
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

// Close stops cache and releases resources
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

// generateInstanceID generates a unique instance ID
func generateInstanceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// cacheName builds a cache name from family and name
func cacheName(family, name string) string {
	if family == "" {
		return name
	}
	return family + "/" + name
}

// ============================================================================
// Public API: Global Registry
// ============================================================================

// GetCache retrieves or creates a global cache instance
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

// CloseCache closes a cache instance
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

// CloseAllCaches closes all cache instances
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

// ErrFamilyRequired indicates family name is required
var ErrFamilyRequired = &ConfigError{Msg: "family name is required"}

// ConfigError is a configuration error
type ConfigError struct {
	Msg string
}

func (e *ConfigError) Error() string {
	return e.Msg
}

// Ensure metrics package is referenced (stats_handler.go uses it, but keep import valid)
var _ = metrics.ObserveCache
