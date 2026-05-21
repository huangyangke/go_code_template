package pulsar

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
)

type Config struct {
	URL                string        `yaml:"url"`
	Topic              string        `yaml:"topic"`
	Subscription       string        `yaml:"subscription"`
	Name               string        `yaml:"name"`
	Timeout            time.Duration `yaml:"timeout"`
	MaxReconnectToBroker int         `yaml:"max_reconnect_to_broker"`
}

func (c *Config) fix() {
	if c.URL == "" {
		c.URL = "pulsar://localhost:6650"
	}
	if c.Subscription == "" {
		c.Subscription = "go-aikit-sub"
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MaxReconnectToBroker <= 0 {
		c.MaxReconnectToBroker = 3
	}
}

type Pulsar struct {
	config     *Config
	client     pulsar.Client
	producer   pulsar.Producer
	consumer   pulsar.Consumer
	producerMu sync.Mutex
	consumerMu sync.Mutex
}

type Option func(c *Config)

func WithURL(url string) Option {
	return func(c *Config) { c.URL = url }
}

func WithTopic(topic string) Option {
	return func(c *Config) { c.Topic = topic }
}

func WithSubscription(sub string) Option {
	return func(c *Config) { c.Subscription = sub }
}

func WithTimeout(d time.Duration) Option {
	return func(c *Config) { c.Timeout = d }
}

func New(c *Config, opts ...Option) *Pulsar {
	for _, opt := range opts {
		opt(c)
	}
	c.fix()

	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL:                     c.URL,
		ConnectionTimeout:       c.Timeout,
		OperationTimeout:        c.Timeout,
		MaxConnectionsPerBroker: c.MaxReconnectToBroker,
	})
	if err != nil {
		panic(fmt.Sprintf("pulsar connect error: %v", err))
	}

	p := &Pulsar{
		config: c,
		client: client,
	}

	if c.Topic != "" {
		p.producer, err = client.CreateProducer(pulsar.ProducerOptions{
			Topic: c.Topic,
			Name:  c.Name,
		})
		if err != nil {
			client.Close()
			panic(fmt.Sprintf("pulsar create producer error: %v", err))
		}
	}

	if c.Topic != "" && c.Subscription != "" {
		p.consumer, err = client.Subscribe(pulsar.ConsumerOptions{
			Topic:            c.Topic,
			SubscriptionName: c.Subscription,
			Type:             pulsar.Shared,
		})
		if err != nil {
			client.Close()
			if p.producer != nil {
				p.producer.Close()
			}
			panic(fmt.Sprintf("pulsar subscribe error: %v", err))
		}
	}

	return p
}

func (p *Pulsar) Client() pulsar.Client {
	return p.client
}

func (p *Pulsar) Producer() pulsar.Producer {
	return p.producer
}

func (p *Pulsar) Consumer() pulsar.Consumer {
	return p.consumer
}

func (p *Pulsar) Send(ctx context.Context, data []byte) error {
	p.producerMu.Lock()
	defer p.producerMu.Unlock()

	if p.producer == nil {
		return fmt.Errorf("pulsar producer not initialized")
	}

	_, err := p.producer.Send(ctx, &pulsar.ProducerMessage{
		Payload: data,
	})
	return err
}

func (p *Pulsar) Receive(ctx context.Context) (pulsar.Message, error) {
	p.consumerMu.Lock()
	defer p.consumerMu.Unlock()

	if p.consumer == nil {
		return nil, fmt.Errorf("pulsar consumer not initialized")
	}

	return p.consumer.Receive(ctx)
}

func (p *Pulsar) Acknowledge(msg pulsar.Message) error {
	if p.consumer != nil {
		return p.consumer.Ack(msg)
	}
	return nil
}

func (p *Pulsar) Close() error {
	if p.producer != nil {
		p.producer.Close()
	}

	if p.consumer != nil {
		p.consumer.Close()
	}

	if p.client != nil {
		p.client.Close()
	}

	return nil
}
