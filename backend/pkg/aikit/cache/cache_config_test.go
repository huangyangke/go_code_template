package cache

import "testing"

func TestConfig_Fix(t *testing.T) {
	c := &Config{Name: "test", Family: "svc"}
	c.Fix()

	if c.CacheType != CacheTypeBoth {
		t.Errorf("CacheType = %q, want %q", c.CacheType, CacheTypeBoth)
	}
	if c.LocalType != LocalCacheTypeFreeCache {
		t.Errorf("LocalType = %q, want %q", c.LocalType, LocalCacheTypeFreeCache)
	}
	if c.LocalTTL != 60 {
		t.Errorf("LocalTTL = %d, want 60", c.LocalTTL)
	}
	if c.RemoteTTL != 3600 {
		t.Errorf("RemoteTTL = %d, want 3600", c.RemoteTTL)
	}
	if c.NullValueTTL != 60 {
		t.Errorf("NullValueTTL = %d, want 60", c.NullValueTTL)
	}
	if c.LocalMemSize != "256MB" {
		t.Errorf("LocalMemSize = %q, want %q", c.LocalMemSize, "256MB")
	}
	if c.LocalItemSize != 1000 {
		t.Errorf("LocalItemSize = %d, want 1000", c.LocalItemSize)
	}
	if c.Codec != "msgpack" {
		t.Errorf("Codec = %q, want %q", c.Codec, "msgpack")
	}
	if c.RefreshConcurrency != 4 {
		t.Errorf("RefreshConcurrency = %d, want 4", c.RefreshConcurrency)
	}
	if c.EventChBufSize != 100 {
		t.Errorf("EventChBufSize = %d, want 100", c.EventChBufSize)
	}
}

func TestConfig_Fix_NoOverride(t *testing.T) {
	c := &Config{
		Name:               "test",
		CacheType:          CacheTypeLocal,
		LocalType:          LocalCacheTypeTinyLFU,
		LocalTTL:           30,
		RemoteTTL:          1800,
		NullValueTTL:       10,
		LocalMemSize:       "512MB",
		LocalItemSize:      2000,
		Codec:              "json",
		RefreshDuration:    60,
		RefreshConcurrency: 8,
	}
	c.Fix()

	if c.CacheType != CacheTypeLocal {
		t.Errorf("CacheType = %q, want %q", c.CacheType, CacheTypeLocal)
	}
	if c.LocalItemSize != 2000 {
		t.Errorf("LocalItemSize = %d, want 2000", c.LocalItemSize)
	}
	if c.LocalMemSize != "512MB" {
		t.Errorf("LocalMemSize = %q, want %q", c.LocalMemSize, "512MB")
	}
	if c.Codec != "json" {
		t.Errorf("Codec = %q, want %q", c.Codec, "json")
	}
	if c.RefreshDuration != 60 {
		t.Errorf("RefreshDuration = %d, want 60", c.RefreshDuration)
	}
	if c.RefreshConcurrency != 8 {
		t.Errorf("RefreshConcurrency = %d, want 8", c.RefreshConcurrency)
	}
}

func TestConfig_Validate_MissingName(t *testing.T) {
	c := &Config{}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for empty Name")
	}
}

func TestConfig_Validate_SyncLocalRequiresBoth(t *testing.T) {
	c := &Config{Name: "test", SyncLocal: true, CacheType: CacheTypeLocal}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for SyncLocal with non-both cache type")
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	c := &Config{Family: "svc", Name: "test"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
