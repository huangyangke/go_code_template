// Package pulsar Apache Pulsar 客户端封装.
// 提供生产者与消费者管理、消息发送与接收、日志桥接.
package pulsar

import (
	"fmt"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"

	"github.com/huangyangke/go-aikit/log"
)

// Config Pulsar 客户端配置.
type Config struct {
	Name                    string        `yaml:"name"`
	URL                     string        `yaml:"url"`
	ConnectionTimeout       time.Duration `yaml:"connection_timeout"`
	OperationTimeout        time.Duration `yaml:"operation_timeout"`
	KeepAliveInterval       time.Duration `yaml:"keep_alive_interval"`
	MaxConnectionsPerBroker int           `yaml:"max_connections_per_broker"`
}

// Fix 填充零值字段的默认值.
func (c *Config) Fix() {
	if c.URL == "" {
		c.URL = "pulsar://localhost:6650"
	}
	if c.ConnectionTimeout <= 0 {
		c.ConnectionTimeout = 3 * time.Second
	}
	if c.OperationTimeout <= 0 {
		c.OperationTimeout = 5 * time.Second
	}
	if c.KeepAliveInterval <= 0 {
		c.KeepAliveInterval = 30 * time.Second
	}
	if c.MaxConnectionsPerBroker <= 0 {
		c.MaxConnectionsPerBroker = 1
	}
}

// Validate 校验必填字段，缺少时返回错误.
// 返回值：err - 缺少必填字段时的错误.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("pulsar: Name is required (used as Prometheus datasource label)")
	}
	return nil
}

// Client Pulsar 客户端封装，是创建生产者与消费者的入口.
type Client struct {
	client pulsar.Client
	config *Config
}

// Option Pulsar 客户端配置函数.
type Option func(*Config)

// WithURL 设置 Pulsar 服务地址.
// 参数：url - 服务 URL.
// 返回值：Option - 配置函数.
func WithURL(url string) Option {
	return func(c *Config) { c.URL = url }
}

// WithConnectionTimeout 设置连接超时时间.
// 参数：d - 超时时间.
// 返回值：Option - 配置函数.
func WithConnectionTimeout(d time.Duration) Option {
	return func(c *Config) { c.ConnectionTimeout = d }
}

// WithOperationTimeout 设置操作超时时间.
// 参数：d - 超时时间.
// 返回值：Option - 配置函数.
func WithOperationTimeout(d time.Duration) Option {
	return func(c *Config) { c.OperationTimeout = d }
}

// WithKeepAliveInterval 设置心跳保活间隔.
// 参数：d - 保活间隔.
// 返回值：Option - 配置函数.
func WithKeepAliveInterval(d time.Duration) Option {
	return func(c *Config) { c.KeepAliveInterval = d }
}

// WithMaxConnectionsPerBroker 设置每个 Broker 的最大连接数.
// 参数：n - 最大连接数.
// 返回值：Option - 配置函数.
func WithMaxConnectionsPerBroker(n int) Option {
	return func(c *Config) { c.MaxConnectionsPerBroker = n }
}

// New 创建 Pulsar 客户端，连接失败时 panic.
// 参数：c - 连接配置, opts - 配置选项.
// 返回值：*Client - Pulsar 客户端实例.
func New(c *Config, opts ...Option) *Client {
	for _, opt := range opts {
		opt(c)
	}
	c.Fix()
	if err := c.Validate(); err != nil {
		panic(err.Error())
	}

	log.Info("[Pulsar][connect_start][url=%s]", c.URL)

	logger := defaultLogger()
	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL:                     c.URL,
		ConnectionTimeout:       c.ConnectionTimeout,
		OperationTimeout:        c.OperationTimeout,
		KeepAliveInterval:       c.KeepAliveInterval,
		MaxConnectionsPerBroker: c.MaxConnectionsPerBroker,
		Logger:                  logger,
	})
	if err != nil {
		log.Error("[Pulsar][connect_error][url=%s]: %v", c.URL, err)
		panic(fmt.Sprintf("pulsar: connect error: %v", err))
	}

	log.Info("[Pulsar][connected][url=%s]", c.URL)
	return &Client{client: client, config: c}
}

// Close 关闭 Pulsar 客户端.
func (c *Client) Close() {
	c.client.Close()
	log.Info("[Pulsar][closed][url=%s]", c.config.URL)
}

// Raw 返回底层 pulsar.Client 用于高级用法.
// 返回值：pulsar.Client - 底层客户端实例.
func (c *Client) Raw() pulsar.Client {
	return c.client
}

// Name 返回配置的客户端名称，用于指标标签.
// 返回值：string - 客户端名称.
func (c *Client) Name() string {
	if c.config == nil {
		return ""
	}
	return c.config.Name
}
