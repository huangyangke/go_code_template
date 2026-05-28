package async_queue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskContext_Fields(t *testing.T) {
	params := map[string]any{"input": "data"}
	ctx := newTaskContext(context.Background(), "task-001", "/process", params, nil)

	assert.Equal(t, "task-001", ctx.TaskID())
	assert.Equal(t, "/process", ctx.Endpoint())
	assert.Equal(t, params, ctx.Params())
}

func TestTaskContext_ReportProgress_NilStore(t *testing.T) {
	ctx := newTaskContext(context.Background(), "task-001", "/process", nil, nil)
	assert.NoError(t, ctx.ReportProgress(50, "halfway"))
}

func TestTaskContext_ContextPropagation(t *testing.T) {
	type ctxKey string
	parent := context.WithValue(context.Background(), ctxKey("key"), "value")
	ctx := newTaskContext(parent, "task-001", "/process", nil, nil)

	assert.Equal(t, "value", ctx.Value(ctxKey("key")))
}

func TestTaskContext_ReportProgress_WithStore(t *testing.T) {
	_, store := setupStatusStore(t)
	ctx := context.Background()

	// Seed a task so the status hash exists
	require.NoError(t, store.InitQueued(ctx, "task-progress", "/ep", 5))
	require.NoError(t, store.MarkRunning(ctx, "task-progress"))

	taskCtx := newTaskContext(ctx, "task-progress", "/ep", nil, store)
	err := taskCtx.ReportProgress(75, "three-quarters done")
	require.NoError(t, err)

	ts, err := store.Get(ctx, "task-progress")
	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, 75, ts.Progress)
	assert.Equal(t, "three-quarters done", ts.Message)
	assert.True(t, ts.SupportsProgress)
}
