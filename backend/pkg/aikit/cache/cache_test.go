package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_LocalOnly_FreeCache(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-freecache",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, cache)

	err = cache.Set(ctx, "hello", "world")
	require.NoError(t, err)

	value, err := cache.Get(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, "world", value)

	_ = cache.Close()
}

func TestNew_LocalOnly_TinyLFU(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-tinylfu",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeTinyLFU,
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, cache)

	err = cache.Set(ctx, "hello", "world")
	require.NoError(t, err)

	value, err := cache.Get(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, "world", value)

	_ = cache.Close()
}

func TestNew_RemoteOnly(t *testing.T) {
	t.Skip("Remote test requires Redis instance")
}

func TestNew_EmptyName(t *testing.T) {
	cfg := Config{Name: ""}

	_, err := New(cfg)
	assert.Error(t, err)
}

func TestGetOrLoad(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-getorload",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, cache)
	defer cache.Close()

	loadCount := 0
	fetchFunc := func(ctx context.Context) (interface{}, error) {
		loadCount++
		return "loaded-value", nil
	}

	value, err := cache.GetOrLoad(ctx, "key", fetchFunc)
	require.NoError(t, err)
	assert.Equal(t, "loaded-value", value)

	assert.Equal(t, 1, loadCount)

	value, err = cache.GetOrLoad(ctx, "key", fetchFunc)
	require.NoError(t, err)
	assert.Equal(t, "loaded-value", value)

	assert.Equal(t, 1, loadCount)
}

func TestGetOrLoad_NilValue(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-getorload-nil",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	defer cache.Close()

	fetchFunc := func(ctx context.Context) (interface{}, error) {
		return nil, nil
	}

	value, err := cache.GetOrLoad(ctx, "nil-key", fetchFunc)
	require.NoError(t, err)
	// jetcache-go returns nil for nil values
	assert.Nil(t, value)
}

func TestSetAndGet(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-setget",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, cache)
	defer cache.Close()

	testCases := []struct {
		key   string
		value interface{}
	}{
		{"simple-string", "test value"},
		{"int-value", 42},
		{"bool-value", true},
		{"map-value", map[string]interface{}{"key": "value", "count": 10}},
	}

	for _, tc := range testCases {
		t.Run(tc.key, func(t *testing.T) {
			err := cache.Set(ctx, tc.key, tc.value)
			require.NoError(t, err)

			value, err := cache.Get(ctx, tc.key)
			require.NoError(t, err)
			assert.NotNil(t, value)
		})
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-delete",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, cache)
	defer cache.Close()

	err = cache.Set(ctx, "test-key", "test-value")
	require.NoError(t, err)

	value, err := cache.Get(ctx, "test-key")
	require.NoError(t, err)
	assert.Equal(t, "test-value", value)

	err = cache.Delete(ctx, "test-key")
	require.NoError(t, err)

	value, err = cache.Get(ctx, "test-key")
	require.NoError(t, err)
	assert.Nil(t, value)
}

func TestClear_RemovesLocalEntries(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-clear",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	defer cache.Close()

	err = cache.Set(ctx, "clear-key", "clear-value")
	require.NoError(t, err)

	value, err := cache.Get(ctx, "clear-key")
	require.NoError(t, err)
	assert.Equal(t, "clear-value", value)

	err = cache.Clear(ctx)
	require.NoError(t, err)

	value, err = cache.Get(ctx, "clear-key")
	require.NoError(t, err)
	assert.Nil(t, value)
}

func TestExists(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-exists",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, cache)
	defer cache.Close()

	err = cache.Set(ctx, "existing-key", "test-value")
	require.NoError(t, err)

	exists := cache.Exists(ctx, "existing-key")
	assert.True(t, exists)

	exists = cache.Exists(ctx, "non-existent-key")
	assert.False(t, exists)
}

func TestLocalCacheExpiration(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-expiration",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      1,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, cache)
	defer cache.Close()

	err = cache.Set(ctx, "expiring-key", "will expire")
	require.NoError(t, err)

	value, err := cache.Get(ctx, "expiring-key")
	require.NoError(t, err)
	assert.Equal(t, "will expire", value)

	time.Sleep(2 * time.Second)

	value, err = cache.Get(ctx, "expiring-key")
	require.NoError(t, err)
	assert.Nil(t, value)
}

func TestGetCache(t *testing.T) {
	t.Skip("Integration test with registry")
}

func TestMetrics(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-metrics",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, cache)
	defer cache.Close()

	_, err = cache.Get(ctx, "non-existent-key")
	require.NoError(t, err)

	err = cache.Set(ctx, "existent-key", "value")
	require.NoError(t, err)

	_, err = cache.Get(ctx, "existent-key")
	require.NoError(t, err)
}

func TestMsgpackCodec(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-msgpack",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
		Codec:         "msgpack",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	defer cache.Close()

	err = cache.Set(ctx, "key", "msgpack-value")
	require.NoError(t, err)

	value, err := cache.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "msgpack-value", value)
}

func TestJSONCodec(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Family:        "test-family",
		Name:          "test-json-codec",
		CacheType:     CacheTypeLocal,
		LocalItemSize: 100,
		LocalTTL:      60,
		LocalType:     LocalCacheTypeFreeCache,
		LocalMemSize:  "1MB",
		Codec:         "json",
	}

	cache, err := New(cfg)
	require.NoError(t, err)
	defer cache.Close()

	err = cache.Set(ctx, "key", "json-value")
	require.NoError(t, err)

	value, err := cache.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "json-value", value)
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"256MB", 256 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"512KB", 512 * 1024},
		{"100B", 100},
		{"0", 0},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseSize(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, int64(got))
		})
	}
}

func TestParseSize_Invalid(t *testing.T) {
	_, err := ParseSize("")
	assert.Error(t, err)

	_, err = ParseSize("abc")
	assert.Error(t, err)

	_, err = ParseSize("100")
	assert.Error(t, err) // missing unit
}
