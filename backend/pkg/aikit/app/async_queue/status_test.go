package async_queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupStatusStore(t *testing.T) (*miniredis.Miniredis, *StatusStore) {
	t.Helper()
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		rdb.Close()
		mr.Close()
	})
	return mr, NewStatusStore(rdb, "test")
}

func TestInitQueued_SetsAllFields(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	err := store.InitQueued(ctx, "task-1", "/ep", 7)
	require.NoError(t, err)

	ts, err := store.Get(ctx, "task-1")
	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, TaskStatusQueued, ts.Status)
	assert.Equal(t, "/ep", ts.Endpoint)
	assert.Equal(t, 7, ts.Priority)
	assert.Equal(t, 0, ts.Progress)
}

func TestInitQueued_PreventsDuplicates(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	err := store.InitQueued(ctx, "task-1", "/ep", 5)
	require.NoError(t, err)

	err = store.InitQueued(ctx, "task-1", "/ep2", 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestInitQueued_SetsTTL(t *testing.T) {
	mr, store := setupStatusStore(t)
	ctx := context.Background()

	err := store.InitQueued(ctx, "task-1", "/ep", 5)
	require.NoError(t, err)

	key := buildStatusKey("test", "task-1")
	ttl := mr.TTL(key)
	assert.True(t, ttl > 0, "key must have a TTL set")
}

func TestInitQueued_Atomic_NoOrphanKeyOnPartialFailure(t *testing.T) {
	// After InitQueued succeeds, the key must have both data AND a TTL.
	// With a Lua script, all operations are atomic so partial failure is impossible.
	mr, store := setupStatusStore(t)
	ctx := context.Background()

	err := store.InitQueued(ctx, "task-1", "/ep", 5)
	require.NoError(t, err)

	key := buildStatusKey("test", "task-1")

	// Verify key exists and has all expected fields
	result, err := store.rdb.HGetAll(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, TaskStatusQueued, result["status"])
	assert.Equal(t, "/ep", result["endpoint"])
	assert.Equal(t, "5", result["priority"])

	// Verify TTL is set
	ttl := mr.TTL(key)
	assert.True(t, ttl > 0, "key must have TTL — no orphan keys without expiry")
}

func TestMarkRunning(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.MarkRunning(ctx, "task-1"))

	ts, err := store.Get(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, TaskStatusRunning, ts.Status)
	assert.True(t, ts.StartedAt > 0)
}

func TestMarkSuccess(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.MarkSuccess(ctx, "task-1", map[string]any{"output": "done"}))

	ts, err := store.Get(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, TaskStatusSuccess, ts.Status)
	assert.Equal(t, 100, ts.Progress)
	assert.Contains(t, ts.Result, "output")
}

func TestMarkFailed(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.MarkFailed(ctx, "task-1", "something went wrong"))

	ts, err := store.Get(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, TaskStatusFailed, ts.Status)
	assert.Equal(t, "something went wrong", ts.Error)
}

func TestMarkCancelled(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.MarkCancelled(ctx, "task-1", "user cancelled"))

	ts, err := store.Get(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, TaskStatusCancelled, ts.Status)
}

func TestUpdateProgress(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.UpdateProgress(ctx, "task-1", 50, "halfway"))

	ts, err := store.Get(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, 50, ts.Progress)
	assert.Equal(t, "halfway", ts.Message)
	assert.True(t, ts.SupportsProgress)
}

func TestGet_NotFound(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	ts, err := store.Get(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, ts)
}

func TestCancelIfQueued_FromQueued(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))

	cancelled, err := store.CancelIfQueued(ctx, "task-1", "user cancelled")
	require.NoError(t, err)
	assert.True(t, cancelled)

	ts, err := store.Get(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, TaskStatusCancelled, ts.Status)
}

func TestCancelIfQueued_FromRunning(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.MarkRunning(ctx, "task-1"))

	// CancelIfQueued should NOT transition from "running" to "cancelled"
	cancelled, err := store.CancelIfQueued(ctx, "task-1", "user cancelled")
	require.NoError(t, err)
	assert.False(t, cancelled)

	ts, err := store.Get(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, TaskStatusRunning, ts.Status, "status should remain running")
}

func TestCancelIfQueued_Nonexistent(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	cancelled, err := store.CancelIfQueued(ctx, "nonexistent", "reason")
	require.NoError(t, err)
	assert.False(t, cancelled)
}

// subscribeChannel returns a closeable subscription to the task-events channel
// for the given namespace. The receiver should drain msgs until the test ends.
func subscribeChannel(t *testing.T, rdb *redis.Client, namespace string) (<-chan *redis.Message, func()) {
	t.Helper()
	sub := rdb.Subscribe(context.Background(), buildTaskEventsChannel(namespace))
	// Wait for subscription confirmation so the first published message isn't missed.
	_, err := sub.Receive(context.Background())
	require.NoError(t, err)
	return sub.Channel(), func() { _ = sub.Close() }
}

// receiveEvent waits for one pub/sub message and returns it as a map.
func receiveEvent(t *testing.T, ch <-chan *redis.Message) map[string]any {
	t.Helper()
	select {
	case msg := <-ch:
		var ev map[string]any
		require.NoError(t, json.Unmarshal([]byte(msg.Payload), &ev))
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pub/sub event")
		return nil
	}
}

func TestMarkRunning_PublishesEvent(t *testing.T) {
	mr, store := setupStatusStore(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	ctx := context.Background()

	ch, closeSub := subscribeChannel(t, rdb, "test")
	defer closeSub()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.MarkRunning(ctx, "task-1"))

	ev := receiveEvent(t, ch)
	assert.Equal(t, TaskStatusRunning, ev["status"])
	assert.Equal(t, "task-1", ev["task_id"])
}

func TestMarkSuccess_PublishesEvent_ResultIsRawJSON(t *testing.T) {
	mr, store := setupStatusStore(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	ctx := context.Background()

	ch, closeSub := subscribeChannel(t, rdb, "test")
	defer closeSub()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.MarkSuccess(ctx, "task-1", map[string]any{"output": 42}))

	ev := receiveEvent(t, ch)
	assert.Equal(t, TaskStatusSuccess, ev["status"])
	// `result` must be inlined as an object, not a JSON-encoded string.
	result, ok := ev["result"].(map[string]any)
	require.True(t, ok, "result should decode as object, got %T", ev["result"])
	assert.EqualValues(t, 42, result["output"])
}

func TestMarkFailed_PublishesEvent(t *testing.T) {
	mr, store := setupStatusStore(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	ctx := context.Background()

	ch, closeSub := subscribeChannel(t, rdb, "test")
	defer closeSub()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.MarkFailed(ctx, "task-1", "boom"))

	ev := receiveEvent(t, ch)
	assert.Equal(t, TaskStatusFailed, ev["status"])
	assert.Equal(t, "boom", ev["error"])
}

func TestMarkCancelled_PublishesEvent(t *testing.T) {
	mr, store := setupStatusStore(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	ctx := context.Background()

	ch, closeSub := subscribeChannel(t, rdb, "test")
	defer closeSub()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.MarkCancelled(ctx, "task-1", "user cancelled"))

	ev := receiveEvent(t, ch)
	assert.Equal(t, TaskStatusCancelled, ev["status"])
	assert.Equal(t, "user cancelled", ev["message"])
}

func TestCancelIfQueued_PublishesEventOnTransition(t *testing.T) {
	mr, store := setupStatusStore(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	ctx := context.Background()

	ch, closeSub := subscribeChannel(t, rdb, "test")
	defer closeSub()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	cancelled, err := store.CancelIfQueued(ctx, "task-1", "user cancelled")
	require.NoError(t, err)
	require.True(t, cancelled)

	ev := receiveEvent(t, ch)
	assert.Equal(t, TaskStatusCancelled, ev["status"])
}

func TestUpdateProgress_PublishesEvent(t *testing.T) {
	mr, store := setupStatusStore(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	ctx := context.Background()

	ch, closeSub := subscribeChannel(t, rdb, "test")
	defer closeSub()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.UpdateProgress(ctx, "task-1", 42, "almost there"))

	ev := receiveEvent(t, ch)
	assert.EqualValues(t, 42, ev["progress"])
	assert.Equal(t, "almost there", ev["message"])
}

func TestDelete_RemovesStatus(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	require.NoError(t, store.InitQueued(ctx, "task-1", "/ep", 5))
	require.NoError(t, store.Delete(ctx, "task-1"))

	ts, err := store.Get(ctx, "task-1")
	require.NoError(t, err)
	assert.Nil(t, ts)
}
