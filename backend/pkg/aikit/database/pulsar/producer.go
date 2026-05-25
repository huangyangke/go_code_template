package pulsar

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
)

// Producer wraps pulsar.Producer.
type Producer struct {
	producer pulsar.Producer
	topic    string
}

// ProducerConfig holds configuration for a Pulsar producer.
type ProducerConfig struct {
	Topic                   string                       `yaml:"topic"`
	Name                    string                       `yaml:"name"`
	SendTimeout             time.Duration                `yaml:"send_timeout"`
	CompressionType         pulsar.CompressionType       `yaml:"compression_type"`
	DisableBlockIfQueueFull bool                         `yaml:"disable_block_if_queue_full"`
	Properties              map[string]string            `yaml:"properties"`
	Interceptors            []pulsar.ProducerInterceptor `yaml:"-"`
}

func (c *ProducerConfig) fix() {
	if c.SendTimeout <= 0 {
		c.SendTimeout = 3 * time.Second
	}
}

// ProducerOption configures a ProducerConfig.
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

func WithDisableBlockIfQueueFull(disable bool) ProducerOption {
	return func(c *ProducerConfig) { c.DisableBlockIfQueueFull = disable }
}

func WithProducerProperties(props map[string]string) ProducerOption {
	return func(c *ProducerConfig) { c.Properties = props }
}

func WithProducerInterceptor(ics ...pulsar.ProducerInterceptor) ProducerOption {
	return func(c *ProducerConfig) {
		c.Interceptors = append(c.Interceptors, ics...)
	}
}

// NewProducer creates a producer for the given topic.
func (c *Client) NewProducer(topic string, opts ...ProducerOption) (*Producer, error) {
	if topic == "" {
		return nil, fmt.Errorf("pulsar: producer topic is required")
	}
	cfg := &ProducerConfig{
		Topic:                   topic,
		DisableBlockIfQueueFull: true,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	cfg.fix()

	interceptors := cfg.Interceptors
	// Auto-inject metrics interceptor
	interceptors = append([]pulsar.ProducerInterceptor{&producerMetricsInterceptor{topic: topic}}, interceptors...)

	p, err := c.client.CreateProducer(pulsar.ProducerOptions{
		Topic:                   cfg.Topic,
		Name:                    cfg.Name,
		SendTimeout:             cfg.SendTimeout,
		CompressionType:         cfg.CompressionType,
		DisableBlockIfQueueFull: cfg.DisableBlockIfQueueFull,
		Properties:              cfg.Properties,
		Interceptors:            interceptors,
	})
	if err != nil {
		return nil, fmt.Errorf("pulsar: create producer: %w", err)
	}
	return &Producer{producer: p, topic: topic}, nil
}

// MustNewProducer creates a producer and panics on error.
func (c *Client) MustNewProducer(topic string, opts ...ProducerOption) *Producer {
	p, err := c.NewProducer(topic, opts...)
	if err != nil {
		panic(err)
	}
	return p
}

// Send sends a raw byte payload synchronously.
func (p *Producer) Send(ctx context.Context, data []byte) (pulsar.MessageID, error) {
	return p.producer.Send(ctx, &pulsar.ProducerMessage{Payload: data})
}

// SendAsync sends a raw byte payload asynchronously.
func (p *Producer) SendAsync(ctx context.Context, data []byte, callback func(pulsar.MessageID, *pulsar.ProducerMessage, error)) {
	p.producer.SendAsync(ctx, &pulsar.ProducerMessage{Payload: data}, callback)
}

// SendObj marshals obj to JSON and sends it as a string message.
func (p *Producer) SendObj(ctx context.Context, obj interface{}) (pulsar.MessageID, error) {
	bs, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("pulsar: marshal message: %w", err)
	}
	return p.producer.Send(ctx, &pulsar.ProducerMessage{Value: string(bs)})
}

// Close closes the producer.
func (p *Producer) Close() {
	p.producer.Close()
}
