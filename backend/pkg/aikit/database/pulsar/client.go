package pulsar

import (
	"fmt"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	pulsarlog "github.com/apache/pulsar-client-go/pulsar/log"

	"github.com/example/go-template/pkg/aikit/log"
)

// Config holds configuration for the Pulsar client.
type Config struct {
	Name                    string        `yaml:"name"`
	URL                     string        `yaml:"url"`
	ConnectionTimeout       time.Duration `yaml:"connection_timeout"`
	OperationTimeout        time.Duration `yaml:"operation_timeout"`
	KeepAliveInterval       time.Duration `yaml:"keep_alive_interval"`
	MaxConnectionsPerBroker int           `yaml:"max_connections_per_broker"`
}

// Fix fills default values for zero/empty fields.
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

// Validate checks required fields and returns an error if any are missing.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("pulsar: Name is required (used as Prometheus datasource label)")
	}
	return nil
}


// Client wraps pulsar.Client and is the entry point for creating producers and consumers.
type Client struct {
	client pulsar.Client
	config *Config
}

// Option configures a Config.
type Option func(*Config)

func WithURL(url string) Option {
	return func(c *Config) { c.URL = url }
}

func WithConnectionTimeout(d time.Duration) Option {
	return func(c *Config) { c.ConnectionTimeout = d }
}

func WithOperationTimeout(d time.Duration) Option {
	return func(c *Config) { c.OperationTimeout = d }
}

func WithKeepAliveInterval(d time.Duration) Option {
	return func(c *Config) { c.KeepAliveInterval = d }
}

func WithMaxConnectionsPerBroker(n int) Option {
	return func(c *Config) { c.MaxConnectionsPerBroker = n }
}

// New creates a Pulsar client. Panics on connection error.
func New(c *Config, opts ...Option) *Client {
	for _, opt := range opts {
		opt(c)
	}
	c.Fix()
	if err := c.Validate(); err != nil {
		panic(err.Error())
	}

	log.Info("[Pulsar][connect_start][url=%s]", c.URL)

	var logger pulsarlog.Logger = defaultLogger()
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

// Close closes the underlying Pulsar client.
func (c *Client) Close() {
	c.client.Close()
	log.Info("[Pulsar][closed][url=%s]", c.config.URL)
}

// Raw returns the underlying pulsar.Client for advanced usage.
func (c *Client) Raw() pulsar.Client {
	return c.client
}

// Name returns the configured client name (for metrics labels).
func (c *Client) Name() string {
	if c.config == nil {
		return ""
	}
	return c.config.Name
}
