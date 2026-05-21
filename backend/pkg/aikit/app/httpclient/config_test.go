package httpclient

import (
	"testing"
	"time"
)

func TestConfig_Fix(t *testing.T) {
	cfg := Config{Name: "test"}
	cfg.Fix()

	if cfg.Timeout != 2*time.Second {
		t.Errorf("expected timeout=2s, got %s", cfg.Timeout)
	}
}

func TestConfig_Validate(t *testing.T) {
	cfg := Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty Name")
	}

	cfg.Name = "test"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBreakerConfig_Fix(t *testing.T) {
	cfg := BreakerConfig{}
	cfg.Fix()

	if cfg.Name != "httpclient" {
		t.Errorf("expected default name httpclient, got %s", cfg.Name)
	}
	if cfg.MaxRequests != 1 {
		t.Errorf("expected 1, got %d", cfg.MaxRequests)
	}
	if cfg.RequestVolumeThreshold != 20 {
		t.Errorf("expected 20, got %d", cfg.RequestVolumeThreshold)
	}
	if cfg.SleepWindow != 5*time.Second {
		t.Errorf("expected 5s, got %s", cfg.SleepWindow)
	}
	if cfg.ErrorPercentThreshold != 50 {
		t.Errorf("expected 50, got %d", cfg.ErrorPercentThreshold)
	}
}

func TestRetryConfig_Fix(t *testing.T) {
	cfg := RetryConfig{}
	cfg.Fix()

	if cfg.MaxRetries != 3 {
		t.Errorf("expected 3, got %d", cfg.MaxRetries)
	}
	if cfg.WaitBetween != 1*time.Second {
		t.Errorf("expected 1s, got %s", cfg.WaitBetween)
	}
	if cfg.JitterFraction != 0.1 {
		t.Errorf("expected 0.1, got %f", cfg.JitterFraction)
	}
}
