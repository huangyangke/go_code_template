package pulsar

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"

	"github.com/huangyangke/go-aikit/log"
	"github.com/huangyangke/go-aikit/metrics"
	"github.com/huangyangke/go-aikit/utils/gopool"
)

// HandlerFunc 处理 Pulsar 消息，返回 error 则 Nack，返回 nil 则 Ack.
type HandlerFunc func(ctx context.Context, msg pulsar.Message) error

// Consumer 封装 pulsar.Consumer，提供生命周期管理.
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

// ConsumerOptions 消费者配置.
type ConsumerOptions struct {
	SubscriptionName string
	SubscriptionType pulsar.SubscriptionType
	Concurrency      int
	Properties       map[string]string
	Interceptors     []pulsar.ConsumerInterceptor
}

// ConsumerOption 消费者配置函数.
type ConsumerOption func(*ConsumerOptions)

// WithSubscription 设置订阅名称.
// 参数：sub - 订阅名称.
// 返回值：ConsumerOption - 配置函数.
func WithSubscription(sub string) ConsumerOption {
	return func(o *ConsumerOptions) { o.SubscriptionName = sub }
}

// WithSubscriptionType 设置订阅类型.
// 参数：t - 订阅类型.
// 返回值：ConsumerOption - 配置函数.
func WithSubscriptionType(t pulsar.SubscriptionType) ConsumerOption {
	return func(o *ConsumerOptions) { o.SubscriptionType = t }
}

// WithConcurrency 设置并发处理数.
// 参数：n - 并发数.
// 返回值：ConsumerOption - 配置函数.
func WithConcurrency(n int) ConsumerOption {
	return func(o *ConsumerOptions) { o.Concurrency = n }
}

// WithConsumerProperties 设置消费者属性.
// 参数：props - 属性映射.
// 返回值：ConsumerOption - 配置函数.
func WithConsumerProperties(props map[string]string) ConsumerOption {
	return func(o *ConsumerOptions) { o.Properties = props }
}

// WithConsumerInterceptor 添加消费者拦截器.
// 参数：ics - 拦截器列表.
// 返回值：ConsumerOption - 配置函数.
func WithConsumerInterceptor(ics ...pulsar.ConsumerInterceptor) ConsumerOption {
	return func(o *ConsumerOptions) { o.Interceptors = append(o.Interceptors, ics...) }
}

// NewConsumer 为指定主题创建消费者.
// 参数：topic - 主题名称, opts - 配置选项.
// 返回值：*Consumer - 消费者实例, err - 创建失败时的错误.
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

// MustNewConsumer 创建消费者，失败时 panic.
// 参数：topic - 主题名称, opts - 配置选项.
// 返回值：*Consumer - 消费者实例.
func (c *Client) MustNewConsumer(topic string, opts ...ConsumerOption) *Consumer {
	cs, err := c.NewConsumer(topic, opts...)
	if err != nil {
		panic(err)
	}
	return cs
}

// Start 启动消息消费循环，幂等操作，已启动时立即返回.
// 参数：fn - 消息处理函数.
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
				metrics.ObservePulsarConsume(cc.topic, "nack", 0)
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
				metrics.ObservePulsarConsume(cc.topic, "nack", 0)
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

// Close 停止消费循环并关闭底层消费者，阻塞等待所有进行中的处理完成.
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
