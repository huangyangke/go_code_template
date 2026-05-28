package redis

import (
	"testing"
	"time"
)

func TestConfig_Fix(t *testing.T) {
	c := &Config{Name: "test", Addrs: []string{"127.0.0.1:6379"}}
	c.Fix()

	if c.Type != ClusterType {
		t.Errorf("Type = %q, want %q", c.Type, ClusterType)
	}
	if c.PoolSize != 16 {
		t.Errorf("PoolSize = %d, want 16", c.PoolSize)
	}
	if c.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", c.MaxRetries)
	}
	if c.MinIdleConns != 4 {
		t.Errorf("MinIdleConns = %d, want 4", c.MinIdleConns)
	}
	if c.DialTimeout != 3*time.Second {
		t.Errorf("DialTimeout = %v, want 3s", c.DialTimeout)
	}
	if c.ReadTimeout != time.Second {
		t.Errorf("ReadTimeout = %v, want 1s", c.ReadTimeout)
	}
	if c.WriteTimeout != time.Second {
		t.Errorf("WriteTimeout = %v, want 1s", c.WriteTimeout)
	}
	if c.PingTimeout != 3*time.Second {
		t.Errorf("PingTimeout = %v, want 3s", c.PingTimeout)
	}
}

func TestConfig_Fix_NoOverride(t *testing.T) {
	c := &Config{
		Addrs:      []string{"127.0.0.1:6379"},
		Type:       StandaloneType,
		PoolSize:   32,
		MaxRetries: 5,
	}
	c.Fix()

	if c.Type != StandaloneType {
		t.Errorf("Type = %q, want %q", c.Type, StandaloneType)
	}
	if c.PoolSize != 32 {
		t.Errorf("PoolSize = %d, want 32", c.PoolSize)
	}
	if c.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", c.MaxRetries)
	}
}

func TestConfig_Validate_NoAddrs(t *testing.T) {
	c := &Config{}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for empty Addrs")
	}
}

func TestConfig_Validate_SentinelNoMaster(t *testing.T) {
	c := &Config{
		Addrs: []string{"127.0.0.1:26379"},
		Type:  SentinelType,
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for sentinel without MasterName")
	}
}

func TestConfig_Validate_NoName(t *testing.T) {
	c := &Config{Addrs: []string{"127.0.0.1:6379"}, Type: StandaloneType}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty Name")
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	c := &Config{
		Name:  "my-redis",
		Addrs: []string{"127.0.0.1:6379"},
		Type:  StandaloneType,
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfig_fix_Panics(t *testing.T) {
	t.Run("no name", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for empty Name")
			}
		}()
		c := &Config{Addrs: []string{"127.0.0.1:6379"}, Type: StandaloneType}
		c.Fix()
		if err := c.Validate(); err != nil {
			panic(err.Error())
		}
	})

	t.Run("no addrs", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for empty Addrs")
			}
		}()
		c := &Config{Name: "test"}
		c.Fix()
		if err := c.Validate(); err != nil {
			panic(err.Error())
		}
	})

	t.Run("sentinel no master", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for sentinel without MasterName")
			}
		}()
		c := &Config{
			Name:  "test",
			Addrs: []string{"127.0.0.1:26379"},
			Type:  SentinelType,
		}
		c.Fix()
		if err := c.Validate(); err != nil {
			panic(err.Error())
		}
	})
}
