package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	jcache "github.com/mgtv-tech/jetcache-go"
	"github.com/mgtv-tech/jetcache-go/encoding"
	"github.com/mgtv-tech/jetcache-go/local"
	"github.com/mgtv-tech/jetcache-go/remote"
	"github.com/mgtv-tech/jetcache-go/stats"

	dbredis "github.com/example/go-template/pkg/aikit/database/redis"
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
	LocalCacheTypeLRU       LocalCacheType = "lru"       // Backward compat: maps to TinyLFU
	LocalCacheTypeTTL       LocalCacheType = "ttl"       // Backward compat: maps to FreeCache
)

// ErrCacheNotFound is returned when a cached "not found" placeholder is hit.
var ErrCacheNotFound = errors.New("cache: not found")

// Config defines cache configuration
type Config struct {
	Family          string          `yaml:"family"`
	Name            string          `yaml:"name"`
	CacheType       CacheType       `yaml:"cache_type"`
	LocalType       LocalCacheType  `yaml:"local_type"`
	LocalMaxSize    int             `yaml:"local_max_size"` // LRU/TTL compat: max entries; TinyLFU: item count
	LocalTTL        int             `yaml:"local_ttl"`      // seconds
	RemoteTTL       int             `yaml:"remote_ttl"`     // seconds
	RedisConfig     *dbredis.Config `yaml:"redis_config"`
	EnableRefresh   bool            `yaml:"enable_refresh"`   // Backward compat: maps to RefreshDuration > 0
	RefreshInterval int             `yaml:"refresh_interval"` // Backward compat: maps to RefreshDuration
	NullValueTTL    int             `yaml:"null_value_ttl"`   // seconds
	TTLJitterMin    int             `yaml:"ttl_jitter_min"`   // Deprecated: jetcache-go handles jitter
	TTLJitterMax    int             `yaml:"ttl_jitter_max"`   // Deprecated: jetcache-go handles jitter
	EventChannel    string          `yaml:"event_channel"`

	// New fields
	LocalMemSize       string `yaml:"local_mem_size"`                 // FreeCache memory limit, e.g., "256MB"
	LocalItemSize      int    `yaml:"local_item_size"`                // TinyLFU max items
	RefreshDuration    int    `yaml:"refresh_duration"`               // seconds, async refresh interval (>0 enables)
	StopRefreshAfter   int    `yaml:"stop_refresh_after_last_access"` // seconds, stop refresh after idle
	RefreshConcurrency int    `yaml:"refresh_concurrency"`            // max concurrent refreshes
	Codec              string `yaml:"codec"`                          // "msgpack"|"json"
	SourceID           string `yaml:"-"`                              // auto-generated instance ID
	SyncLocal          bool   `yaml:"sync_local"`                     // cross-instance L1 invalidation
	EventChBufSize     int    `yaml:"event_ch_buf_size"`              // event channel buffer size
}

// Global cache registry
var (
	cacheRegistry   = make(map[string]*MultiLevelCache)
	cacheRegistryMu sync.RWMutex
)

// MultiLevelCache implements a multi-level cache (L1 local + L2 Redis) backed by jetcache-go.
type MultiLevelCache struct {
	config      Config
	cache       jcache.Cache
	redisClient *dbredis.Redis
	pubsub      *dbredis.PubSub
	stopCh      chan struct{}
	closeOnce   sync.Once
	instanceID  string
	localCache  *trackedLocal
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
	// Map backward-compat local types
	switch c.LocalType {
	case LocalCacheTypeLRU:
		c.LocalType = LocalCacheTypeTinyLFU
	case LocalCacheTypeTTL:
		c.LocalType = LocalCacheTypeFreeCache
	case "":
		c.LocalType = LocalCacheTypeFreeCache
	}
	if c.LocalMaxSize <= 0 {
		c.LocalMaxSize = 1000
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
	// New field defaults
	if c.LocalMemSize == "" {
		c.LocalMemSize = "256MB"
	}
	if c.LocalItemSize <= 0 {
		c.LocalItemSize = c.LocalMaxSize
	}
	if c.Codec == "" {
		c.Codec = "msgpack"
	}
	// Backward compat: EnableRefresh/RefreshInterval -> RefreshDuration
	if c.RefreshDuration <= 0 && c.RefreshInterval > 0 {
		c.RefreshDuration = c.RefreshInterval
	}
	if c.EnableRefresh && c.RefreshDuration <= 0 {
		c.RefreshDuration = c.LocalTTL / 2
		if c.RefreshDuration < 1 {
			c.RefreshDuration = 1
		}
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
	if c.EventChannel == "" && c.Family != "" {
		c.EventChannel = "aikit:cache:" + c.Family + ":" + c.Name + ":invalidate"
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
		config:     cfg,
		stopCh:     make(chan struct{}),
		instanceID: cfg.SourceID,
	}

	// Build jetcache-go options
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

	// Create local cache backend (for local or both mode)
	if cfg.CacheType == CacheTypeLocal || cfg.CacheType == CacheTypeBoth {
		m.localCache = newTrackedLocal(m.newLocalCache())
		opts = append(opts, jcache.WithLocal(m.localCache))
	}

	// Create remote cache backend (for remote or both mode)
	if cfg.CacheType == CacheTypeRemote || cfg.CacheType == CacheTypeBoth {
		remoteClient, err := m.initRedisClient()
		if err != nil {
			return nil, err
		}
		if remoteClient != nil {
			opts = append(opts, jcache.WithRemote(
				remote.NewGoRedisV9Adapter(remoteClient.Cmdable()),
			))
		}
	}

	// SyncLocal: cross-instance L1 invalidation via EventHandler
	if cfg.SyncLocal && cfg.CacheType == CacheTypeBoth {
		opts = append(opts,
			jcache.WithSyncLocal(true),
			jcache.WithEventHandler(m.handleSyncLocalEvent),
		)
	}

	m.cache = jcache.New(opts...)

	// Legacy Pub/Sub invalidation (when SyncLocal is false but EventChannel is set)
	if cfg.CacheType == CacheTypeBoth && !cfg.SyncLocal && cfg.EventChannel != "" && m.redisClient != nil {
		go m.startEventSubscription()
	}

	log.Info("[Cache][%s/%s][created][type=%s][local=%s][codec=%s]",
		cfg.Family, cfg.Name, cfg.CacheType, cfg.LocalType, cfg.Codec)
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
	case LocalCacheTypeTinyLFU:
		return local.NewTinyLFU(m.config.LocalItemSize, localTTL)
	default:
		return local.NewTinyLFU(m.config.LocalItemSize, localTTL)
	}
}

// initRedisClient initializes the Redis client for remote cache.
func (m *MultiLevelCache) initRedisClient() (*dbredis.Redis, error) {
	if m.config.RedisConfig == nil {
		return nil, nil
	}
	rdb := dbredis.New(m.config.RedisConfig)
	m.redisClient = rdb
	log.Info("[Cache][%s/%s][redis_connected]", m.config.Family, m.config.Name)
	return rdb, nil
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
		jcache.Refresh(m.config.EnableRefresh || m.config.RefreshDuration > 0),
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

// Clear removes all cache entries
func (m *MultiLevelCache) Clear(ctx context.Context) error {
	var remoteErr error
	if m.redisClient != nil {
		pattern := "aikit:cache:" + m.config.Family + ":" + m.config.Name + ":*"
		if m.config.Family == "" {
			pattern = "aikit:cache:" + m.config.Name + ":*"
		}
		var cursor uint64
		for {
			keys, newCursor, err := m.redisClient.Cmdable().Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				log.Warn("[Cache][%s/%s][clear_scan_error]: %v", m.config.Family, m.config.Name, err)
				remoteErr = err
				break
			}
			if len(keys) > 0 {
				if err := m.redisClient.Cmdable().Del(ctx, keys...).Err(); err != nil {
					log.Warn("[Cache][%s/%s][clear_delete_error][count=%d]: %v",
						m.config.Family, m.config.Name, len(keys), err)
					remoteErr = err
					break
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
	m.cache.Close()
	if m.pubsub != nil {
		_ = m.pubsub.Close()
	}
	if m.redisClient != nil {
		_ = m.redisClient.Close()
	}
	log.Info("[Cache][%s/%s][closed]", m.config.Family, m.config.Name)
	return nil
}

// handleSyncLocalEvent handles SyncLocal events from jetcache-go.
// When SyncLocal is enabled, jetcache-go sends events on Set/Delete.
// We publish these to Redis Pub/Sub for cross-instance propagation.
func (m *MultiLevelCache) handleSyncLocalEvent(event *jcache.Event) {
	if m.redisClient == nil || m.config.EventChannel == "" {
		return
	}
	data, err := json.Marshal(map[string]interface{}{
		"action":    "invalidate",
		"keys":      event.Keys,
		"source":    event.SourceID,
		"eventType": int(event.EventType),
	})
	if err != nil {
		log.Warn("[Cache][%s/%s][sync_local_marshal_error]: %v", m.config.Family, m.config.Name, err)
		return
	}
	if err := m.redisClient.Cmdable().Publish(context.Background(), m.config.EventChannel, string(data)).Err(); err != nil {
		log.Warn("[Cache][%s/%s][sync_local_publish_error]: %v", m.config.Family, m.config.Name, err)
	}
}

// startEventSubscription listens for invalidate events from Redis Pub/Sub (legacy mode).
func (m *MultiLevelCache) startEventSubscription() {
	if m.redisClient == nil {
		return
	}
	cmd := m.redisClient.Cmdable()
	pubsub := dbredis.NewPubSub(cmd, m.config.EventChannel)
	if pubsub == nil {
		return
	}
	m.pubsub = pubsub
	log.Info("[Cache][%s/%s][event_subscription][channel=%s]",
		m.config.Family, m.config.Name, m.config.EventChannel)

	ch := pubsub.Channel()
	for {
		select {
		case <-m.stopCh:
			return
		case msg, ok := <-ch:
			if !ok {
				log.Warn("[Cache][%s/%s][event_subscription_closed][channel=%s]",
					m.config.Family, m.config.Name, m.config.EventChannel)
				m.resubscribeEvents()
				return
			}
			m.handleEvent(msg.Payload)
		}
	}
}

// handleEvent handles incoming invalidate events (legacy Pub/Sub mode).
func (m *MultiLevelCache) handleEvent(data string) {
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		log.Warn("[Cache][%s/%s][event_decode_error]: %v", m.config.Family, m.config.Name, err)
		return
	}
	action, _ := event["action"].(string)
	source, _ := event["source"].(string)
	if action != "invalidate" || source == m.instanceID {
		return
	}
	keys, _ := event["keys"].([]interface{})
	if len(keys) > 0 {
		for _, k := range keys {
			key, ok := k.(string)
			if ok {
				if m.localCache != nil {
					m.localCache.Del(m.buildKey(key))
				} else {
					m.cache.DeleteFromLocalCache(m.buildKey(key))
				}
			}
		}
	} else {
		// Single key format (backward compat with old cache)
		key, ok := event["key"].(string)
		if ok {
			if m.localCache != nil {
				m.localCache.Del(m.buildKey(key))
			} else {
				m.cache.DeleteFromLocalCache(m.buildKey(key))
			}
		}
	}
}

// resubscribeEvents attempts to re-subscribe after a disconnection.
func (m *MultiLevelCache) resubscribeEvents() {
	time.Sleep(1 * time.Second)
	if m.redisClient == nil {
		return
	}
	cmd := m.redisClient.Cmdable()
	pubsub := dbredis.NewPubSub(cmd, m.config.EventChannel)
	if pubsub == nil {
		return
	}
	m.pubsub = pubsub
	log.Info("[Cache][%s/%s][event_resubscribed][channel=%s]",
		m.config.Family, m.config.Name, m.config.EventChannel)

	ch := pubsub.Channel()
	for {
		select {
		case <-m.stopCh:
			return
		case msg, ok := <-ch:
			if !ok {
				m.resubscribeEvents()
				return
			}
			m.handleEvent(msg.Payload)
		}
	}
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
	return family + ":" + name
}

// ============================================================================
// Public API: Global Registry
// ============================================================================

// GetCache retrieves or creates a global cache instance
func GetCache(
	name string,
	family string,
	cacheType CacheType,
	localMaxSize int,
	localTTL int,
	remoteTTL int,
	redisConfig *dbredis.Config,
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
		Name:         name,
		Family:       family,
		CacheType:    cacheType,
		LocalMaxSize: localMaxSize,
		LocalTTL:     localTTL,
		RemoteTTL:    remoteTTL,
		RedisConfig:  redisConfig,
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
var _ = metrics.ObserveCacheHit
