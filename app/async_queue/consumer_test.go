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

	streamKey := buildStreamKey("test")
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
		cfg:               RedisConfig{},
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

	streamKey := buildStreamKey("test")
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
		cfg:               RedisConfig{},
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
		rdb, RedisConfig{},
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
		rdb, RedisConfig{},
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

	// Consumer must be in running state for spawn to execute the handler.
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

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

// ================================
// Option setters
// ================================

func TestWithGroupName_SetsField(t *testing.T) {
	_, rdb := setupMiniredis(t)
	c := NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{}, "fam",
		WithGroupName("my-group"))
	assert.Equal(t, "my-group", c.groupName)
}

func TestWithConsumerName_SetsField(t *testing.T) {
	_, rdb := setupMiniredis(t)
	c := NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{}, "fam",
		WithConsumerName("my-consumer"))
	assert.Equal(t, "my-consumer", c.consumerName)
}

func TestWithPel_SetsField(t *testing.T) {
	_, rdb := setupMiniredis(t)
	pel := PelConfig{MinIdle: 10 * time.Minute, MaxRetries: 5, ScanOnStartupOnly: true}
	c := NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{}, "fam",
		WithPel(pel))
	assert.Equal(t, pel, c.pel)
}

func TestWithLimiter_SetsField(t *testing.T) {
	_, rdb := setupMiniredis(t)
	lim := NewLocalConcurrencyLimiter(nil, 10)
	c := NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{}, "fam",
		WithLimiter(lim))
	assert.Equal(t, lim, c.limiter)
}

func TestWithFeatures_SetsField(t *testing.T) {
	_, rdb := setupMiniredis(t)
	feat := FeatureConfig{EnableStatusStore: true, EnableCancel: false}
	c := NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{}, "fam",
		WithFeatures(feat))
	assert.Equal(t, feat, c.features)
}

// ================================
// NewConsumer panic cases
// ================================

func TestNewConsumer_EmptyFamily_Panics(t *testing.T) {
	_, rdb := setupMiniredis(t)
	assert.Panics(t, func() {
		NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{}, "")
	})
}

func TestNewConsumer_NilHandler_Panics(t *testing.T) {
	_, rdb := setupMiniredis(t)
	assert.Panics(t, func() {
		NewConsumer(rdb, RedisConfig{},
			map[string]EndpointConfig{"/ep": {Handler: nil}}, "fam")
	})
}

// ================================
// createGroup idempotency
// ================================

func TestCreateGroup_Idempotent(t *testing.T) {
	_, rdb := setupMiniredis(t)
	ctx := context.Background()
	c := &Consumer{
		rdb:       rdb,
		cfg:       RedisConfig{},
		groupName: "grp",
	}
	require.NoError(t, c.createGroup(ctx))
	// Second call must not error (BUSYGROUP is swallowed)
	require.NoError(t, c.createGroup(ctx))
}

// ================================
// admitMessage
// ================================

func TestAdmitMessage_L2Pass_GoesDirectlyToHandler(t *testing.T) {
	_, rdb := setupMiniredis(t)

	handled := make(chan string, 1)
	c := NewConsumer(
		rdb, RedisConfig{},
		map[string]EndpointConfig{
			"/ep": {Handler: func(ctx Context) (any, error) {
				handled <- ctx.TaskID()
				return nil, nil
			}},
		},
		"fam",
		WithLimiter(&NoopConcurrencyLimiter{}),
	)
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	ctx := context.Background()
	c.admitMessage(ctx, "msg-1", map[string]any{
		"task_id":  "task-1",
		"endpoint": "/ep",
		"priority": 5,
		"params":   "{}",
	})

	select {
	case tid := <-handled:
		assert.Equal(t, "task-1", tid)
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not called")
	}

	// pending queue must be empty
	c.mu.Lock()
	q := c.pendingByEndpoint["/ep"]
	c.mu.Unlock()
	if q != nil {
		assert.True(t, q.IsEmpty(), "pending queue should be empty when L2 passes")
	}
}

// blockingLimiter rejects TryAcquire always.
type blockingLimiter struct{}

func (b *blockingLimiter) TryAcquire(_ context.Context, _ string) (bool, error) { return false, nil }
func (b *blockingLimiter) Release(_ context.Context, _ string) error            { return nil }

func TestAdmitMessage_L2Reject_GoesToPending(t *testing.T) {
	_, rdb := setupMiniredis(t)

	c := NewConsumer(
		rdb, RedisConfig{},
		map[string]EndpointConfig{
			"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }},
		},
		"fam",
		WithLimiter(&blockingLimiter{}),
	)

	ctx := context.Background()
	c.admitMessage(ctx, "msg-2", map[string]any{
		"task_id":  "task-2",
		"endpoint": "/ep",
		"priority": 5,
		"params":   "{}",
	})

	c.mu.Lock()
	q := c.pendingByEndpoint["/ep"]
	c.mu.Unlock()
	require.NotNil(t, q)
	assert.Equal(t, 1, q.Len(), "rejected message should be in pending queue")
}

// ================================
// drainPending
// ================================

func TestDrainPending_DispatchesAndRemovesFromQueue(t *testing.T) {
	_, rdb := setupMiniredis(t)

	handled := make(chan string, 1)
	c := NewConsumer(
		rdb, RedisConfig{},
		map[string]EndpointConfig{
			"/ep": {Handler: func(ctx Context) (any, error) {
				handled <- ctx.TaskID()
				return nil, nil
			}},
		},
		"fam",
		WithLimiter(&NoopConcurrencyLimiter{}),
	)
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	// Manually insert into pending queue
	c.mu.Lock()
	q := &EndpointPendingQueue{}
	q.Push(5, "msg-3", map[string]any{
		"task_id":  "task-3",
		"endpoint": "/ep",
		"priority": 5,
		"params":   "{}",
	})
	c.pendingByEndpoint["/ep"] = q
	c.pendingOrder = []string{"/ep"}
	c.mu.Unlock()

	scheduled := c.drainPending(context.Background())
	assert.Equal(t, 1, scheduled)

	// Queue should now be empty
	c.mu.Lock()
	q2 := c.pendingByEndpoint["/ep"]
	c.mu.Unlock()
	if q2 != nil {
		assert.True(t, q2.IsEmpty())
	}

	select {
	case tid := <-handled:
		assert.Equal(t, "task-3", tid)
	case <-time.After(2 * time.Second):
		t.Fatal("drained task was not handled")
	}
}

// ================================
// runHeartbeat
// ================================

func TestRunHeartbeat_KeyExistsAndDeletedAfterCancel(t *testing.T) {
	_, rdb := setupMiniredis(t)
	ctx, cancel := context.WithCancel(context.Background())

	consumerName := "hb-consumer"
	// MinIdle = 6s → ttl = min(3s, 30s) = 3s → clamped to 5s → ticker every ~1.67s.
	// Use Eventually with 4s timeout to catch the first tick in real time.
	c := &Consumer{
		rdb:       rdb,
		namespace: "fam",
		groupName: "grp",
		pel:       PelConfig{MinIdle: 6 * time.Second},
	}

	done := make(chan struct{})
	go func() {
		c.runHeartbeat(ctx, consumerName)
		close(done)
	}()

	key := buildHeartbeatKey("fam", "grp", consumerName)

	// Poll until key appears (waits for the ticker to fire in real time)
	require.Eventually(t, func() bool {
		n, err := rdb.Exists(context.Background(), key).Result()
		return err == nil && n > 0
	}, 4*time.Second, 100*time.Millisecond, "heartbeat key should exist after first tick")

	cancel()

	// After cancel, goroutine should delete the key
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runHeartbeat did not exit after cancel")
	}

	n, err := rdb.Exists(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), n, "heartbeat key should be deleted after cancel")
}

// ================================
// runCancelSubscriber
// ================================

func TestRunCancelSubscriber_CancelsFnOnMessage(t *testing.T) {
	_, rdb := setupMiniredis(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cancelled := make(chan struct{})
	c := &Consumer{
		rdb:       rdb,
		client:    rdb,
		namespace: "fam",
		taskCancelFuncs: map[string]context.CancelFunc{
			"task-x": func() { close(cancelled) },
		},
	}

	go c.runCancelSubscriber(ctx)

	// Give subscriber time to subscribe
	time.Sleep(50 * time.Millisecond)

	// Publish cancel message
	require.NoError(t, rdb.Publish(ctx, buildCancelChannel("fam"), "task-x").Err())

	select {
	case <-cancelled:
		// pass
	case <-time.After(2 * time.Second):
		t.Fatal("cancel function was not called after Pub/Sub message")
	}
}

// ================================
// runningCount
// ================================

func TestRunningCount_ReflectsActiveMsgIDs(t *testing.T) {
	_, rdb := setupMiniredis(t)
	c := NewConsumer(rdb, RedisConfig{},
		map[string]EndpointConfig{}, "fam")

	assert.Equal(t, int64(0), c.runningCount())

	c.mu.Lock()
	c.activeMsgIDs["msg-a"] = struct{}{}
	c.activeMsgIDs["msg-b"] = struct{}{}
	c.mu.Unlock()

	assert.Equal(t, int64(2), c.runningCount())
}

// ================================
// clampPriority
// ================================

func TestClampPriority(t *testing.T) {
	assert.Equal(t, 0, clampPriority(-1))
	assert.Equal(t, 9, clampPriority(10))
	assert.Equal(t, 5, clampPriority(5))
	assert.Equal(t, 0, clampPriority(0))
	assert.Equal(t, 9, clampPriority(9))
}

// ================================
// toStringMap
// ================================

func TestToStringMap_CopiesCorrectly(t *testing.T) {
	src := map[string]interface{}{"a": "1", "b": 2}
	dst := toStringMap(src)
	assert.Equal(t, src["a"], dst["a"])
	assert.Equal(t, src["b"], dst["b"])
	assert.Len(t, dst, 2)
	// Ensure it's a copy (modifying dst does not affect src)
	dst["c"] = "new"
	assert.Nil(t, src["c"])
}

// ================================
// handleTask outcomes
// ================================

func newTestConsumer(t *testing.T, rdb *redis.Client, handler HandlerFunc) *Consumer {
	t.Helper()
	c := NewConsumer(
		rdb, RedisConfig{},
		map[string]EndpointConfig{
			"/ep": {Handler: handler},
		},
		"fam",
	)
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()
	return c
}

func TestHandleTask_Success_MarksSuccess(t *testing.T) {
	_, rdb := setupMiniredis(t)
	ctx := context.Background()

	c := newTestConsumer(t, rdb, func(ctx Context) (any, error) {
		return "done", nil
	})

	// Seed stream + group so XAck/XDel don't fail
	require.NoError(t, rdb.XGroupCreateMkStream(ctx, "test:stream", c.groupName, "0").Err())
	msgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "test:stream",
		Values: map[string]any{"task_id": "t1", "endpoint": "/ep", "priority": "5", "params": "{}"},
	}).Result()
	require.NoError(t, err)

	require.NoError(t, c.statusStore.InitQueued(ctx, "t1", "/ep", 5))

	done := make(chan struct{})
	go func() {
		c.handleTask(ctx, msgID, map[string]any{
			"task_id":  "t1",
			"endpoint": "/ep",
			"priority": 5,
			"params":   "{}",
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handleTask did not complete")
	}

	ts, err := c.statusStore.Get(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, TaskStatusSuccess, ts.Status)
}

func TestHandleTask_Failed_MarksFailure(t *testing.T) {
	_, rdb := setupMiniredis(t)
	ctx := context.Background()

	c := newTestConsumer(t, rdb, func(ctx Context) (any, error) {
		return nil, fmt.Errorf("boom")
	})

	require.NoError(t, rdb.XGroupCreateMkStream(ctx, "test:stream", c.groupName, "0").Err())
	msgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "test:stream",
		Values: map[string]any{"task_id": "t2", "endpoint": "/ep", "priority": "5", "params": "{}"},
	}).Result()
	require.NoError(t, err)

	require.NoError(t, c.statusStore.InitQueued(ctx, "t2", "/ep", 5))

	done := make(chan struct{})
	go func() {
		c.handleTask(ctx, msgID, map[string]any{
			"task_id":  "t2",
			"endpoint": "/ep",
			"priority": 5,
			"params":   "{}",
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handleTask did not complete")
	}

	ts, err := c.statusStore.Get(ctx, "t2")
	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, TaskStatusFailed, ts.Status)
}

func TestHandleTask_CancelledBeforeRun(t *testing.T) {
	_, rdb := setupMiniredis(t)
	ctx := context.Background()

	// Handler should NOT be called
	handlerCalled := false
	c := newTestConsumer(t, rdb, func(ctx Context) (any, error) {
		handlerCalled = true
		return nil, nil
	})

	require.NoError(t, rdb.XGroupCreateMkStream(ctx, "test:stream", c.groupName, "0").Err())
	msgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "test:stream",
		Values: map[string]any{"task_id": "t3", "endpoint": "/ep", "priority": "5", "params": "{}"},
	}).Result()
	require.NoError(t, err)

	require.NoError(t, c.statusStore.InitQueued(ctx, "t3", "/ep", 5))

	// Pre-set cancel key
	cancelKey := buildCancelKey("fam", "t3")
	require.NoError(t, rdb.Set(ctx, cancelKey, "1", time.Minute).Err())

	done := make(chan struct{})
	go func() {
		c.handleTask(ctx, msgID, map[string]any{
			"task_id":  "t3",
			"endpoint": "/ep",
			"priority": 5,
			"params":   "{}",
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handleTask did not complete")
	}

	assert.False(t, handlerCalled, "handler must not be called when cancelled before run")

	ts, err := c.statusStore.Get(ctx, "t3")
	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, TaskStatusCancelled, ts.Status)
}

// ── NewConsumer validation panics ─────────────────────────────────────────────

func TestNewConsumer_InvalidSchedulerConfig_Panics(t *testing.T) {
	_, rdb := setupMiniredis(t)
	// DefaultTimeout < 0 triggers ValidateSchedulerConfig panic
	// (WorkerCapacity <= 0 is auto-corrected, so we must use DefaultTimeout to trigger it)
	assert.Panics(t, func() {
		NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{
			"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }},
		}, "ns", WithScheduler(SchedulerConfig{WorkerCapacity: 10, DefaultTimeout: -1 * time.Second}))
	})
}

func TestNewConsumer_InvalidPelConfig_Panics(t *testing.T) {
	_, rdb := setupMiniredis(t)
	assert.Panics(t, func() {
		NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{
			"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }},
		}, "ns", WithPel(PelConfig{MinIdle: 0, MaxRetries: 3}))
	})
}

func TestNewConsumer_ValidPelConfig(t *testing.T) {
	_, rdb := setupMiniredis(t)
	c := NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{},
		"ns", WithPel(PelConfig{MinIdle: 5 * time.Minute, MaxRetries: 5}))
	assert.Equal(t, 5, c.pel.MaxRetries)
}

func TestNewConsumer_WithLimiter(t *testing.T) {
	_, rdb := setupMiniredis(t)
	lim := NewLocalConcurrencyLimiter(nil, 3)
	c := NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{},
		"ns", WithLimiter(lim))
	assert.Equal(t, lim, c.limiter)
}

func TestNewConsumer_WithFeatures(t *testing.T) {
	_, rdb := setupMiniredis(t)
	feat := ResolveFeatureMode(FeatureModeLite, nil)
	c := NewConsumer(rdb, RedisConfig{}, map[string]EndpointConfig{},
		"ns", WithFeatures(feat))
	assert.False(t, c.features.EnableStatusStore)
}

// ── createGroup error path ────────────────────────────────────────────────────

func TestCreateGroup_ErrorOnInvalidStream(t *testing.T) {
	mr, rdb := setupMiniredis(t)
	ctx := context.Background()

	// Set the stream key to a non-stream type to force an error
	require.NoError(t, rdb.Set(ctx, buildStreamKey("bad-ns"), "notastream", 0).Err())

	c := &Consumer{
		rdb:       rdb,
		cfg:       RedisConfig{},
		namespace: "bad-ns",
		groupName: "g",
	}
	err := c.createGroup(ctx)
	assert.Error(t, err)
	_ = mr
}

// ── Stop drain timeout ────────────────────────────────────────────────────────

func TestConsumer_Stop_DrainTimeout(t *testing.T) {
	_, rdb := setupMiniredis(t)

	c := NewConsumer(rdb, RedisConfig{},
		map[string]EndpointConfig{"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }}},
		"ns")

	// Reduce stop timeout so test doesn't take 30s
	origTimeout := DefaultStopTimeout
	DefaultStopTimeout = 100 * time.Millisecond
	defer func() { DefaultStopTimeout = origTimeout }()

	// Manually add a wg item that never finishes
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()
	c.wg.Add(1) // never Done — simulates stuck handler

	done := make(chan struct{})
	go func() {
		c.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop returned after timeout — correct
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Stop did not return after drain timeout")
	}
	c.wg.Done() // cleanup
}

// ── handleTask: timeout + RetryOnTimeout ─────────────────────────────────────

func TestHandleTask_Timeout_RetryOnTimeout(t *testing.T) {
	_, rdb := setupMiniredis(t)
	ctx := context.Background()

	// Seed stream + group
	streamKey := buildStreamKey("ns-timeout")
	require.NoError(t, rdb.XGroupCreateMkStream(ctx, streamKey, "g", "0").Err())
	msgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]any{"task_id": "tto", "endpoint": "/ep", "priority": "5", "params": "{}"},
	}).Result()
	require.NoError(t, err)

	statusStore := NewStatusStore(rdb, "ns-timeout")
	require.NoError(t, statusStore.InitQueued(ctx, "tto", "/ep", 5))

	handlerStarted := make(chan struct{})
	c := &Consumer{
		rdb:    rdb,
		client: rdb,
		cfg:    RedisConfig{},
		endpoints: map[string]EndpointConfig{
			"/ep": {
				Handler: func(ctx Context) (any, error) {
					close(handlerStarted)
					// Block until context expires (timeout)
					<-ctx.Done()
					return nil, ctx.Err()
				},
				Timeout:        50 * time.Millisecond,
				RetryOnTimeout: true,
			},
		},
		namespace:         "ns-timeout",
		groupName:         "g",
		statusStore:       statusStore,
		limiter:           &NoopConcurrencyLimiter{},
		activeMsgIDs:      map[string]struct{}{msgID: {}},
		pendingByEndpoint: make(map[string]*EndpointPendingQueue),
		taskCancelFuncs:   make(map[string]context.CancelFunc),
		scheduler:         defaultSchedulerConfig(),
		features:          ResolveFeatureMode(FeatureModeFull, nil),
	}
	c.sem = semaphore.NewWeighted(int64(c.scheduler.WorkerCapacity))
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	done := make(chan struct{})
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.handleTask(ctx, msgID, map[string]any{
			"task_id": "tto", "endpoint": "/ep", "priority": 5, "params": "{}",
		})
		close(done)
	}()

	<-handlerStarted
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleTask did not finish after timeout")
	}

	// Status should be re-queued (not failed)
	ts, err := statusStore.Get(ctx, "tto")
	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, TaskStatusQueued, ts.Status)
}

// ── handleTask: with callback ─────────────────────────────────────────────────

func TestHandleTask_CallBack_OnSuccess(t *testing.T) {
	_, rdb := setupMiniredis(t)
	ctx := context.Background()

	streamKey := buildStreamKey("ns-cb")
	require.NoError(t, rdb.XGroupCreateMkStream(ctx, streamKey, "g", "0").Err())
	msgID, _ := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]any{"task_id": "tcb", "endpoint": "/ep", "priority": "5", "params": "{}"},
	}).Result()

	statusStore := NewStatusStore(rdb, "ns-cb")
	require.NoError(t, statusStore.InitQueued(ctx, "tcb", "/ep", 5))

	callbackCalled := make(chan *TaskResponse, 1)
	c := &Consumer{
		rdb:    rdb,
		client: rdb,
		cfg:    RedisConfig{},
		endpoints: map[string]EndpointConfig{
			"/ep": {
				Handler: func(ctx Context) (any, error) { return "done", nil },
				CallBack: func(resp *TaskResponse) error {
					callbackCalled <- resp
					return nil
				},
			},
		},
		namespace:         "ns-cb",
		groupName:         "g",
		statusStore:       statusStore,
		limiter:           &NoopConcurrencyLimiter{},
		activeMsgIDs:      map[string]struct{}{msgID: {}},
		pendingByEndpoint: make(map[string]*EndpointPendingQueue),
		taskCancelFuncs:   make(map[string]context.CancelFunc),
		scheduler:         defaultSchedulerConfig(),
		features:          ResolveFeatureMode(FeatureModeFull, nil),
	}
	c.sem = semaphore.NewWeighted(int64(c.scheduler.WorkerCapacity))
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.handleTask(ctx, msgID, map[string]any{
			"task_id": "tcb", "endpoint": "/ep", "priority": 5, "params": "{}",
		})
	}()
	c.wg.Wait()

	select {
	case resp := <-callbackCalled:
		assert.Nil(t, resp.Err)
		assert.Equal(t, "tcb", resp.TaskID)
	case <-time.After(2 * time.Second):
		t.Fatal("callback not called")
	}
}

// ── spawnAdmit stopped guard ──────────────────────────────────────────────────

func TestSpawnAdmit_SkipsWhenStopped(t *testing.T) {
	_, rdb := setupMiniredis(t)
	c := NewConsumer(rdb, RedisConfig{},
		map[string]EndpointConfig{"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }}},
		"ns")

	// running=false: spawnAdmit should return without executing admitMessage
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()

	handlerCalled := false
	c.endpoints["/ep"] = EndpointConfig{
		Handler: func(ctx Context) (any, error) {
			handlerCalled = true
			return nil, nil
		},
	}

	c.spawnAdmit(context.Background(), "msg-1", map[string]any{
		"task_id": "t1", "endpoint": "/ep", "priority": 5, "params": "{}",
	})
	c.wg.Wait()
	assert.False(t, handlerCalled)
}

// ── drainPending: empty queue cleanup ─────────────────────────────────────────

func TestDrainPending_EmptyQueueCleanup(t *testing.T) {
	_, rdb := setupMiniredis(t)
	c := NewConsumer(rdb, RedisConfig{},
		map[string]EndpointConfig{"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }}},
		"ns")
	c.mu.Lock()
	c.running = true
	// Add an endpoint to pendingOrder but leave queue empty
	c.pendingByEndpoint["/ep"] = &EndpointPendingQueue{}
	c.pendingOrder = []string{"/ep"}
	c.mu.Unlock()

	n := c.drainPending(context.Background())
	assert.Equal(t, 0, n)

	c.mu.Lock()
	_, exists := c.pendingByEndpoint["/ep"]
	c.mu.Unlock()
	assert.False(t, exists, "empty queue should be cleaned up")
}

// ── recoverPending: poisoned message ─────────────────────────────────────────

func TestRecoverPending_PoisonedMessage(t *testing.T) {
	mr, rdb := setupMiniredis(t)
	ctx := context.Background()

	streamKey := buildStreamKey("ns-poison")
	groupName := "g"
	consumerName := "c1"

	require.NoError(t, rdb.XGroupCreateMkStream(ctx, streamKey, groupName, "0").Err())
	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]any{"task_id": "tpoison", "endpoint": "/ep", "priority": "5", "params": "{}"},
	}).Result()
	require.NoError(t, err)

	// Read to put in PEL
	_, err = rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: groupName, Consumer: "dead-consumer",
		Streams: []string{streamKey, ">"}, Count: 1,
	}).Result()
	require.NoError(t, err)

	// Fast forward past MinIdle
	mr.FastForward(10 * time.Minute)

	statusStore := NewStatusStore(rdb, "ns-poison")
	require.NoError(t, statusStore.InitQueued(ctx, "tpoison", "/ep", 5))

	c := &Consumer{
		rdb:       rdb,
		client:    rdb,
		cfg:       RedisConfig{},
		namespace: "ns-poison",
		groupName: groupName,
		pel: PelConfig{
			MinIdle:    5 * time.Minute,
			MaxRetries: 3, // message has been delivered once, use low retries
		},
		endpoints: map[string]EndpointConfig{
			"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }},
		},
		statusStore:       statusStore,
		limiter:           &NoopConcurrencyLimiter{},
		activeMsgIDs:      make(map[string]struct{}),
		pendingByEndpoint: make(map[string]*EndpointPendingQueue),
		taskCancelFuncs:   make(map[string]context.CancelFunc),
		scheduler:         defaultSchedulerConfig(),
	}
	c.sem = semaphore.NewWeighted(int64(c.scheduler.WorkerCapacity))

	// Read 3 more times to bump delivery count above MaxRetries
	for i := 0; i < 3; i++ {
		rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: groupName, Consumer: "dead-consumer2",
			Streams: []string{streamKey, "0"}, Count: 1,
		})
	}
	mr.FastForward(10 * time.Minute)

	require.NoError(t, c.recoverPending(ctx, consumerName))

	// Message should be claimed (and either poisoned→failed or re-queued for retry)
	// The key assertion: recoverPending completes without error
	ts, err := statusStore.Get(ctx, "tpoison")
	require.NoError(t, err)
	require.NotNil(t, ts)
	// Status has been updated by recoverPending (either failed or queued for retry)
	t.Logf("status after recover: %s", ts.Status)
}

// ── runCancelSubscriber: closed channel ───────────────────────────────────────

func TestRunCancelSubscriber_ClosedChannel(t *testing.T) {
	_, rdb := setupMiniredis(t)
	ctx, cancel := context.WithCancel(context.Background())

	c := NewConsumer(rdb, RedisConfig{},
		map[string]EndpointConfig{"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }}},
		"ns")

	done := make(chan struct{})
	go func() {
		c.runCancelSubscriber(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runCancelSubscriber did not stop after ctx cancel")
	}
}

// ── extractPriority: string with no digits ────────────────────────────────────

func TestExtractPriority_StringNoDigits(t *testing.T) {
	assert.Equal(t, 5, extractPriority(map[string]any{"priority": "high"}))
}
