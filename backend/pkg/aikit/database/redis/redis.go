package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/go-template/pkg/aikit/log"
)

const (
	Nil            = redis.Nil
	ClusterType    = "cluster"
	SentinelType   = "sentinel"
	StandaloneType = "standalone"
)

type (
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
		DisableKeyPrefix bool          `yaml:"disable_key_prefix"`
		hooks            []redis.Hook  `yaml:"-"` // Redis command hooks
		logger           logger        `yaml:"-"` // Custom logger
	}

	Redis struct {
		config *Config
		client redis.Cmdable
	}

	Option func(c *Config)

	logger interface {
		Printf(ctx context.Context, format string, v ...interface{})
	}
)

func WithHook(hook ...redis.Hook) Option {
	return func(c *Config) {
		c.hooks = append(c.hooks, hook...)
	}
}

func WithLogger(l logger) Option {
	return func(c *Config) {
		c.logger = l
	}
}

func New(c *Config, opts ...Option) *Redis {
	for _, opt := range opts {
		opt(c)
	}
	c.Fix()
	if err := c.Validate(); err != nil {
		panic(err.Error())
	}
	c.hooks = append([]redis.Hook{NewPrometheusHook(c.Name)}, c.hooks...)
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
		panic(fmt.Sprintf("redis type must be one of (cluster, sentinel, standalone), got: %s", c.Type))
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.PingTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Error("[Redis][connect_error][type=%s][addrs=%v]: %v", c.Type, c.Addrs, err)
		panic(fmt.Sprintf("redis connect error: %v", err))
	}

	log.Info("[Redis][connected][type=%s][addrs=%v]", c.Type, c.Addrs)
	return &Redis{config: c, client: client}
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


// DefaultConfig returns a Config with sensible defaults (sharing them with Fix()).
func DefaultConfig() Config {
	var c Config
	c.Fix()
	return c
}

// Fix fills default values for zero/empty fields
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

// Validate checks required fields and returns an error if any are missing
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("redis: Name is required (used as Prometheus datasource label)")
	}
	if len(c.Addrs) < 1 {
		return fmt.Errorf("redis: no addresses in config")
	}
	if c.Type == SentinelType && c.MasterName == "" {
		return fmt.Errorf("redis: sentinel type requires MasterName")
	}
	return nil
}

// Cmdable returns the underlying redis.Cmdable for generic usage.
func (r *Redis) Cmdable() redis.Cmdable {
	return r.client
}

// Cluster returns the cluster client (nil if not cluster mode).
func (r *Redis) Cluster() *redis.ClusterClient {
	c, _ := r.client.(*redis.ClusterClient)
	return c
}

// Sentinel returns the failover client (nil if not sentinel mode).
func (r *Redis) Sentinel() *redis.Client {
	if r.config.Type == SentinelType {
		c, _ := r.client.(*redis.Client)
		return c
	}
	return nil
}

// Standalone returns the standalone client (nil if not standalone mode).
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
