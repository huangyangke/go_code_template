package redis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNew_NoAddrs_Panics(t *testing.T) {
	cfg := &Config{
		Addrs: []string{},
		Type:  StandaloneType,
	}
	_, err := New(cfg)
	assert.Error(t, err)
}

func TestNew_InvalidType_Panics(t *testing.T) {
	cfg := &Config{
		Addrs: []string{"localhost:6379"},
		Type:  "invalid",
	}
	_, err := New(cfg)
	assert.Error(t, err)
}

func TestNew_SentinelNoMasterName_Panics(t *testing.T) {
	cfg := &Config{
		Addrs: []string{"localhost:26379"},
		Type:  SentinelType,
		// MasterName intentionally empty
	}
	_, err := New(cfg)
	assert.Error(t, err)
}

func TestConfig_fix_Defaults(t *testing.T) {
	cfg := &Config{
		Name:  "test",
		Addrs: []string{"localhost:6379"},
	}
	cfg.Fix()
	assert.Equal(t, ClusterType, cfg.Type)
	assert.Equal(t, 16, cfg.PoolSize)
	assert.Equal(t, 3, cfg.MaxRetries)
}

func TestKeyPrefix(t *testing.T) {
	r := &Redis{
		config: &Config{KeyPrefix: "myapp"},
		client: nil,
	}
	assert.Equal(t, "myapp:users", r.key("users"))
	assert.Equal(t, "myapp:", r.key(""))
}

func TestKeyNoPrefix(t *testing.T) {
	r := &Redis{
		config: &Config{},
		client: nil,
	}
	assert.Equal(t, "users", r.key("users"))
}

func TestKeyDisableKeyPrefix(t *testing.T) {
	r := &Redis{
		config: &Config{KeyPrefix: "myapp", DisableKeyPrefix: true},
		client: nil,
	}
	assert.Equal(t, "users", r.key("users"))
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, ClusterType, cfg.Type)
	assert.Equal(t, 16, cfg.PoolSize)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 4, cfg.MinIdleConns)
	assert.Equal(t, 3*time.Second, cfg.DialTimeout)
	assert.Equal(t, time.Second, cfg.ReadTimeout)
	assert.Equal(t, time.Second, cfg.WriteTimeout)
	assert.Equal(t, time.Minute, cfg.IdleTimeout)
	assert.Equal(t, 3*time.Second, cfg.PingTimeout)
}
