package async_queue

import (
	"context"
	"testing"

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
