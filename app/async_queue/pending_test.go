package async_queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEndpointPendingQueue_Empty(t *testing.T) {
	q := &EndpointPendingQueue{}
	assert.True(t, q.IsEmpty())
	assert.Equal(t, 0, q.Len())

	_, _, ok := q.Peek()
	assert.False(t, ok)

	_, _, ok = q.Pop()
	assert.False(t, ok)
}

func TestEndpointPendingQueue_PushAndPop(t *testing.T) {
	q := &EndpointPendingQueue{}

	q.Push(5, "msg-1", map[string]any{"key": "val1"})
	q.Push(3, "msg-2", map[string]any{"key": "val2"})
	q.Push(9, "msg-3", map[string]any{"key": "val3"})

	assert.Equal(t, 3, q.Len())
	assert.False(t, q.IsEmpty())

	msgID, data, ok := q.Pop()
	assert.True(t, ok)
	assert.Equal(t, "msg-3", msgID)
	assert.Equal(t, "val3", data["key"])

	msgID, data, ok = q.Pop()
	assert.True(t, ok)
	assert.Equal(t, "msg-1", msgID)
	assert.Equal(t, "val1", data["key"])

	msgID, data, ok = q.Pop()
	assert.True(t, ok)
	assert.Equal(t, "msg-2", msgID)
	assert.Equal(t, "val2", data["key"])

	assert.True(t, q.IsEmpty())
}

func TestEndpointPendingQueue_Peek(t *testing.T) {
	q := &EndpointPendingQueue{}

	q.Push(1, "low", map[string]any{})
	q.Push(10, "high", map[string]any{})

	msgID, _, ok := q.Peek()
	assert.True(t, ok)
	assert.Equal(t, "high", msgID)

	assert.Equal(t, 2, q.Len())
}

func TestEndpointPendingQueue_SamePriority(t *testing.T) {
	q := &EndpointPendingQueue{}

	q.Push(5, "msg-a", map[string]any{})
	q.Push(5, "msg-b", map[string]any{})
	q.Push(5, "msg-c", map[string]any{})

	assert.Equal(t, 3, q.Len())

	ids := make(map[string]bool)
	for i := 0; i < 3; i++ {
		msgID, _, ok := q.Pop()
		assert.True(t, ok)
		ids[msgID] = true
	}
	assert.True(t, ids["msg-a"])
	assert.True(t, ids["msg-b"])
	assert.True(t, ids["msg-c"])
}
