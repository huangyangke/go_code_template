package async_queue

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopConcurrencyLimiter(t *testing.T) {
	l := &NoopConcurrencyLimiter{}
	ctx := context.Background()

	ok, err := l.TryAcquire(ctx, "any-endpoint")
	assert.True(t, ok)
	assert.NoError(t, err)

	assert.NoError(t, l.Release(ctx, "any-endpoint"))
}

func TestLocalConcurrencyLimiter_Basic(t *testing.T) {
	limits := map[string]int{"ep1": 2}
	l := NewLocalConcurrencyLimiter(limits, 1)
	ctx := context.Background()

	ok, err := l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestLocalConcurrencyLimiter_Release(t *testing.T) {
	limits := map[string]int{"ep1": 1}
	l := NewLocalConcurrencyLimiter(limits, 1)
	ctx := context.Background()

	ok, _ := l.TryAcquire(ctx, "ep1")
	assert.True(t, ok)

	ok, _ = l.TryAcquire(ctx, "ep1")
	assert.False(t, ok)

	require.NoError(t, l.Release(ctx, "ep1"))

	ok, _ = l.TryAcquire(ctx, "ep1")
	assert.True(t, ok)
}

func TestLocalConcurrencyLimiter_DefaultLimit(t *testing.T) {
	l := NewLocalConcurrencyLimiter(nil, 2)
	ctx := context.Background()

	ok, _ := l.TryAcquire(ctx, "unknown")
	assert.True(t, ok)

	ok, _ = l.TryAcquire(ctx, "unknown")
	assert.True(t, ok)

	ok, _ = l.TryAcquire(ctx, "unknown")
	assert.False(t, ok)
}

func TestLocalConcurrencyLimiter_ZeroLimit(t *testing.T) {
	limits := map[string]int{"unlimited": 0}
	l := NewLocalConcurrencyLimiter(limits, 1)
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		ok, err := l.TryAcquire(ctx, "unlimited")
		assert.True(t, ok)
		assert.NoError(t, err)
	}
}

func TestLocalConcurrencyLimiter_Release_NeverNegative(t *testing.T) {
	l := NewLocalConcurrencyLimiter(nil, 1)
	ctx := context.Background()

	require.NoError(t, l.Release(ctx, "ep1"))
	require.NoError(t, l.Release(ctx, "ep1"))

	assert.Equal(t, 0, l.counts["ep1"])
}

func TestLocalConcurrencyLimiter_MultipleEndpoints(t *testing.T) {
	limits := map[string]int{"ep1": 1, "ep2": 1}
	l := NewLocalConcurrencyLimiter(limits, 0)
	ctx := context.Background()

	ok1, _ := l.TryAcquire(ctx, "ep1")
	ok2, _ := l.TryAcquire(ctx, "ep2")
	assert.True(t, ok1)
	assert.True(t, ok2)

	blocked1, _ := l.TryAcquire(ctx, "ep1")
	blocked2, _ := l.TryAcquire(ctx, "ep2")
	assert.False(t, blocked1)
	assert.False(t, blocked2)
}

// ================================
// RedisConcurrencyLimiter tests
// ================================

func setupRedisLimiter(t *testing.T, limits map[string]int, defaultLimit int) (*RedisConcurrencyLimiter, func()) {
	t.Helper()
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	limiter := NewRedisConcurrencyLimiter(rdb, limits, defaultLimit, "test:limit", 60)
	return limiter, func() {
		rdb.Close()
		mr.Close()
	}
}

func TestNewRedisConcurrencyLimiter_Creation(t *testing.T) {
	l, cleanup := setupRedisLimiter(t, map[string]int{"ep1": 3}, 1)
	defer cleanup()
	assert.NotNil(t, l)
	assert.Equal(t, "test:limit", l.keyPrefix)
	assert.Equal(t, 1, l.defaultLimit)
}

func TestRedisConcurrencyLimiter_TryAcquire_ZeroLimit_AlwaysTrue(t *testing.T) {
	// defaultLimit=0 means no limit — TryAcquire must always return true
	l, cleanup := setupRedisLimiter(t, nil, 0)
	defer cleanup()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		ok, err := l.TryAcquire(ctx, "ep1")
		require.NoError(t, err)
		assert.True(t, ok)
	}
}

func TestRedisConcurrencyLimiter_TryAcquire_FirstAcquireSucceeds(t *testing.T) {
	l, cleanup := setupRedisLimiter(t, map[string]int{"ep1": 2}, 1)
	defer cleanup()
	ctx := context.Background()

	ok, err := l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRedisConcurrencyLimiter_TryAcquire_AtLimit_ReturnsFalse(t *testing.T) {
	l, cleanup := setupRedisLimiter(t, map[string]int{"ep1": 2}, 1)
	defer cleanup()
	ctx := context.Background()

	ok, err := l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.True(t, ok)

	// third acquire should be rejected (limit=2)
	ok, err = l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRedisConcurrencyLimiter_Release_Decrements(t *testing.T) {
	l, cleanup := setupRedisLimiter(t, map[string]int{"ep1": 1}, 1)
	defer cleanup()
	ctx := context.Background()

	ok, err := l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.True(t, ok)

	// now at limit — second acquire blocked
	ok, err = l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.False(t, ok)

	// release one slot
	require.NoError(t, l.Release(ctx, "ep1"))

	// acquire should succeed again
	ok, err = l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRedisConcurrencyLimiter_Release_NoUnderflow(t *testing.T) {
	l, cleanup := setupRedisLimiter(t, map[string]int{"ep1": 2}, 1)
	defer cleanup()
	ctx := context.Background()

	// Release without any prior acquire — should not go below 0
	require.NoError(t, l.Release(ctx, "ep1"))
	require.NoError(t, l.Release(ctx, "ep1"))

	// After releases on zero, acquiring should still respect the limit
	ok, err := l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRedisConcurrencyLimiter_MultipleEndpoints_IndependentCounts(t *testing.T) {
	l, cleanup := setupRedisLimiter(t, map[string]int{"ep1": 1, "ep2": 1}, 1)
	defer cleanup()
	ctx := context.Background()

	// Saturate ep1
	ok1, err := l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.True(t, ok1)

	// ep2 should be unaffected
	ok2, err := l.TryAcquire(ctx, "ep2")
	require.NoError(t, err)
	assert.True(t, ok2)

	// Both are now at limit
	blocked1, err := l.TryAcquire(ctx, "ep1")
	require.NoError(t, err)
	assert.False(t, blocked1)

	blocked2, err := l.TryAcquire(ctx, "ep2")
	require.NoError(t, err)
	assert.False(t, blocked2)
}

func TestRedisConcurrencyLimiter_TryAcquire_RedisError(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close(); mr.Close() })

	l := NewRedisConcurrencyLimiter(rdb, map[string]int{"/ep": 2}, 1, "test:limit", 60)
	mr.Close() // force error

	_, err := l.TryAcquire(context.Background(), "/ep")
	assert.Error(t, err)
}
