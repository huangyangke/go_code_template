// Package redis Redis 客户端封装，支持 Cluster/Sentinel/Standalone 模式.
// 提供键前缀拼接、熔断与指标 Hook、分布式锁等能力.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/huangyangke/go-aikit/log"
)

// Nil Redis 返回的空值错误.
const Nil = redis.Nil

// ClusterType 集群模式标识.
const ClusterType = "cluster"

// SentinelType 哨兵模式标识.
const SentinelType = "sentinel"

// StandaloneType 单节点模式标识.
const StandaloneType = "standalone"

type (
	// Config Redis 连接配置.
	Config struct {
		Name             string        `yaml:"name"`
		KeyPrefix        string        `yaml:"key_prefix"`
		Addrs            []string      `yaml:"addrs"`
		Type             string        `yaml:"type"` // cluster | sentinel | standalone
		MasterName       string        `yaml:"master_name"`
		PoolSize         int           `yaml:"pool_size"`
		MaxRetries       int           `yaml:"max_retries"`
		MinIdleConns     int           `yaml:"min_idle_conns"`
		DialTimeout      time.Duration `yaml:"dial_timeout"`
		ReadTimeout      time.Duration `yaml:"read_timeout"`
		WriteTimeout     time.Duration `yaml:"write_timeout"`
		IdleTimeout      time.Duration `yaml:"idle_timeout"`
		PingTimeout      time.Duration `yaml:"ping_timeout"`
		ReadOnly         bool          `yaml:"read_only"`
		RouteByLatency   bool          `yaml:"route_by_latency"`
		RouteRandomly    bool          `yaml:"route_randomly"`
		UserName         string        `yaml:"username"`
		UserPassword     string        `yaml:"password"`
		SentinelUserName string        `yaml:"sentinel_username"`
		SentinelPassword string        `yaml:"sentinel_password"`
		DB               int           `yaml:"db"`
		// DisableKeyPrefix 完全跳过 KeyPrefix 拼接；仅在调用方已自行管理 key 命名空间时使用。
		DisableKeyPrefix bool `yaml:"disable_key_prefix"`
		// EnableMetrics 挂载 Prometheus 指标 Hook；零值=关。裸客户端默认不采集，
		// FastApp 注册时自动置 true（与 MySQL 一致）。启用时 Name 必填。
		EnableMetrics bool         `yaml:"enable_metrics"`
		hooks         []redis.Hook `yaml:"-"` // Redis command hooks
		logger        logger       `yaml:"-"` // Custom logger
	}

	// Redis Redis 客户端，持有配置与底层 Cmdable 实例.
	Redis struct {
		config *Config
		client redis.Cmdable
	}

	// Option Redis 配置函数.
	Option func(c *Config)

	logger interface {
		Printf(ctx context.Context, format string, v ...interface{})
	}
)

// WithHook 添加 Redis 命令 Hook.
// 参数：hook - Hook 实例列表.
// 返回值：Option - 配置函数.
func WithHook(hook ...redis.Hook) Option {
	return func(c *Config) {
		c.hooks = append(c.hooks, hook...)
	}
}

// WithLogger 设置自定义日志器.
// 参数：l - 日志接口.
// 返回值：Option - 配置函数.
func WithLogger(l logger) Option {
	return func(c *Config) {
		c.logger = l
	}
}

// New 创建 Redis 客户端，连接失败时 panic.
// 参数：c - 连接配置, opts - 配置选项.
// 返回值：*Redis - Redis 客户端实例.
func New(c *Config, opts ...Option) (*Redis, error) {
	for _, opt := range opts {
		opt(c)
	}
	c.Fix()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	// Prometheus 指标 Hook：opt-in（裸客户端默认关，FastApp 注册时启用）.
	if c.EnableMetrics {
		c.hooks = append([]redis.Hook{NewPrometheusHook(c.Name)}, c.hooks...)
	}
	log.Info("[Redis][connect_start][type=%s][addrs=%v][db=%d]", c.Type, c.Addrs, c.DB)

	if c.logger != nil {
		redis.SetLogger(c.logger)
	}

	var client redis.Cmdable
	switch c.Type {
	case ClusterType:
		client = c.newClusterClient()
	case SentinelType:
		client = c.newSentinelClient()
	case StandaloneType:
		client = c.newStandaloneClient()
	default:
		return nil, fmt.Errorf("redis type must be one of (cluster, sentinel, standalone), got: %s", c.Type)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.PingTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Error("[Redis][connect_error][type=%s][addrs=%v]: %v", c.Type, c.Addrs, err)
		return nil, fmt.Errorf("redis connect error: %w", err)
	}

	log.Info("[Redis][connected][type=%s][addrs=%v]", c.Type, c.Addrs)
	return &Redis{config: c, client: client}, nil
}

// MustNew 与 New 相同，但发生错误时 panic.
func MustNew(c *Config, opts ...Option) *Redis {
	r, err := New(c, opts...)
	if err != nil {
		panic(err.Error())
	}
	return r
}

func (c *Config) newClusterClient() *redis.ClusterClient {
	cl := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:           c.Addrs,
		ReadOnly:        c.ReadOnly,
		RouteByLatency:  c.RouteByLatency,
		RouteRandomly:   c.RouteRandomly,
		MaxRetries:      c.MaxRetries,
		DialTimeout:     c.DialTimeout,
		ReadTimeout:     c.ReadTimeout,
		WriteTimeout:    c.WriteTimeout,
		PoolSize:        c.PoolSize,
		MinIdleConns:    c.MinIdleConns,
		ConnMaxIdleTime: c.IdleTimeout,
		Username:        c.UserName,
		Password:        c.UserPassword,
	})
	for _, hook := range c.hooks {
		cl.AddHook(hook)
	}
	return cl
}

func (c *Config) newSentinelClient() *redis.Client {
	cl := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:       c.MasterName,
		SentinelAddrs:    c.Addrs,
		MaxRetries:       c.MaxRetries,
		DialTimeout:      c.DialTimeout,
		ReadTimeout:      c.ReadTimeout,
		WriteTimeout:     c.WriteTimeout,
		PoolSize:         c.PoolSize,
		MinIdleConns:     c.MinIdleConns,
		ConnMaxIdleTime:  c.IdleTimeout,
		SentinelUsername: c.SentinelUserName,
		SentinelPassword: c.SentinelPassword,
		Username:         c.UserName,
		Password:         c.UserPassword,
	})
	for _, hook := range c.hooks {
		cl.AddHook(hook)
	}
	return cl
}

func (c *Config) newStandaloneClient() *redis.Client {
	cl := redis.NewClient(&redis.Options{
		Addr:            c.Addrs[0],
		MaxRetries:      c.MaxRetries,
		DialTimeout:     c.DialTimeout,
		ReadTimeout:     c.ReadTimeout,
		WriteTimeout:    c.WriteTimeout,
		PoolSize:        c.PoolSize,
		MinIdleConns:    c.MinIdleConns,
		ConnMaxIdleTime: c.IdleTimeout,
		Username:        c.UserName,
		Password:        c.UserPassword,
		DB:              c.DB,
	})
	for _, hook := range c.hooks {
		cl.AddHook(hook)
	}
	return cl
}

// DefaultConfig 返回带默认值的配置，等价于先零值再 Fix.
// 返回值：Config - 默认配置.
func DefaultConfig() Config {
	var c Config
	c.Fix()
	return c
}

// Fix 填充零值字段的默认值.
func (c *Config) Fix() {
	if c.Type == "" {
		c.Type = ClusterType
	}
	if c.PoolSize <= 0 {
		c.PoolSize = 16
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
	if c.MinIdleConns <= 0 {
		c.MinIdleConns = 4
	}
	if c.DialTimeout <= 0 {
		c.DialTimeout = 3 * time.Second
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = time.Second
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = time.Minute
	}
	if c.PingTimeout <= 0 {
		c.PingTimeout = 3 * time.Second
	}
}

// Validate 校验必填字段，缺少时返回错误.
// 返回值：err - 缺少必填字段时的错误.
func (c *Config) Validate() error {
	if c.EnableMetrics && c.Name == "" {
		return fmt.Errorf("redis: Name is required when EnableMetrics is true (used as Prometheus datasource label)")
	}
	if len(c.Addrs) < 1 {
		return fmt.Errorf("redis: no addresses in config")
	}
	if c.Type == SentinelType && c.MasterName == "" {
		return fmt.Errorf("redis: sentinel type requires MasterName")
	}
	return nil
}

// Cmdable 返回底层 redis.Cmdable 用于通用操作.
// 返回值：redis.Cmdable - 底层命令接口.
func (r *Redis) Cmdable() redis.Cmdable {
	return r.client
}

// Cluster 返回集群客户端，非集群模式时返回 nil.
// 返回值：*redis.ClusterClient - 集群客户端.
func (r *Redis) Cluster() *redis.ClusterClient {
	c, _ := r.client.(*redis.ClusterClient)
	return c
}

// Sentinel 返回哨兵模式的客户端，非哨兵模式时返回 nil.
// 返回值：*redis.Client - 哨兵客户端.
func (r *Redis) Sentinel() *redis.Client {
	if r.config.Type == SentinelType {
		c, _ := r.client.(*redis.Client)
		return c
	}
	return nil
}

// Standalone 返回单节点模式的客户端，非单节点模式时返回 nil.
// 返回值：*redis.Client - 单节点客户端.
func (r *Redis) Standalone() *redis.Client {
	if r.config.Type == StandaloneType {
		c, _ := r.client.(*redis.Client)
		return c
	}
	return nil
}

func (r *Redis) key(key string) string {
	if r.config.KeyPrefix != "" && !r.config.DisableKeyPrefix {
		return fmt.Sprintf("%s:%s", r.config.KeyPrefix, key)
	}
	return key
}

// Key 返回带前缀拼接后的完整键名.
// 参数：key - 原始键名.
// 返回值：string - 拼接后的完整键名.
func (r *Redis) Key(key string) string {
	return r.key(key)
}

func (r *Redis) keys(keys ...string) []string {
	ret := make([]string, len(keys))
	for i, k := range keys {
		ret[i] = r.key(k)
	}
	return ret
}

// Close 关闭 Redis 连接.
// 返回值：err - 关闭失败时的错误.
func (r *Redis) Close() error {
	switch c := r.client.(type) {
	case *redis.ClusterClient:
		err := c.Close()
		if err != nil {
			log.Error("[Redis][close_error][type=%s][addrs=%v]: %v", r.config.Type, r.config.Addrs, err)
			return err
		}
		log.Info("[Redis][closed][type=%s][addrs=%v]", r.config.Type, r.config.Addrs)
		return nil
	case *redis.Client:
		err := c.Close()
		if err != nil {
			log.Error("[Redis][close_error][type=%s][addrs=%v]: %v", r.config.Type, r.config.Addrs, err)
			return err
		}
		log.Info("[Redis][closed][type=%s][addrs=%v]", r.config.Type, r.config.Addrs)
		return nil
	}
	log.Warn("[Redis][close_skip][reason=unknown_client]")
	return nil
}
