package httpclient

import (
	"fmt"
	"time"
)

// Config holds HTTP client configuration.
type Config struct {
	Name           string        `yaml:"name"`
	Addr           string        `yaml:"addr"`            // Base URL, auto-prefixed to request paths
	Timeout        time.Duration `yaml:"timeout"`         // Per-request timeout, default 2s
	Breaker        *BreakerConfig `yaml:"breaker"`        // nil = disabled
	Retry          *RetryConfig   `yaml:"retry"`          // nil = disabled
	DisableMetrics bool          `yaml:"disable_metrics"`
}

// BreakerConfig holds circuit breaker configuration for HTTP client.
type BreakerConfig struct {
	Name                   string        `yaml:"name"`
	MaxRequests            uint32        `yaml:"max_requests"`
	RequestVolumeThreshold int           `yaml:"request_volume_threshold"`
	SleepWindow            time.Duration `yaml:"sleep_window"`
	ErrorPercentThreshold  int           `yaml:"error_percent_threshold"`
}

// RetryConfig holds retry configuration for HTTP client.
type RetryConfig struct {
	MaxRetries     int           `yaml:"max_retries"`      // default 3
	WaitBetween    time.Duration `yaml:"wait_between"`     // default 1s
	JitterFraction float64       `yaml:"jitter_fraction"`  // default 0.1
}

// Fix fills default values for zero/empty fields.
func (c *Config) Fix() {
	if c.Timeout <= 0 {
		c.Timeout = 2 * time.Second
	}
	if c.Breaker != nil {
		c.Breaker.Fix()
	}
	if c.Retry != nil {
		c.Retry.Fix()
	}
}

// Validate checks required fields.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("httpclient: Name is required")
	}
	return nil
}

// Fix fills default values for BreakerConfig.
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

// Fix fills default values for RetryConfig.
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
