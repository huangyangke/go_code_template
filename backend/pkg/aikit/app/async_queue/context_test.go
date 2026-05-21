package async_queue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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
