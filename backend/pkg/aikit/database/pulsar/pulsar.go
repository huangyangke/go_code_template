package pulsar

import (
	"context"
	"fmt"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
)

// ClientConfig holds configuration for the Pulsar client.
type ClientConfig struct {
	URL                     string        `yaml:"url"`
	Timeout                 time.Duration `yaml:"timeout"`
	KeepAliveInterval       time.Duration `yaml:"keep_alive_interval"`
	MaxConnectionsPerBroker int           `yaml:"max_connections_per_broker"`
}

func (c *ClientConfig) fix() {
	if c.URL == "" {
		c.URL = "pulsar://localhost:6650"
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.KeepAliveInterval <= 0 {
		c.KeepAliveInterval = 30 * time.Second
	}
	if c.MaxConnectionsPerBroker <= 0 {
		c.MaxConnectionsPerBroker = 1
	}
}

// Client wraps pulsar.Client and is the entry point for creating producers and consumers.
type Client struct {
	client pulsar.Client
	config *ClientConfig
}

type ClientOption func(*ClientConfig)

func WithURL(url string) ClientOption {
	return func(c *ClientConfig) { c.URL = url }
}

func WithTimeout(d time.Duration) ClientOption {
	return func(c *ClientConfig) { c.Timeout = d }
}

func WithKeepAliveInterval(d time.Duration) ClientOption {
	return func(c *ClientConfig) { c.KeepAliveInterval = d }
}

func WithMaxConnectionsPerBroker(n int) ClientOption {
	return func(c *ClientConfig) { c.MaxConnectionsPerBroker = n }
}

// New creates a Pulsar client. Panics on connection error.
func New(c *ClientConfig, opts ...ClientOption) *Client {
	for _, opt := range opts {
		opt(c)
	}
	c.fix()

	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL:                     c.URL,
		ConnectionTimeout:       c.Timeout,
		OperationTimeout:        c.Timeout,
		KeepAliveInterval:       c.KeepAliveInterval,
		MaxConnectionsPerBroker: c.MaxConnectionsPerBroker,
	})
	if err != nil {
		panic(fmt.Sprintf("pulsar: connect error: %v", err))
	}

	return &Client{client: client, config: c}
}

// Close closes the underlying Pulsar client.
func (c *Client) Close() {
	c.client.Close()
}

// Raw returns the underlying pulsar.Client for advanced usage.
func (c *Client) Raw() pulsar.Client {
	return c.client
}

// ============================================================
// Producer
// ============================================================

// Producer wraps pulsar.Producer.
type Producer struct {
	producer pulsar.Producer
}

// ProducerConfig holds configuration for a Pulsar producer.
type ProducerConfig struct {
	Topic           string                 `yaml:"topic"`
	Name            string                 `yaml:"name"`
	SendTimeout     time.Duration          `yaml:"send_timeout"`
	CompressionType pulsar.CompressionType `yaml:"compression_type"`
}

func (c *ProducerConfig) fix() {
	if c.SendTimeout <= 0 {
		c.SendTimeout = 3 * time.Second
	}
}

type ProducerOption func(*ProducerConfig)

func WithProducerName(name string) ProducerOption {
	return func(c *ProducerConfig) { c.Name = name }
}

func WithSendTimeout(d time.Duration) ProducerOption {
	return func(c *ProducerConfig) { c.SendTimeout = d }
}

func WithCompressionType(ct pulsar.CompressionType) ProducerOption {
	return func(c *ProducerConfig) { c.CompressionType = ct }
}

// NewProducer creates a producer for the given topic.
func (c *Client) NewProducer(topic string, opts ...ProducerOption) (*Producer, error) {
	if topic == "" {
		return nil, fmt.Errorf("pulsar: producer topic is required")
	}
	cfg := &ProducerConfig{Topic: topic}
	for _, opt := range opts {
		opt(cfg)
	}
	cfg.fix()

	p, err := c.client.CreateProducer(pulsar.ProducerOptions{
		Topic:           cfg.Topic,
		Name:            cfg.Name,
		SendTimeout:     cfg.SendTimeout,
		CompressionType: cfg.CompressionType,
	})
	if err != nil {
		return nil, fmt.Errorf("pulsar: create producer: %w", err)
	}
	return &Producer{producer: p}, nil
}

// Send sends a raw byte payload.
func (p *Producer) Send(ctx context.Context, data []byte) error {
	_, err := p.producer.Send(ctx, &pulsar.ProducerMessage{Payload: data})
	return err
}

// SendAsync sends a raw byte payload asynchronously.
func (p *Producer) SendAsync(ctx context.Context, data []byte, callback func(pulsar.MessageID, *pulsar.ProducerMessage, error)) {
	p.producer.SendAsync(ctx, &pulsar.ProducerMessage{Payload: data}, callback)
}

// Close closes the producer.
func (p *Producer) Close() {
	p.producer.Close()
}

// ============================================================
// Consumer
// ============================================================

// Consumer wraps pulsar.Consumer.
type Consumer struct {
	consumer pulsar.Consumer
}

// ConsumerConfig holds configuration for a Pulsar consumer.
type ConsumerConfig struct {
	Topic            string                  `yaml:"topic"`
	Subscription     string                  `yaml:"subscription"`
	SubscriptionType pulsar.SubscriptionType `yaml:"subscription_type"`
}

func (c *ConsumerConfig) fix() {
	if c.Subscription == "" {
		c.Subscription = "go-aikit-sub"
	}
}

type ConsumerOption func(*ConsumerConfig)

func WithSubscription(sub string) ConsumerOption {
	return func(c *ConsumerConfig) { c.Subscription = sub }
}

func WithSubscriptionType(t pulsar.SubscriptionType) ConsumerOption {
	return func(c *ConsumerConfig) { c.SubscriptionType = t }
}

// NewConsumer creates a consumer for the given topic.
func (c *Client) NewConsumer(topic string, opts ...ConsumerOption) (*Consumer, error) {
	if topic == "" {
		return nil, fmt.Errorf("pulsar: consumer topic is required")
	}
	cfg := &ConsumerConfig{Topic: topic, SubscriptionType: pulsar.Shared}
	for _, opt := range opts {
		opt(cfg)
	}
	cfg.fix()

	cs, err := c.client.Subscribe(pulsar.ConsumerOptions{
		Topic:            cfg.Topic,
		SubscriptionName: cfg.Subscription,
		Type:             cfg.SubscriptionType,
	})
	if err != nil {
		return nil, fmt.Errorf("pulsar: subscribe: %w", err)
	}
	return &Consumer{consumer: cs}, nil
}

// Receive blocks until a message is available.
func (cs *Consumer) Receive(ctx context.Context) (pulsar.Message, error) {
	return cs.consumer.Receive(ctx)
}

// Ack acknowledges successful processing of a message.
func (cs *Consumer) Ack(msg pulsar.Message) error {
	return cs.consumer.Ack(msg)
}

// Nack signals that the message failed processing and should be redelivered.
func (cs *Consumer) Nack(msg pulsar.Message) {
	cs.consumer.NackID(msg.ID())
}

// Close closes the consumer.
func (cs *Consumer) Close() {
	cs.consumer.Close()
}
