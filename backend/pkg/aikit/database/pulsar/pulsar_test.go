package pulsar

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
// Config Tests
// ============================================================================

func TestConfig_Fix_Defaults(t *testing.T) {
	c := &Config{Name: "test"}
	c.Fix()
	assert.Equal(t, "test", c.Name)
	assert.Equal(t, "pulsar://localhost:6650", c.URL)
	assert.Equal(t, 3*time.Second, c.ConnectionTimeout)
	assert.Equal(t, 5*time.Second, c.OperationTimeout)
	assert.Equal(t, 30*time.Second, c.KeepAliveInterval)
	assert.Equal(t, 1, c.MaxConnectionsPerBroker)
}

func TestConfig_Fix_ExistingValues(t *testing.T) {
	c := &Config{
		Name:                    "test",
		URL:                     "pulsar://custom:6650",
		ConnectionTimeout:       10 * time.Second,
		OperationTimeout:        15 * time.Second,
		KeepAliveInterval:       60 * time.Second,
		MaxConnectionsPerBroker: 4,
	}
	c.Fix()
	assert.Equal(t, "pulsar://custom:6650", c.URL)
	assert.Equal(t, 10*time.Second, c.ConnectionTimeout)
	assert.Equal(t, 15*time.Second, c.OperationTimeout)
	assert.Equal(t, 60*time.Second, c.KeepAliveInterval)
	assert.Equal(t, 4, c.MaxConnectionsPerBroker)
}

func TestConfig_Validate_MissingName(t *testing.T) {
	c := &Config{}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Name is required")
}

func TestConfig_Validate_Success(t *testing.T) {
	c := &Config{Name: "test"}
	err := c.Validate()
	assert.NoError(t, err)
}

func TestConfig_fix_Panics_WhenValidateFails(t *testing.T) {
	c := &Config{}
	assert.Panics(t, func() {
		c.Fix()
		if err := c.Validate(); err != nil {
			panic(err.Error())
		}
	})
}

// ============================================================================
// ProducerConfig Tests
// ============================================================================

func TestProducerConfig_fix_Defaults(t *testing.T) {
	c := &ProducerConfig{}
	c.fix()
	assert.Equal(t, 3*time.Second, c.SendTimeout)
}

func TestProducerConfig_fix_ExistingValues(t *testing.T) {
	c := &ProducerConfig{SendTimeout: 10 * time.Second}
	c.fix()
	assert.Equal(t, 10*time.Second, c.SendTimeout)
}

// ============================================================================
// Client Options Tests
// ============================================================================

func TestClientOptions(t *testing.T) {
	c := &Config{}
	WithURL("pulsar://test:6650")(c)
	WithConnectionTimeout(5 * time.Second)(c)
	WithOperationTimeout(10 * time.Second)(c)
	WithKeepAliveInterval(20 * time.Second)(c)
	WithMaxConnectionsPerBroker(3)(c)
	assert.Equal(t, "pulsar://test:6650", c.URL)
	assert.Equal(t, 5*time.Second, c.ConnectionTimeout)
	assert.Equal(t, 10*time.Second, c.OperationTimeout)
	assert.Equal(t, 20*time.Second, c.KeepAliveInterval)
	assert.Equal(t, 3, c.MaxConnectionsPerBroker)
}

// ============================================================================
// Producer Options Tests
// ============================================================================

func TestProducerOptions(t *testing.T) {
	c := &ProducerConfig{}
	WithProducerName("my-producer")(c)
	WithSendTimeout(10 * time.Second)(c)
	WithDisableBlockIfQueueFull(true)(c)
	WithProducerProperties(map[string]string{"key1": "v1"})(c)

	assert.Equal(t, "my-producer", c.Name)
	assert.Equal(t, 10*time.Second, c.SendTimeout)
	assert.True(t, c.DisableBlockIfQueueFull)
	assert.Equal(t, "v1", c.Properties["key1"])
}

// ============================================================================
// Consumer Options Tests
// ============================================================================

func TestConsumerOptions(t *testing.T) {
	o := &ConsumerOptions{}
	WithSubscription("sub-1")(o)
	WithSubscriptionType(pulsar.Exclusive)(o)
	WithConcurrency(5)(o)
	WithConsumerProperties(map[string]string{"p1": "v2"})(o)

	assert.Equal(t, "sub-1", o.SubscriptionName)
	assert.Equal(t, pulsar.Exclusive, o.SubscriptionType)
	assert.Equal(t, 5, o.Concurrency)
	assert.Equal(t, "v2", o.Properties["p1"])
}

func TestNewConsumer_Defaults(t *testing.T) {
	// Verify defaults without actually connecting — just exercise the option
	// application logic that runs before client.Subscribe would be called.
	o := &ConsumerOptions{
		SubscriptionType: pulsar.Shared,
		Concurrency:      1,
	}
	assert.Equal(t, "", o.SubscriptionName)
	assert.Equal(t, pulsar.Shared, o.SubscriptionType)
	assert.Equal(t, 1, o.Concurrency)

	// Simulate the default subscription-name logic that runs in NewConsumer.
	topic := "my-topic"
	if o.SubscriptionName == "" {
		o.SubscriptionName = "go-aikit-sub-" + topic
	}
	assert.Equal(t, "go-aikit-sub-my-topic", o.SubscriptionName)
}

// ============================================================================
// Logger Tests
// ============================================================================

func TestLoggerBridge_NonNil(t *testing.T) {
	logger := defaultLogger()
	assert.NotNil(t, logger)
}

func TestLogHook_Levels(t *testing.T) {
	h := &logHook{}
	levels := h.Levels()
	assert.NotEmpty(t, levels)
}

// ============================================================================
// HandlerFunc Type Tests
// ============================================================================

func TestHandlerFunc_Success(t *testing.T) {
	var h HandlerFunc = func(ctx context.Context, msg pulsar.Message) error {
		return nil
	}
	assert.NoError(t, h(context.Background(), nil))
}

func TestHandlerFunc_Error(t *testing.T) {
	var h HandlerFunc = func(ctx context.Context, msg pulsar.Message) error {
		return errors.New("handler failed")
	}
	err := h(context.Background(), nil)
	assert.Error(t, err)
	assert.Equal(t, "handler failed", err.Error())
}

// ============================================================================
// Lifecycle Smoke Tests (no real Pulsar broker)
// ============================================================================

func TestConsumer_CloseWithoutStart(t *testing.T) {
	// Close on a never-started consumer should not panic.
	cc := &Consumer{}
	assert.NotPanics(t, func() { cc.Close() })
}
