package pulsar

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"

	"github.com/example/go-template/pkg/aikit/log"
	"github.com/example/go-template/pkg/aikit/metrics"
	"github.com/example/go-template/pkg/aikit/utils/gopool"
)

// HandlerFunc processes a Pulsar message. Return error to Nack, nil to Ack.
type HandlerFunc func(ctx context.Context, msg pulsar.Message) error

// Consumer wraps pulsar.Consumer with lifecycle management.
type Consumer struct {
	consumer pulsar.Consumer
	topic    string
	opts     *ConsumerOptions
	started  bool
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup // tracks in-flight handler goroutines
}

// ConsumerOptions holds configuration for a Pulsar consumer.
type ConsumerOptions struct {
	SubscriptionName string
	SubscriptionType pulsar.SubscriptionType
	Concurrency      int
	Properties       map[string]string
	Interceptors     []pulsar.ConsumerInterceptor
}

// ConsumerOption configures consumer creation.
type ConsumerOption func(*ConsumerOptions)

func WithSubscription(sub string) ConsumerOption {
	return func(o *ConsumerOptions) { o.SubscriptionName = sub }
}

func WithSubscriptionType(t pulsar.SubscriptionType) ConsumerOption {
	return func(o *ConsumerOptions) { o.SubscriptionType = t }
}

func WithConcurrency(n int) ConsumerOption {
	return func(o *ConsumerOptions) { o.Concurrency = n }
}

func WithConsumerProperties(props map[string]string) ConsumerOption {
	return func(o *ConsumerOptions) { o.Properties = props }
}

func WithConsumerInterceptor(ics ...pulsar.ConsumerInterceptor) ConsumerOption {
	return func(o *ConsumerOptions) { o.Interceptors = append(o.Interceptors, ics...) }
}

// NewConsumer creates a consumer for the given topic.
func (c *Client) NewConsumer(topic string, opts ...ConsumerOption) (*Consumer, error) {
	if topic == "" {
		return nil, fmt.Errorf("pulsar: consumer topic is required")
	}
	o := &ConsumerOptions{
		SubscriptionType: pulsar.Shared,
		Concurrency:      1,
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.SubscriptionName == "" {
		o.SubscriptionName = fmt.Sprintf("go-aikit-sub-%s", topic)
	}

	cs, err := c.client.Subscribe(pulsar.ConsumerOptions{
		Topic:            topic,
		SubscriptionName: o.SubscriptionName,
		Type:             o.SubscriptionType,
		Properties:       o.Properties,
		Interceptors:     o.Interceptors,
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
// For concurrency>1, messages are dispatched to bounded goroutines via semaphore.
func (cc *Consumer) Start(fn HandlerFunc) {
	cc.mu.Lock()
	if cc.started {
		cc.mu.Unlock()
		return
	}
	cc.started = true
	cc.ctx, cc.cancel = context.WithCancel(context.Background())
	cc.mu.Unlock()

	if cc.opts.Concurrency <= 1 {
		cc.startSerial(fn)
	} else {
		cc.startConcurrent(fn)
	}
}

func (cc *Consumer) startSerial(fn HandlerFunc) {
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorv(cc.ctx, log.KVString("topic", cc.topic),
					log.KVString("log", fmt.Sprintf("Consumer panic: %v", r)))
			}
		}()
		for {
			msg, err := cc.consumer.Receive(cc.ctx)
			if err != nil {
				if cc.ctx.Err() != nil {
					log.Infov(cc.ctx, log.KVString("topic", cc.topic),
						log.KVString("log", "Consumer stopped"))
					return
				}
				log.Errorv(cc.ctx, log.KVString("topic", cc.topic),
					log.KVString("log", fmt.Sprintf("Failed to receive message: %v", err)))
				time.Sleep(time.Second) // backoff on transient errors
				continue
			}
			cc.handleAndAck(msg, fn)
		}
	})
}

func (cc *Consumer) startConcurrent(fn HandlerFunc) {
	ch := cc.consumer.Chan()
	sem := make(chan struct{}, cc.opts.Concurrency)

	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorv(cc.ctx, log.KVString("topic", cc.topic),
					log.KVString("log", fmt.Sprintf("Consumer panic: %v", r)))
			}
		}()
		for {
			select {
			case <-cc.ctx.Done():
				log.Infov(cc.ctx, log.KVString("topic", cc.topic),
					log.KVString("log", "Consumer stopped"))
				return
			case cm, ok := <-ch:
				if !ok {
					return
				}
				// Acquire semaphore slot (backs off the consumer when at limit).
				// We use regular goroutines for handler dispatch (not gopool.Go)
				// because we need explicit WaitGroup-based lifecycle control.
				sem <- struct{}{}
				cc.wg.Add(1)
				go func(cm pulsar.ConsumerMessage) {
					defer cc.wg.Done()
					defer func() { <-sem }()
					cc.handleAndAck(cm.Message, fn)
				}(cm)
			}
		}
	})
}

func (cc *Consumer) handleAndAck(msg pulsar.Message, fn HandlerFunc) {
	start := time.Now()
	err := fn(cc.ctx, msg)
	duration := time.Since(start)

	if err != nil {
		metrics.ObservePulsarConsume(cc.topic, "nack", duration)
		cc.consumer.NackID(msg.ID())
		log.Errorv(cc.ctx,
			log.KVString("topic", cc.topic),
			log.KVString("log", fmt.Sprintf("Handler error, nacking message: %v", err)),
		)
	} else {
		metrics.ObservePulsarConsume(cc.topic, "ack", duration)
		if ackErr := cc.consumer.AckID(msg.ID()); ackErr != nil {
			log.Errorv(cc.ctx,
				log.KVString("topic", cc.topic),
				log.KVString("log", fmt.Sprintf("Failed to ack message: %v", ackErr)),
			)
		}
	}
}

// Close stops the consumer loop and closes the underlying consumer.
// Safe to call multiple times. Blocks until all in-flight handlers complete.
func (cc *Consumer) Close() {
	cc.mu.Lock()
	if !cc.started {
		cc.mu.Unlock()
		if cc.consumer != nil {
			cc.consumer.Close()
		}
		return
	}
	cc.started = false
	cc.cancel()
	cc.mu.Unlock()
	cc.wg.Wait() // wait for in-flight handlers
	cc.consumer.Close()
}
