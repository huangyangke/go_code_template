package mysql

import (
	"testing"
	"time"
)

func TestConfig_Fill(t *testing.T) {
	c := &Config{DSN: "user:pass@tcp(127.0.0.1:3306)/db"}
	c.Fix()

	if c.MaxIdleConns != 5 {
		t.Errorf("MaxIdleConns = %d, want 5", c.MaxIdleConns)
	}
	if c.MaxOpenConns != 20 {
		t.Errorf("MaxOpenConns = %d, want 20", c.MaxOpenConns)
	}
	if c.MaxLifetime != 600*time.Second {
		t.Errorf("MaxLifetime = %v, want 600s", c.MaxLifetime)
	}
}

func TestConfig_Fix_NoOverride(t *testing.T) {
	c := &Config{
		DSN:          "user:pass@tcp(127.0.0.1:3306)/db",
		MaxIdleConns: 10,
		MaxOpenConns: 50,
		MaxLifetime:  300 * time.Second,
	}
	c.Fix()

	if c.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns = %d, want 10", c.MaxIdleConns)
	}
	if c.MaxOpenConns != 50 {
		t.Errorf("MaxOpenConns = %d, want 50", c.MaxOpenConns)
	}
	if c.MaxLifetime != 300*time.Second {
		t.Errorf("MaxLifetime = %v, want 300s", c.MaxLifetime)
	}
}

func TestConfig_Validate_MissingDSN(t *testing.T) {
	c := &Config{}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	c := &Config{DSN: "user:pass@tcp(127.0.0.1:3306)/db"}
	err := c.Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
