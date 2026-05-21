package pulsar

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfig_fix_Defaults(t *testing.T) {
	c := &Config{}
	c.fix()
	assert.Equal(t, "pulsar://localhost:6650", c.URL)
	assert.Equal(t, "go-aikit-sub", c.Subscription)
	assert.Equal(t, 30, int(c.Timeout.Seconds()))
	assert.Equal(t, 3, c.MaxReconnectToBroker)
}

func TestConfig_fix_ExistingValues(t *testing.T) {
	c := &Config{
		URL:                "pulsar://custom:6650",
		Subscription:       "custom-sub",
		Timeout:            60 * time.Second,
		MaxReconnectToBroker: 5,
	}
	c.fix()
	assert.Equal(t, "pulsar://custom:6650", c.URL)
	assert.Equal(t, "custom-sub", c.Subscription)
	assert.Equal(t, 60, int(c.Timeout.Seconds()))
	assert.Equal(t, 5, c.MaxReconnectToBroker)
}

func TestWithURL(t *testing.T) {
	cfg := &Config{}
	WithURL("pulsar://test:6650")(cfg)
	assert.Equal(t, "pulsar://test:6650", cfg.URL)
}

func TestWithTopic(t *testing.T) {
	cfg := &Config{}
	WithTopic("my-topic")(cfg)
	assert.Equal(t, "my-topic", cfg.Topic)
}

func TestWithSubscription(t *testing.T) {
	cfg := &Config{}
	WithSubscription("my-sub")(cfg)
	assert.Equal(t, "my-sub", cfg.Subscription)
}

func TestWithTimeout(t *testing.T) {
	cfg := &Config{}
	WithTimeout(120 * time.Second)(cfg)
	assert.Equal(t, 120, int(cfg.Timeout.Seconds()))
}
