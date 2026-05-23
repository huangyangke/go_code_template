package pulsar

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClientConfig_fix_Defaults(t *testing.T) {
	c := &ClientConfig{}
	c.fix()
	assert.Equal(t, "pulsar://localhost:6650", c.URL)
	assert.Equal(t, 30*time.Second, c.Timeout)
	assert.Equal(t, 30*time.Second, c.KeepAliveInterval)
	assert.Equal(t, 1, c.MaxConnectionsPerBroker)
}

func TestClientConfig_fix_ExistingValues(t *testing.T) {
	c := &ClientConfig{
		URL:                     "pulsar://custom:6650",
		Timeout:                 60 * time.Second,
		KeepAliveInterval:       10 * time.Second,
		MaxConnectionsPerBroker: 4,
	}
	c.fix()
	assert.Equal(t, "pulsar://custom:6650", c.URL)
	assert.Equal(t, 60*time.Second, c.Timeout)
	assert.Equal(t, 10*time.Second, c.KeepAliveInterval)
	assert.Equal(t, 4, c.MaxConnectionsPerBroker)
}

func TestProducerConfig_fix_Defaults(t *testing.T) {
	c := &ProducerConfig{}
	c.fix()
	assert.Equal(t, 3*time.Second, c.SendTimeout)
}

func TestConsumerConfig_fix_Defaults(t *testing.T) {
	c := &ConsumerConfig{}
	c.fix()
	assert.Equal(t, "go-aikit-sub", c.Subscription)
}

func TestConsumerConfig_fix_ExistingValues(t *testing.T) {
	c := &ConsumerConfig{Subscription: "my-sub"}
	c.fix()
	assert.Equal(t, "my-sub", c.Subscription)
}

func TestClientOptions(t *testing.T) {
	c := &ClientConfig{}
	WithURL("pulsar://test:6650")(c)
	WithTimeout(5 * time.Second)(c)
	WithKeepAliveInterval(15 * time.Second)(c)
	WithMaxConnectionsPerBroker(2)(c)
	assert.Equal(t, "pulsar://test:6650", c.URL)
	assert.Equal(t, 5*time.Second, c.Timeout)
	assert.Equal(t, 15*time.Second, c.KeepAliveInterval)
	assert.Equal(t, 2, c.MaxConnectionsPerBroker)
}

func TestProducerOptions(t *testing.T) {
	c := &ProducerConfig{}
	WithProducerName("my-producer")(c)
	WithSendTimeout(10 * time.Second)(c)
	assert.Equal(t, "my-producer", c.Name)
	assert.Equal(t, 10*time.Second, c.SendTimeout)
}

func TestConsumerOptions(t *testing.T) {
	c := &ConsumerConfig{}
	WithSubscription("my-sub")(c)
	assert.Equal(t, "my-sub", c.Subscription)
}
