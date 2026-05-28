package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbredis "github.com/huangyangke/go-aikit/database/redis"
)

func newTestRedis(t *testing.T) (*dbredis.Redis, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	r := dbredis.New(&dbredis.Config{
		Name:  "test",
		Type:  dbredis.StandaloneType,
		Addrs: []string{mr.Addr()},
	})
	return r, mr
}

func TestLock_TryLock_Success(t *testing.T) {
	r, _ := newTestRedis(t)
	lock := r.NewLock(context.Background(), "test:lock", 5*time.Second)

	ok, err := lock.TryLock()
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestLock_TryLock_AlreadyHeld(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	lock1 := r.NewLock(ctx, "test:lock", 5*time.Second)
	lock2 := r.NewLock(ctx, "test:lock", 5*time.Second)

	ok, err := lock1.TryLock()
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = lock2.TryLock()
	require.NoError(t, err)
	assert.False(t, ok, "second lock should fail when key is held")
}

func TestLock_Unlock_Success(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	lock := r.NewLock(ctx, "test:lock", 5*time.Second)
	ok, err := lock.TryLock()
	require.NoError(t, err)
	require.True(t, ok)

	released, err := lock.Unlock()
	require.NoError(t, err)
	assert.True(t, released)
}

func TestLock_Unlock_NotOwner(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	lock1 := r.NewLock(ctx, "test:lock", 5*time.Second)
	lock2 := r.NewLock(ctx, "test:lock", 5*time.Second)

	ok, _ := lock1.TryLock()
	require.True(t, ok)

	// lock2 does not own the key, should not release it
	released, err := lock2.Unlock()
	require.NoError(t, err)
	assert.False(t, released, "non-owner should not be able to unlock")
}

func TestLock_TryLock_AfterUnlock(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	lock := r.NewLock(ctx, "test:lock", 5*time.Second)
	ok, _ := lock.TryLock()
	require.True(t, ok)
	_, _ = lock.Unlock()

	lock2 := r.NewLock(ctx, "test:lock", 5*time.Second)
	ok, err := lock2.TryLock()
	require.NoError(t, err)
	assert.True(t, ok, "should be acquirable after unlock")
}

func TestLock_Refresh(t *testing.T) {
	r, mr := newTestRedis(t)
	ctx := context.Background()

	lock := r.NewLock(ctx, "test:lock", 200*time.Millisecond)
	ok, err := lock.TryLock()
	require.NoError(t, err)
	require.True(t, ok)

	mr.FastForward(100 * time.Millisecond)
	err = lock.Refresh()
	require.NoError(t, err)

	// After refresh, TTL should be reset; key must still exist
	mr.FastForward(150 * time.Millisecond)
	exists := mr.Exists("test:lock")
	assert.True(t, exists, "key should still exist after refresh extended TTL")
}

func TestLock_Watchdog_KeepsLockAlive(t *testing.T) {
	r, mr := newTestRedis(t)
	ctx := context.Background()

	// Short expire; watchdog should renew before it expires
	lock := r.NewLock(ctx, "test:watchdog", 300*time.Millisecond, dbredis.WithWatchdog(ctx))
	ok, err := lock.TryLock()
	require.NoError(t, err)
	require.True(t, ok)

	// Fast-forward past original TTL in small steps to allow watchdog ticks
	for i := 0; i < 5; i++ {
		time.Sleep(80 * time.Millisecond)
		mr.FastForward(80 * time.Millisecond)
	}
	// Total elapsed: ~400ms > 300ms expire, but watchdog should have refreshed
	exists := mr.Exists("test:watchdog")
	assert.True(t, exists, "watchdog should keep the lock alive past its original TTL")

	released, err := lock.Unlock()
	require.NoError(t, err)
	assert.True(t, released)
}

func TestLock_Watchdog_StopsOnUnlock(t *testing.T) {
	r, mr := newTestRedis(t)
	ctx := context.Background()

	lock := r.NewLock(ctx, "test:watchdog:stop", 300*time.Millisecond, dbredis.WithWatchdog(ctx))
	ok, err := lock.TryLock()
	require.NoError(t, err)
	require.True(t, ok)

	// Unlock stops watchdog
	released, err := lock.Unlock()
	require.NoError(t, err)
	require.True(t, released)

	// Key is deleted, and won't reappear because watchdog stopped
	time.Sleep(50 * time.Millisecond)
	mr.FastForward(50 * time.Millisecond)
	exists := mr.Exists("test:watchdog:stop")
	assert.False(t, exists, "key should not exist after unlock")
}

func TestLock_Watchdog_StopsOnContextCancel(t *testing.T) {
	r, mr := newTestRedis(t)
	ctx, cancel := context.WithCancel(context.Background())

	lock := r.NewLock(ctx, "test:watchdog:ctx", 300*time.Millisecond, dbredis.WithWatchdog(ctx))
	ok, err := lock.TryLock()
	require.NoError(t, err)
	require.True(t, ok)

	// Cancel external context — watchdog should stop
	cancel()
	time.Sleep(50 * time.Millisecond)

	// Fast-forward past expire — lock should expire now
	mr.FastForward(350 * time.Millisecond)
	exists := mr.Exists("test:watchdog:ctx")
	assert.False(t, exists, "lock should expire after watchdog context cancelled")
}
