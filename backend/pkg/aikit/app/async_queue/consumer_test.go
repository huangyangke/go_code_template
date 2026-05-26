package async_queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
)

// setupMiniredis creates a miniredis server and a redis.Client connected to it.
func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		rdb.Close()
		mr.Close()
	})
	return mr, rdb
}

// TestRecoverPending_SkippedEntriesAdvanceCursor verifies that recoverPending
// does not loop infinitely when all pending entries have idle < MinIdle.
// Previously the `start` cursor was only updated for eligible entries,
// causing the same batch to be returned forever.
func TestRecoverPending_SkippedEntriesAdvanceCursor(t *testing.T) {
	mr, rdb := setupMiniredis(t)
	ctx := context.Background()

	streamKey := "test:stream"
	groupName := "test-group"
	consumerName := "test-consumer"

	// Create stream and group
	require.NoError(t, rdb.XGroupCreateMkStream(ctx, streamKey, groupName, "0").Err())

	// Add 101 messages and let them be pending (read by consumer but not acked).
	// We need >= 100 so that len(pendingInfo) < 100 does NOT trigger the break,
	// which would mask the infinite loop bug.
	for i := 0; i < 101; i++ {
		_, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			Values: map[string]any{
				"task_id":  fmt.Sprintf("t%d", i),
				"endpoint": "/ep",
				"priority": "5",
				"params":   "{}",
			},
		}).Result()
		require.NoError(t, err)
	}

	// Read all messages so they become pending (but don't ack)
	_, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    groupName,
		Consumer: consumerName,
		Streams:  []string{streamKey, ">"},
		Count:    200,
	}).Result()
	require.NoError(t, err)

	// The message is now in the PEL with very low idle time (< MinIdle).
	// Create a consumer with MinIdle=5 minutes so the entry is always skipped.

	c := &Consumer{
		rdb:               rdb,
		client:            rdb,
		cfg:               RedisConfig{StreamKey: streamKey},
		namespace:         "test",
		groupName:         groupName,
		consumerName:      consumerName,
		pel:               PelConfig{MinIdle: 5 * time.Minute, MaxRetries: 3, ScanOnStartupOnly: true},
		endpoints:         map[string]EndpointConfig{"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }}},
		statusStore:       NewStatusStore(rdb, "test"),
		limiter:           &NoopConcurrencyLimiter{},
		activeMsgIDs:      make(map[string]struct{}),
		pendingByEndpoint: make(map[string]*EndpointPendingQueue),
		taskCancelFuncs:   make(map[string]context.CancelFunc),
		scheduler:         defaultSchedulerConfig(),
	}
	c.sem = semaphore.NewWeighted(int64(c.scheduler.WorkerCapacity))

	// Fast-forward miniredis time so the message is old enough to appear in XPENDING
	// but still below our MinIdle threshold. We need the message to appear in the
	// pending list (idle > 0) but be skipped (idle < MinIdle).
	// miniredis starts at 0 idle, so we advance time by 1 second.
	mr.FastForward(1 * time.Second)

	// Run recoverPending with a timeout to guard against infinite loop.
	done := make(chan error, 1)
	go func() {
		done <- c.recoverPending(ctx, consumerName)
	}()

	select {
	case err := <-done:
		// Must complete without hanging
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("recoverPending hung — likely infinite loop due to cursor not advancing on skipped entries")
	}

	// Verify at least some messages are still pending (not claimed, since idle < MinIdle)
	pending, err := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: streamKey,
		Group:  groupName,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	require.NoError(t, err)
	assert.NotEmpty(t, pending, "messages should still be in PEL since idle < MinIdle")
}

// TestRecoverPending_ClaimsEligibleEntries verifies that entries with
// idle >= MinIdle are correctly claimed and processed.
func TestRecoverPending_ClaimsEligibleEntries(t *testing.T) {
	mr, rdb := setupMiniredis(t)
	ctx := context.Background()

	streamKey := "test:stream"
	groupName := "test-group"
	consumerName := "test-consumer"

	require.NoError(t, rdb.XGroupCreateMkStream(ctx, streamKey, groupName, "0").Err())

	// Add a message
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]any{"task_id": "t1", "endpoint": "/ep", "priority": "5", "params": "{}"},
	}).Result()
	require.NoError(t, err)

	// Read the message so it becomes pending
	_, err = rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    groupName,
		Consumer: "other-consumer",
		Streams:  []string{streamKey, ">"},
		Count:    1,
	}).Result()
	require.NoError(t, err)

	// Fast-forward so the entry is idle long enough
	mr.FastForward(10 * time.Minute)

	c := &Consumer{
		rdb:               rdb,
		client:            rdb,
		cfg:               RedisConfig{StreamKey: streamKey},
		namespace:         "test",
		groupName:         groupName,
		consumerName:      consumerName,
		pel:               PelConfig{MinIdle: 5 * time.Minute, MaxRetries: 3, ScanOnStartupOnly: true},
		endpoints:         map[string]EndpointConfig{"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }}},
		statusStore:       NewStatusStore(rdb, "test"),
		limiter:           &NoopConcurrencyLimiter{},
		activeMsgIDs:      make(map[string]struct{}),
		pendingByEndpoint: make(map[string]*EndpointPendingQueue),
		taskCancelFuncs:   make(map[string]context.CancelFunc),
		scheduler:         defaultSchedulerConfig(),
	}
	c.sem = semaphore.NewWeighted(int64(c.scheduler.WorkerCapacity))

	done := make(chan error, 1)
	go func() {
		done <- c.recoverPending(ctx, consumerName)
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("recoverPending timed out")
	}
}

// TestNewConsumer_ZeroWorkerCapacity_DoesNotDeadlock verifies that
// WithScheduler(SchedulerConfig{}) leaves WorkerCapacity at a safe default
// instead of creating semaphore.NewWeighted(0) which blocks forever.
func TestNewConsumer_ZeroWorkerCapacity_DoesNotDeadlock(t *testing.T) {
	_, rdb := setupMiniredis(t)

	c := NewConsumer(
		rdb, RedisConfig{StreamKey: "test:stream"},
		map[string]EndpointConfig{},
		"test",
		WithScheduler(SchedulerConfig{}), // zero capacity
	)

	assert.Equal(t, DefaultWorkerCapacity, c.scheduler.WorkerCapacity,
		"zero WorkerCapacity should default to %d, got %d",
		DefaultWorkerCapacity, c.scheduler.WorkerCapacity)
	assert.NotNil(t, c.sem, "semaphore must be created")
}

// TestConsumer_Stop_WaitsForHandlerDrain verifies that Stop waits for
// in-flight handlers to finish before returning.
func TestConsumer_Stop_WaitsForHandlerDrain(t *testing.T) {
	_, rdb := setupMiniredis(t)

	handlerStarted := make(chan struct{})
	letHandlerFinish := make(chan struct{})
	handlerDone := make(chan struct{})

	c := NewConsumer(
		rdb, RedisConfig{StreamKey: "test:stream"},
		map[string]EndpointConfig{
			"/ep": {
				Handler: func(ctx Context) (any, error) {
					close(handlerStarted)
					<-letHandlerFinish // block until test signals
					close(handlerDone)
					return nil, nil
				},
			},
		},
		"test",
	)

	c.spawnHandleTask(context.Background(), "msg-1", map[string]any{
		"task_id":  "t1",
		"endpoint": "/ep",
		"priority": 5,
	})

	<-handlerStarted

	stopDone := make(chan struct{})
	go func() {
		c.Stop()
		close(stopDone)
	}()

	// Stop must NOT return while the handler is still running.
	select {
	case <-stopDone:
		t.Fatal("Stop returned while handler was still blocked — WaitGroup drain broken")
	case <-time.After(50 * time.Millisecond):
		// Good - Stop is waiting
	}

	close(letHandlerFinish)

	// Now Stop should complete soon after handler exits.
	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not complete after handler finished")
	}

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not finish")
	}
}
