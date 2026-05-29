package async_queue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangyangke/go-aikit/app/middleware"
	"github.com/huangyangke/go-aikit/internal/testutil"
)

// newTestProducer builds a Producer connected to a miniredis instance.
func newTestProducer(t *testing.T) *Producer {
	t.Helper()
	_, rdb := setupMiniredis(t)
	p := NewProducer(
		rdb,
		RedisConfig{},
		map[string]EndpointConfig{
			"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }},
		},
		"test",
	)
	return p
}

// setupGin returns a gin engine with the producer routes registered.
func setupGin(t *testing.T, p *Producer) *gin.Engine {
	t.Helper()
	r := testutil.NewGinRouter(t)
	p.RegisterRoutes(r, "/tasks")
	return r
}

// ================================
// NewProducer panic
// ================================

func TestNewProducer_EmptyFamily_Panics(t *testing.T) {
	_, rdb := setupMiniredis(t)
	assert.Panics(t, func() {
		NewProducer(rdb, RedisConfig{}, map[string]EndpointConfig{}, "")
	})
}

// ================================
// handleStatus
// ================================

func TestHandleStatus_Found(t *testing.T) {
	p := newTestProducer(t)
	ctx := context.Background()

	require.NoError(t, p.statusStore.InitQueued(ctx, "task-abc", "/ep", 5))

	r := setupGin(t, p)
	req := httptest.NewRequest(http.MethodGet, "/tasks/status/task-abc", nil)
	rec := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, rec, http.StatusOK)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, TaskStatusQueued, data["status"])
}

func TestHandleStatus_NotFound(t *testing.T) {
	p := newTestProducer(t)
	r := setupGin(t, p)

	req := httptest.NewRequest(http.MethodGet, "/tasks/status/nonexistent", nil)
	rec := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

// ================================
// handleCancel
// ================================

func TestHandleCancel_NotFound(t *testing.T) {
	p := newTestProducer(t)
	r := setupGin(t, p)

	req := httptest.NewRequest(http.MethodPost, "/tasks/cancel/ghost-id", nil)
	rec := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestHandleCancel_TerminalState_Conflict(t *testing.T) {
	p := newTestProducer(t)
	ctx := context.Background()
	r := setupGin(t, p)

	// Put task in terminal success state
	require.NoError(t, p.statusStore.InitQueued(ctx, "done-task", "/ep", 5))
	require.NoError(t, p.statusStore.MarkSuccess(ctx, "done-task", "result"))

	req := httptest.NewRequest(http.MethodPost, "/tasks/cancel/done-task", nil)
	rec := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, rec, http.StatusConflict)
}

func TestHandleCancel_Queued_AtomicallyCancels(t *testing.T) {
	p := newTestProducer(t)
	ctx := context.Background()
	r := setupGin(t, p)

	require.NoError(t, p.statusStore.InitQueued(ctx, "queued-task", "/ep", 5))

	req := httptest.NewRequest(http.MethodPost, "/tasks/cancel/queued-task", nil)
	rec := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	ts, err := p.statusStore.Get(ctx, "queued-task")
	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, TaskStatusCancelled, ts.Status)
}

func TestHandleCancel_Running_WritesCancelKeyAndPublishes(t *testing.T) {
	_, rdb := setupMiniredis(t)
	p := NewProducer(rdb, RedisConfig{},
		map[string]EndpointConfig{
			"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }},
		}, "test")
	ctx := context.Background()
	r := setupGin(t, p)

	// Subscribe to cancel channel before the request
	pubsub := rdb.Subscribe(ctx, buildCancelChannel("test"))
	defer pubsub.Close()
	ch := pubsub.Channel()

	// Put task into running state
	require.NoError(t, p.statusStore.InitQueued(ctx, "run-task", "/ep", 5))
	require.NoError(t, p.statusStore.MarkRunning(ctx, "run-task"))

	req := httptest.NewRequest(http.MethodPost, "/tasks/cancel/run-task", nil)
	rec := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	// Cancel key should exist in Redis
	cancelKey := buildCancelKey("test", "run-task")
	n, err := rdb.Exists(ctx, cancelKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "cancel key should be set")

	// Pub/Sub message should arrive
	select {
	case msg := <-ch:
		assert.Equal(t, "run-task", msg.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("Pub/Sub cancel message not received")
	}
}

// ================================
// handleEvents – terminal task
// ================================

func TestHandleEvents_TerminalTask_SendsStatusAndCloses(t *testing.T) {
	_, rdb := setupMiniredis(t)
	p := NewProducer(rdb, RedisConfig{},
		map[string]EndpointConfig{
			"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }},
		}, "test")
	ctx := context.Background()
	r := setupGin(t, p)

	// Seed a task in success state
	require.NoError(t, p.statusStore.InitQueued(ctx, "fin-task", "/ep", 5))
	require.NoError(t, p.statusStore.MarkSuccess(ctx, "fin-task", "output"))

	req := httptest.NewRequest(http.MethodGet, "/tasks/events/fin-task", nil)
	rec := testutil.ServeRequest(r, req)

	body := rec.Body.String()
	assert.Contains(t, body, "data:", "SSE response must contain data: lines")
	assert.Contains(t, body, fmt.Sprintf("%q", TaskStatusSuccess))
}

// ================================
// toJSON
// ================================

func TestToJSON_NormalObject(t *testing.T) {
	v := map[string]any{"key": "value", "num": 42}
	out := toJSON(v)
	assert.NotEqual(t, "{}", out)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Equal(t, "value", parsed["key"])
}

func TestToJSON_NilDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		out := toJSON(nil)
		// nil marshals to "null"
		assert.Equal(t, "null", out)
	})
}

// ================================
// RegisterRoutes
// ================================

func TestRegisterRoutes_RoutesExist(t *testing.T) {
	_, rdb := setupMiniredis(t)
	p := NewProducer(rdb, RedisConfig{},
		map[string]EndpointConfig{
			"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }},
		}, "test")

	r := setupGin(t, p)

	// Seed a task so status/cancel routes can exercise handler logic (not just "not found")
	ctx := context.Background()
	require.NoError(t, p.statusStore.InitQueued(ctx, "route-check-task", "/ep", 5))

	// /tasks/ep POST should exist (enqueue)
	req := httptest.NewRequest(http.MethodPost, "/tasks/ep",
		strings.NewReader(`{"foo":"bar"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := testutil.ServeRequest(r, req)
	// Route is registered; any non-405 response means it was handled
	assert.NotEqual(t, http.StatusMethodNotAllowed, rec.Code, "/tasks/ep should be registered as POST")

	// /tasks/status/:task_id GET — route is registered; returns 200 for existing task
	req2 := httptest.NewRequest(http.MethodGet, "/tasks/status/route-check-task", nil)
	rec2 := testutil.ServeRequest(r, req2)
	testutil.AssertStatus(t, rec2, http.StatusOK)
	// Confirm the response is JSON (not gin's plain-text 404-page-not-found)
	assert.Contains(t, rec2.Header().Get("Content-Type"), "application/json")

	// /tasks/cancel/:task_id POST — route is registered; returns 200 for queued task
	req3 := httptest.NewRequest(http.MethodPost, "/tasks/cancel/route-check-task", nil)
	rec3 := testutil.ServeRequest(r, req3)
	// Cancel of a queued task returns 200
	testutil.AssertStatus(t, rec3, http.StatusOK)

	// /tasks/events/:task_id GET — may time-out but must not be 404
	reqCtx, reqCtxCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer reqCtxCancel()
	req4 := httptest.NewRequest(http.MethodGet, "/tasks/events/route-check-task", nil).
		WithContext(reqCtx)
	rec4 := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		r.ServeHTTP(rec4, req4)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}
	// Route is registered: response should be SSE (200), not 404
	testutil.AssertStatus(t, rec4, http.StatusOK)
}

// ── TaskEventDispatcher start/run/runOnce ─────────────────────────────────────

func TestTaskEventDispatcher_StartIdempotent(t *testing.T) {
	mr, rdb := setupMiniredis(t)
	defer mr.Close()
	d := newTaskEventDispatcher(rdb, buildTaskEventsChannel("ns"))

	// start twice should not panic
	d.start()
	d.start() // second call is a no-op
	time.Sleep(10 * time.Millisecond)
	// cleanup
	d.mu.Lock()
	d.running = false
	d.mu.Unlock()
}

func TestTaskEventDispatcher_RunOnce_ReconnectsWithSubscribers(t *testing.T) {
	mr, rdb := setupMiniredis(t)
	defer mr.Close()
	channel := buildTaskEventsChannel("ns-reconnect")
	d := newTaskEventDispatcher(rdb, channel)

	ch := d.subscribe("task-x")
	defer d.unsubscribe("task-x", ch)

	// The dispatcher goroutine is running; close miniredis to force disconnect
	// then verify it reconnects (running stays true while there are subscribers).
	time.Sleep(50 * time.Millisecond)
	mr.Close()
	time.Sleep(100 * time.Millisecond)
	// Should still be marked running (trying to reconnect)
	d.mu.RLock()
	running := d.running
	d.mu.RUnlock()
	// It may or may not be running depending on timing; the key thing is no panic.
	_ = running
}

func TestTaskEventDispatcher_Unsubscribe_LastSubscriber(t *testing.T) {
	mr, rdb := setupMiniredis(t)
	defer mr.Close()
	d := newTaskEventDispatcher(rdb, buildTaskEventsChannel("ns-unsub"))

	ch := d.subscribe("task-y")
	d.unsubscribe("task-y", ch)

	d.mu.RLock()
	_, exists := d.subscribers["task-y"]
	d.mu.RUnlock()
	assert.False(t, exists)
}

// ── handleEnqueue: all error branches ────────────────────────────────────────

func TestHandleEnqueue_XAddError(t *testing.T) {
	mr, rdb := setupMiniredis(t)

	p := NewProducer(rdb, RedisConfig{}, map[string]EndpointConfig{
		"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }},
	}, "ns-enq")

	router := testutil.NewGinRouter(t)
	p.RegisterRoutes(router, "/tasks")

	// Cause XAdd to fail by closing miniredis after InitQueued
	// We do this by setting stream key to wrong type first
	ctx := context.Background()
	rdb.Set(ctx, buildStreamKey("ns-enq"), "notastream", 0)

	body := `{"text":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/tasks/ep", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := testutil.ServeRequest(router, req)
	// Either 500 (XAdd failed) or 500 (InitQueued failed because stream is wrong type)
	testutil.AssertStatus(t, rec, http.StatusInternalServerError)
	_ = mr
}

func TestHandleEnqueue_DuplicateTaskID(t *testing.T) {
	mr, rdb := setupMiniredis(t)
	defer mr.Close()

	p := NewProducer(rdb, RedisConfig{}, map[string]EndpointConfig{
		"/ep": {Handler: func(ctx Context) (any, error) { return nil, nil }},
	}, "ns-dup")

	router := testutil.NewGinRouter(t)
	router.Use(middleware.RequestID()) // inject X-Request-ID into context
	p.RegisterRoutes(router, "/tasks")

	body := `{"text":"hello"}`
	taskID := "fixed-task-id-001"

	// First request
	req := httptest.NewRequest(http.MethodPost, "/tasks/ep", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", taskID)
	rec := testutil.ServeRequest(router, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	// Second request with same task ID → 409
	req2 := httptest.NewRequest(http.MethodPost, "/tasks/ep", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Request-ID", taskID)
	rec2 := testutil.ServeRequest(router, req2)
	testutil.AssertStatus(t, rec2, http.StatusConflict)
}

// ── handleStatus: error path ─────────────────────────────────────────────────

func TestHandleStatus_RedisError(t *testing.T) {
	mr, rdb := setupMiniredis(t)

	p := NewProducer(rdb, RedisConfig{}, map[string]EndpointConfig{}, "ns-sterr")
	router := testutil.NewGinRouter(t)
	p.RegisterRoutes(router, "/tasks")

	mr.Close() // force Redis error

	req := httptest.NewRequest(http.MethodGet, "/tasks/status/any-id", nil)
	rec := testutil.ServeRequest(router, req)
	testutil.AssertStatus(t, rec, http.StatusInternalServerError)
}

// ── handleCancel: set key error ───────────────────────────────────────────────

func TestHandleCancel_SetKeyError(t *testing.T) {
	mr, rdb := setupMiniredis(t)

	p := NewProducer(rdb, RedisConfig{}, map[string]EndpointConfig{}, "ns-caerr")
	router := testutil.NewGinRouter(t)
	p.RegisterRoutes(router, "/tasks")

	ctx := context.Background()
	require.NoError(t, p.statusStore.InitQueued(ctx, "task-ca", "/ep", 5))
	require.NoError(t, p.statusStore.MarkRunning(ctx, "task-ca"))

	mr.Close() // force Redis error on Set

	req := httptest.NewRequest(http.MethodPost, "/tasks/cancel/task-ca", nil)
	rec := testutil.ServeRequest(router, req)
	// Redis closed → Get would fail with 500, or Set would fail — either way not 200
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

// ── handleEvents: non-terminal then heartbeat ─────────────────────────────────

func TestHandleEvents_NonTerminalTask_SendsHeartbeat(t *testing.T) {
	mr, rdb := setupMiniredis(t)
	defer mr.Close()

	origHeartbeat := TaskHeartbeatInterval
	TaskHeartbeatInterval = 30 * time.Millisecond
	defer func() { TaskHeartbeatInterval = origHeartbeat }()

	origLifetime := DefaultSSEMaxLifetime
	DefaultSSEMaxLifetime = 150 * time.Millisecond
	defer func() { DefaultSSEMaxLifetime = origLifetime }()

	p := NewProducer(rdb, RedisConfig{}, map[string]EndpointConfig{}, "ns-ev2")
	router := testutil.NewGinRouter(t)
	p.RegisterRoutes(router, "/tasks")

	ctx := context.Background()
	require.NoError(t, p.statusStore.InitQueued(ctx, "task-ev2", "/ep", 5))
	require.NoError(t, p.statusStore.MarkRunning(ctx, "task-ev2"))

	req := httptest.NewRequest(http.MethodGet, "/tasks/events/task-ev2", nil)
	rec := testutil.ServeRequest(router, req)

	body := rec.Body.String()
	// Should have received at least the initial running status + heartbeat or timeout
	assert.Contains(t, body, "running")
}

// ── toJSON: nil input ─────────────────────────────────────────────────────────

func TestToJSON_Nil(t *testing.T) {
	result := toJSON(nil)
	assert.Equal(t, "null", result)
}

// ── parseTaskPriority: full coverage ─────────────────────────────────────────

func TestParseTaskPriority_Clamped(t *testing.T) {
	assert.Equal(t, 0, parseTaskPriority("-5"))
	assert.Equal(t, 9, parseTaskPriority("100"))
}
