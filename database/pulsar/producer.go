package pulsar

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"

	"github.com/huangyangke/go-aikit/metrics"
)

// Producer 封装 pulsar.Producer.
type Producer struct {
	producer pulsar.Producer
	topic    string
}

// ProducerConfig 生产者配置.
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

// ProducerOption 生产者配置函数.
type ProducerOption func(*ProducerConfig)

// WithProducerName 设置生产者名称.
// 参数：name - 生产者名称.
// 返回值：ProducerOption - 配置函数.
func WithProducerName(name string) ProducerOption {
	return func(c *ProducerConfig) { c.Name = name }
}

// WithSendTimeout 设置发送超时时间.
// 参数：d - 超时时间.
// 返回值：ProducerOption - 配置函数.
func WithSendTimeout(d time.Duration) ProducerOption {
	return func(c *ProducerConfig) { c.SendTimeout = d }
}

// WithCompressionType 设置压缩类型.
// 参数：ct - 压缩类型.
// 返回值：ProducerOption - 配置函数.
func WithCompressionType(ct pulsar.CompressionType) ProducerOption {
	return func(c *ProducerConfig) { c.CompressionType = ct }
}

// WithDisableBlockIfQueueFull 设置队列满时是否阻塞发送.
// 参数：disable - 是否禁用阻塞.
// 返回值：ProducerOption - 配置函数.
func WithDisableBlockIfQueueFull(disable bool) ProducerOption {
	return func(c *ProducerConfig) { c.DisableBlockIfQueueFull = disable }
}

// WithProducerProperties 设置生产者属性.
// 参数：props - 属性映射.
// 返回值：ProducerOption - 配置函数.
func WithProducerProperties(props map[string]string) ProducerOption {
	return func(c *ProducerConfig) { c.Properties = props }
}

// WithProducerInterceptor 添加生产者拦截器.
// 参数：ics - 拦截器列表.
// 返回值：ProducerOption - 配置函数.
func WithProducerInterceptor(ics ...pulsar.ProducerInterceptor) ProducerOption {
	return func(c *ProducerConfig) {
		c.Interceptors = append(c.Interceptors, ics...)
	}
}

// NewProducer 为指定主题创建生产者.
// 参数：topic - 主题名称, opts - 配置选项.
// 返回值：*Producer - 生产者实例, err - 创建失败时的错误.
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

// MustNewProducer 创建生产者，失败时 panic.
// 参数：topic - 主题名称, opts - 配置选项.
// 返回值：*Producer - 生产者实例.
func (c *Client) MustNewProducer(topic string, opts ...ProducerOption) *Producer {
	p, err := c.NewProducer(topic, opts...)
	if err != nil {
		panic(err)
	}
	return p
}

// Send 同步发送原始字节消息.
// 参数：ctx - 上下文, data - 消息内容.
// 返回值：pulsar.MessageID - 消息 ID, err - 发送失败时的错误.
func (p *Producer) Send(ctx context.Context, data []byte) (pulsar.MessageID, error) {
	start := time.Now()
	id, err := p.producer.Send(ctx, &pulsar.ProducerMessage{Payload: data})
	metrics.ObservePulsarProduce(p.topic, err == nil, time.Since(start))
	return id, err
}

// SendAsync 异步发送原始字节消息.
// 参数：ctx - 上下文, data - 消息内容, callback - 发送结果回调.
func (p *Producer) SendAsync(ctx context.Context, data []byte, callback func(pulsar.MessageID, *pulsar.ProducerMessage, error)) {
	start := time.Now()
	p.producer.SendAsync(ctx, &pulsar.ProducerMessage{Payload: data}, func(id pulsar.MessageID, msg *pulsar.ProducerMessage, err error) {
		metrics.ObservePulsarProduce(p.topic, err == nil, time.Since(start))
		if callback != nil {
			callback(id, msg, err)
		}
	})
}

// SendObj 将对象序列化为 JSON 后同步发送.
// 参数：ctx - 上下文, obj - 待发送对象.
// 返回值：pulsar.MessageID - 消息 ID, err - 序列化或发送失败时的错误.
func (p *Producer) SendObj(ctx context.Context, obj interface{}) (pulsar.MessageID, error) {
	bs, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("pulsar: marshal message: %w", err)
	}
	return p.Send(ctx, bs)
}

// Close 关闭生产者.
func (p *Producer) Close() {
	p.producer.Close()
}
