package xjob

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xxl-job/xxl-job-executor-go"
)

func TestConfig_Fix(t *testing.T) {
	c := &Config{ServerAddr: "http://localhost:8080", Family: "test-svc"}
	c.Fix()

	assert.NotEmpty(t, c.ExecutorIp, "ExecutorIp should be auto-detected")
	assert.Equal(t, "9999", c.ExecutorPort)
	assert.Equal(t, 7, c.MaxAge)
	assert.Equal(t, "logs/xjob", c.LogDir)
}

func TestConfig_Fix_NoOverride(t *testing.T) {
	c := &Config{
		ServerAddr:   "http://localhost:8080",
		Family:       "test-svc",
		ExecutorPort: "8888",
		MaxAge:       14,
		LogDir:       "custom/logs",
	}
	c.Fix()

	assert.Equal(t, "8888", c.ExecutorPort)
	assert.Equal(t, 14, c.MaxAge)
	assert.Equal(t, "custom/logs", c.LogDir)
}

func TestConfig_Validate_MissingServerAddr(t *testing.T) {
	c := &Config{Family: "test"}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server_addr")
}

func TestConfig_Validate_MissingFamily(t *testing.T) {
	c := &Config{ServerAddr: "http://localhost:8080"}
	err := c.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "family")
}

func TestConfig_Validate_Valid(t *testing.T) {
	c := &Config{ServerAddr: "http://localhost:8080", Family: "test"}
	err := c.Validate()
	assert.NoError(t, err)
}

func TestConfig_GetExtra(t *testing.T) {
	c := &Config{
		Extra: map[string]string{"key1": "value1"},
	}
	assert.Equal(t, "value1", c.GetExtra("key1"))
	assert.Equal(t, "", c.GetExtra("missing"))
	assert.Equal(t, "", (*Config)(nil).GetExtra("key1"))
}

func TestNewTask(t *testing.T) {
	taskFunc := func(ctx context.Context, param *xxl.RunReq) string {
		return "ok"
	}
	task := NewTask("testHandler", taskFunc)
	require.NotNil(t, task)
	assert.Equal(t, "testHandler", task.pattern)
}

func TestGetLocalIP(t *testing.T) {
	ip := getLocalIP()
	assert.NotEmpty(t, ip)
}
