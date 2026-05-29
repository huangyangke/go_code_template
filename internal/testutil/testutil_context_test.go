package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestContext(t *testing.T) {
	ctx, cancel := NewTestContext(t)
	require.NotNil(t, ctx)
	require.NotNil(t, cancel)

	// 验证 context 未取消
	select {
	case <-ctx.Done():
		t.Fatal("context should not be done")
	default:
	}

	// 主动取消后可以检测到
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be done after cancel")
	}
}

func TestNewTestContext_AutoCancel(t *testing.T) {
	var ctx context.Context
	t.Run("inner", func(t *testing.T) {
		ctx, _ = NewTestContext(t)
	})
	// 内层测试结束后，context 应该已经自动取消
	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be auto-cancelled after test cleanup")
	}
}

func TestNewContextWithTimeout(t *testing.T) {
	ctx, cancel := NewContextWithTimeout(t, 50*time.Millisecond)
	require.NotNil(t, ctx)
	require.NotNil(t, cancel)

	// 等待超时
	select {
	case <-ctx.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("context should timeout")
	}

	assert.Error(t, ctx.Err(), context.DeadlineExceeded)
}

func TestNewContextWithTimeout_CancelBeforeTimeout(t *testing.T) {
	ctx, cancel := NewContextWithTimeout(t, 5*time.Second)

	// 主动取消
	cancel()

	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be done after cancel")
	}

	// 应该是被取消而非超时
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}
