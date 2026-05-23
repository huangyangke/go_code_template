package redis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_NoAddrs_Panics(t *testing.T) {
	cfg := &Config{
		Addrs: []string{},
		Type:  StandaloneType,
	}
	assert.Panics(t, func() { New(cfg) })
}

func TestNew_InvalidType_Panics(t *testing.T) {
	cfg := &Config{
		Addrs: []string{"localhost:6379"},
		Type:  "invalid",
	}
	assert.Panics(t, func() { New(cfg) })
}

func TestNew_SentinelNoMasterName_Panics(t *testing.T) {
	cfg := &Config{
		Addrs: []string{"localhost:26379"},
		Type:  SentinelType,
		// MasterName intentionally empty
	}
	assert.Panics(t, func() { New(cfg) })
}

func TestConfig_fix_Defaults(t *testing.T) {
	cfg := &Config{
		Name:  "test",
		Addrs: []string{"localhost:6379"},
	}
	cfg.fix()
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
