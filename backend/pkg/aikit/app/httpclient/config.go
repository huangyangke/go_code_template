package httpclient

import (
	"fmt"
	"time"
)

// Config HTTP 客户端配置.
type Config struct {
	Name           string         `yaml:"name"`
	Addr           string         `yaml:"addr"`    // Base URL, auto-prefixed to request paths
	Timeout        time.Duration  `yaml:"timeout"` // Per-request timeout, default 30s
	Breaker        *BreakerConfig `yaml:"breaker"` // nil = disabled
	Retry          *RetryConfig   `yaml:"retry"`   // nil = disabled
	DisableMetrics bool           `yaml:"disable_metrics"`
}

// BreakerConfig 熔断器配置.
type BreakerConfig struct {
	Name                   string        `yaml:"name"`
	MaxRequests            uint32        `yaml:"max_requests"`
	RequestVolumeThreshold int           `yaml:"request_volume_threshold"`
	SleepWindow            time.Duration `yaml:"sleep_window"`
	ErrorPercentThreshold  int           `yaml:"error_percent_threshold"`
}

// RetryConfig 重试配置.
type RetryConfig struct {
	MaxRetries     int           `yaml:"max_retries"`     // default 3
	WaitBetween    time.Duration `yaml:"wait_between"`    // default 1s
	JitterFraction float64       `yaml:"jitter_fraction"` // default 0.1
}

// Fix 填充零值字段的默认值.
func (c *Config) Fix() {
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.Breaker != nil {
		c.Breaker.Fix()
	}
	if c.Retry != nil {
		c.Retry.Fix()
	}
}

// Validate 校验必填字段.
// 返回值：err - 校验失败时的错误.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("httpclient: Name is required")
	}
	return nil
}

// Fix 填充熔断器零值字段的默认值.
func (c *BreakerConfig) Fix() {
	if c.Name == "" {
		c.Name = "httpclient"
	}
	if c.MaxRequests <= 0 {
		c.MaxRequests = 1
	}
	if c.RequestVolumeThreshold <= 0 {
		c.RequestVolumeThreshold = 20
	}
	if c.SleepWindow <= 0 {
		c.SleepWindow = 5 * time.Second
	}
	if c.ErrorPercentThreshold <= 0 {
		c.ErrorPercentThreshold = 50
	}
}

// Fix 填充重试零值字段的默认值.
func (c *RetryConfig) Fix() {
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
	if c.WaitBetween <= 0 {
		c.WaitBetween = 1 * time.Second
	}
	if c.JitterFraction <= 0 {
		c.JitterFraction = 0.1
	}
}
