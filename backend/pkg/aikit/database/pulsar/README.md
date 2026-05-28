# pulsar — Apache Pulsar 客户端

Apache Pulsar 客户端封装，Client/Producer/Consumer 三层分离，内置 Prometheus 指标、并发消费、gopool 托管 goroutine。

## 架构

```
Client (连接管理)
  ├── NewProducer(topic) → Producer (同步/异步发送)
  └── NewConsumer(topic) → Consumer (串行/并发消费)
```

## Client

```go
client := pulsar.New(&pulsar.Config{
    Name: "main",                    // 必填，用于 Prometheus 标签
    URL:  "pulsar://localhost:6650",
})
defer client.Close()

// 访问底层客户端（高级用法）
raw := client.Raw()
```

## Producer

```go
producer, err := client.NewProducer("my-topic",
    pulsar.WithSendTimeout(3 * time.Second),
    pulsar.WithCompressionType(pulsar.LZ4),
)
defer producer.Close()

// 同步发送字节
id, err := producer.Send(ctx, []byte("hello"))

// 同步发送对象（JSON 序列化）
id, err := producer.SendObj(ctx, myStruct)

// 异步发送
producer.SendAsync(ctx, data, func(id pulsar.MessageID, msg *pulsar.ProducerMessage, err error) {
    // 回调中处理结果
})
```

## Consumer

```go
consumer, err := client.NewConsumer("my-topic",
    pulsar.WithSubscription("my-sub"),
    pulsar.WithSubscriptionType(pulsar.Shared),
    pulsar.WithConcurrency(4),          // 并发消费（默认 1，串行）
)

// 启动消费（非阻塞，goroutine 由 gopool 托管）
consumer.Start(func(ctx context.Context, msg pulsar.Message) error {
    // 返回 nil → Ack；返回 error → Nack
    payload := msg.Payload()
    return process(payload)
})

defer consumer.Close() // 等待所有 in-flight handler 完成后关闭
```

## 配置

```yaml
# Client
name: main                       # 必填（Prometheus datasource 标签）
url: pulsar://localhost:6650     # 默认
connection_timeout: 3s           # 默认
operation_timeout: 5s            # 默认
keep_alive_interval: 30s         # 默认
max_connections_per_broker: 1    # 默认
```

## 指标

| 指标名 | 类型 | Labels |
|---|---|---|
| `pulsar_produce_total` | counter | `topic, success` |
| `pulsar_produce_duration_seconds` | histogram | `topic, success` |
| `pulsar_consume_total` | counter | `topic, result`（ack/nack） |
| `pulsar_consume_duration_seconds` | histogram | `topic, result` |
