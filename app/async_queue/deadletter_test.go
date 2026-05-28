package async_queue

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendToDeadLetter(t *testing.T) {
	_, rdb := setupMiniredis(t)

	c := &Consumer{
		rdb:       rdb,
		namespace: "test",
	}

	msg := redis.XMessage{
		ID: "12345-0",
		Values: map[string]any{
			"task_id":  "task-abc",
			"endpoint": "/process",
		},
	}

	ctx := context.Background()
	c.sendToDeadLetter(ctx, msg)

	result, err := rdb.XRange(ctx, "aikit:async:test:deadletter", "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, "task-abc", result[0].Values["task_id"])
	assert.Equal(t, "/process", result[0].Values["endpoint"])
	assert.Equal(t, "12345-0", result[0].Values["original_msg_id"])
	assert.NotNil(t, result[0].Values["dead_at"])
}

func TestSendToDeadLetter_XAddError(t *testing.T) {
	// 手动创建 miniredis 后立即关闭，让 XADD 必然失败
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mr.Close()

	c := &Consumer{
		rdb:       rdb,
		namespace: "test",
	}

	msg := redis.XMessage{
		ID:     "12345-0",
		Values: map[string]any{"task_id": "task-abc"},
	}

	// 不应 panic，只是 log error
	c.sendToDeadLetter(context.Background(), msg)
}

// 即使传入已取消的 ctx，死信写入仍应成功完成（WithoutCancel 脱离原 ctx）
func TestSendToDeadLetter_CanceledCtx(t *testing.T) {
	_, rdb := setupMiniredis(t)
	ctx := context.Background()

	c := &Consumer{
		rdb:       rdb,
		namespace: "cancel-test",
	}

	msg := redis.XMessage{
		ID: "99999-0",
		Values: map[string]any{
			"task_id":  "task-cancel",
			"endpoint": "/cancel",
		},
	}

	// 构造已取消的 ctx，验证 sendToDeadLetter 不受影响
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	c.sendToDeadLetter(cancelCtx, msg)

	result, err := rdb.XRange(ctx, "aikit:async:cancel-test:deadletter", "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "task-cancel", result[0].Values["task_id"])
	assert.Equal(t, "99999-0", result[0].Values["original_msg_id"])
	assert.NotNil(t, result[0].Values["dead_at"])
}

// msg.Values 在 sendToDeadLetter 后不应被污染（copy 而非原地修改）
func TestSendToDeadLetter_OriginalValuesNotMutated(t *testing.T) {
	_, rdb := setupMiniredis(t)

	c := &Consumer{
		rdb:       rdb,
		namespace: "mutate-test",
	}

	originalValues := map[string]any{
		"task_id":  "task-mutate",
		"endpoint": "/mutate",
	}

	msg := redis.XMessage{
		ID:     "55555-0",
		Values: originalValues,
	}

	c.sendToDeadLetter(context.Background(), msg)

	// 传入的 originalValues map 不应包含 dead_at 和 original_msg_id
	assert.NotContains(t, originalValues, "dead_at")
	assert.NotContains(t, originalValues, "original_msg_id")
	assert.Len(t, originalValues, 2)
}
