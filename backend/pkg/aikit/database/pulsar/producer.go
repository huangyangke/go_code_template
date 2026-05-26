package pulsar

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"

	"github.com/example/go-template/pkg/aikit/metrics"
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

	p, err := c.client.CreateProducer(pulsar.ProducerOptions{
		Topic:                   cfg.Topic,
		Name:                    cfg.Name,
		SendTimeout:             cfg.SendTimeout,
		CompressionType:         cfg.CompressionType,
		DisableBlockIfQueueFull: cfg.DisableBlockIfQueueFull,
		Properties:              cfg.Properties,
		Interceptors:            cfg.Interceptors,
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
// Records metrics (success, failure, duration) at the wrapper level, so failed
// sends are visible to Prometheus unlike the interceptor-only approach.
func (p *Producer) Send(ctx context.Context, data []byte) (pulsar.MessageID, error) {
	start := time.Now()
	id, err := p.producer.Send(ctx, &pulsar.ProducerMessage{Payload: data})
	metrics.ObservePulsarProduce(p.topic, err == nil, time.Since(start))
	return id, err
}

// SendAsync sends a raw byte payload asynchronously.
// The callback is wrapped to record metrics (success, failure, duration) when
// the send result is known. With DisableBlockIfQueueFull=true, some sends may
// error before the callback is invoked — those errors are not observable here.
func (p *Producer) SendAsync(ctx context.Context, data []byte, callback func(pulsar.MessageID, *pulsar.ProducerMessage, error)) {
	start := time.Now()
	p.producer.SendAsync(ctx, &pulsar.ProducerMessage{Payload: data}, func(id pulsar.MessageID, msg *pulsar.ProducerMessage, err error) {
		metrics.ObservePulsarProduce(p.topic, err == nil, time.Since(start))
		if callback != nil {
			callback(id, msg, err)
		}
	})
}

// SendObj marshals obj to JSON and sends it synchronously.
// Payload is sent as raw bytes (not schema-encoded), so non-Go consumers see
// plain JSON without Pulsar schema framing.
func (p *Producer) SendObj(ctx context.Context, obj interface{}) (pulsar.MessageID, error) {
	bs, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("pulsar: marshal message: %w", err)
	}
	return p.Send(ctx, bs)
}

// Close closes the producer.
func (p *Producer) Close() {
	p.producer.Close()
}
