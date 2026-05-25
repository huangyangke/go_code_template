package pulsar

import (
	"time"

	"github.com/apache/pulsar-client-go/pulsar"

	"github.com/example/go-template/pkg/aikit/metrics"
)

// producerMetricsInterceptor implements pulsar.ProducerInterceptor to record
// send latency and success/failure counts.
type producerMetricsInterceptor struct {
	topic string
}

func (i *producerMetricsInterceptor) BeforeSend(producer pulsar.Producer, message *pulsar.ProducerMessage) {
	message.EventTime = time.Now()
}

func (i *producerMetricsInterceptor) OnSendAcknowledgement(producer pulsar.Producer, message *pulsar.ProducerMessage, msgID pulsar.MessageID) {
	var duration time.Duration
	if !message.EventTime.IsZero() {
		duration = time.Since(message.EventTime)
	}
	metrics.ObservePulsarProduce(i.topic, true, duration)
}

// consumerMetricsInterceptor implements pulsar.ConsumerInterceptor to record
// consumption result counts.
type consumerMetricsInterceptor struct {
	topic string
}

func (i *consumerMetricsInterceptor) BeforeConsume(message pulsar.ConsumerMessage) {
	// No-op: we don't have a way to track handler processing start time from interceptor.
}

func (i *consumerMetricsInterceptor) OnAcknowledge(consumer pulsar.Consumer, msgID pulsar.MessageID) {
	metrics.ObservePulsarConsume(i.topic, "ack", 0)
}

func (i *consumerMetricsInterceptor) OnNegativeAcksSend(consumer pulsar.Consumer, msgIDs []pulsar.MessageID) {
	metrics.ObservePulsarConsume(i.topic, "nack", 0)
}
