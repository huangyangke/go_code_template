package pulsar

import (
	"context"
	"fmt"
	"sync"

	"github.com/apache/pulsar-client-go/pulsar"
	"golang.org/x/sync/errgroup"

	"github.com/example/go-template/pkg/aikit/log"
	"github.com/example/go-template/pkg/aikit/utils/gopool"
)

// HandlerFunc processes a Pulsar message. Return error to Nack, nil to Ack.
type HandlerFunc func(ctx context.Context, msg pulsar.Message) error

// Consumer wraps pulsar.Consumer with lifecycle management.
type Consumer struct {
	consumer pulsar.Consumer
	topic    string
	opts     *consumerOptions
	started  bool
	mu       sync.Mutex
}

type consumerOptions struct {
	subscriptionName string
	subscriptionType pulsar.SubscriptionType
	concurrency      int
	properties       map[string]string
	interceptors     []pulsar.ConsumerInterceptor
	stop             chan struct{}
}

// ConsumerOption configures consumer creation.
type ConsumerOption func(*consumerOptions)

func WithSubscription(sub string) ConsumerOption {
	return func(o *consumerOptions) { o.subscriptionName = sub }
}

func WithSubscriptionType(t pulsar.SubscriptionType) ConsumerOption {
	return func(o *consumerOptions) { o.subscriptionType = t }
}

func WithConcurrency(n int) ConsumerOption {
	return func(o *consumerOptions) { o.concurrency = n }
}

func WithConsumerProperties(props map[string]string) ConsumerOption {
	return func(o *consumerOptions) { o.properties = props }
}

func WithConsumerInterceptor(ics ...pulsar.ConsumerInterceptor) ConsumerOption {
	return func(o *consumerOptions) { o.interceptors = append(o.interceptors, ics...) }
}

// NewConsumer creates a consumer for the given topic.
func (c *Client) NewConsumer(topic string, opts ...ConsumerOption) (*Consumer, error) {
	if topic == "" {
		return nil, fmt.Errorf("pulsar: consumer topic is required")
	}
	o := &consumerOptions{
		subscriptionType: pulsar.Shared,
		concurrency:      1,
		stop:             make(chan struct{}),
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.subscriptionName == "" {
		o.subscriptionName = "go-aikit-sub"
	}

	interceptors := o.interceptors
	// Auto-inject metrics interceptor
	interceptors = append([]pulsar.ConsumerInterceptor{&consumerMetricsInterceptor{topic: topic}}, interceptors...)

	cs, err := c.client.Subscribe(pulsar.ConsumerOptions{
		Topic:            topic,
		SubscriptionName: o.subscriptionName,
		Type:             o.subscriptionType,
		Properties:       o.properties,
		Interceptors:     interceptors,
	})
	if err != nil {
		return nil, fmt.Errorf("pulsar: subscribe: %w", err)
	}
	return &Consumer{
		consumer: cs,
		topic:    topic,
		opts:     o,
	}, nil
}

// MustNewConsumer creates a consumer and panics on error.
func (c *Client) MustNewConsumer(topic string, opts ...ConsumerOption) *Consumer {
	cs, err := c.NewConsumer(topic, opts...)
	if err != nil {
		panic(err)
	}
	return cs
}

// Start begins consuming messages. Idempotent: returns immediately if already started.
// For concurrency=1 (default), messages are processed serially.
// For concurrency>1, messages are batched from the consumer channel and processed concurrently via errgroup.
func (cc *Consumer) Start(fn HandlerFunc) {
	cc.mu.Lock()
	if cc.started {
		cc.mu.Unlock()
		return
	}
	cc.started = true
	cc.mu.Unlock()

	if cc.opts.concurrency <= 1 {
		cc.startSerial(fn)
	} else {
		cc.startConcurrent(fn)
	}
}

func (cc *Consumer) startSerial(fn HandlerFunc) {
	gopool.Go(func() {
		for {
			select {
			case <-cc.opts.stop:
				log.Infov(context.Background(), log.KVString("topic", cc.topic), log.KVString("log", "Consumer stopped"))
				return
			default:
				ctx := context.Background()
				msg, err := cc.consumer.Receive(ctx)
				if err != nil {
					log.Errorv(ctx, log.KVString("topic", cc.topic), log.KVString("log", fmt.Sprintf("Failed to receive message: %v", err)))
					continue
				}
				cc.handleAndAck(ctx, msg, fn)
			}
		}
	})
}

func (cc *Consumer) startConcurrent(fn HandlerFunc) {
	ch := cc.consumer.Chan()
	gopool.Go(func() {
		for {
			select {
			case <-cc.opts.stop:
				log.Infov(context.Background(), log.KVString("topic", cc.topic), log.KVString("log", "Consumer stopped"))
				return
			case cm, ok := <-ch:
				if !ok {
					return
				}
				cc.handleConcurrentBatch(cm, fn)
			}
		}
	})
}

// handleConcurrentBatch reads a single ConsumerMessage and processes it.
func (cc *Consumer) handleConcurrentBatch(cm pulsar.ConsumerMessage, fn HandlerFunc) {
	ctx := context.Background()
	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(cc.opts.concurrency)

	msg := cm.Message
	g.Go(func() error {
		cc.handleAndAck(ctx, msg, fn)
		return nil
	})
	_ = g.Wait()
}

func (cc *Consumer) handleAndAck(ctx context.Context, msg pulsar.Message, fn HandlerFunc) {
	err := fn(ctx, msg)
	if err != nil {
		cc.consumer.NackID(msg.ID())
		log.Errorv(ctx,
			log.KVString("topic", cc.topic),
			log.KVString("log", fmt.Sprintf("Handler error, nacking message: %v", err)),
		)
	} else {
		if ackErr := cc.consumer.AckID(msg.ID()); ackErr != nil {
			log.Errorv(ctx,
				log.KVString("topic", cc.topic),
				log.KVString("log", fmt.Sprintf("Failed to ack message: %v", ackErr)),
			)
		}
	}
}

// Close stops the consumer loop and closes the underlying consumer.
func (cc *Consumer) Close() {
	cc.mu.Lock()
	if !cc.started {
		cc.mu.Unlock()
		cc.consumer.Close()
		return
	}
	close(cc.opts.stop)
	cc.mu.Unlock()
	cc.consumer.Close()
}
