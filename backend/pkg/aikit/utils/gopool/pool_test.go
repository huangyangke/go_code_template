package gopool_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangyangke/go-aikit/utils/gopool"
)

func TestPool_ExecutesAllTasks(t *testing.T) {
	p := gopool.New("test", 10)
	var count atomic.Int64
	for i := 0; i < 100; i++ {
		p.Go(func() {
			count.Add(1)
		})
	}
	p.Wait()
	assert.Equal(t, int64(100), count.Load())
}

func TestPool_CtxGo_PassesContext(t *testing.T) {
	p := gopool.New("test", 5)
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "hello")
	done := make(chan string, 1)
	p.CtxGo(ctx, func(c context.Context) {
		v, _ := c.Value(key{}).(string)
		done <- v
	})
	select {
	case v := <-done:
		assert.Equal(t, "hello", v)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestPool_WorkerCountBoundedByCap(t *testing.T) {
	const cap = 3
	p := gopool.New("test", cap)
	var maxWorkers atomic.Int64
	var mu sync.Mutex

	for i := 0; i < 30; i++ {
		p.Go(func() {
			mu.Lock()
			cur := p.WorkerCount()
			if int64(cur) > maxWorkers.Load() {
				maxWorkers.Store(int64(cur))
			}
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
		})
	}
	p.Wait()
	assert.LessOrEqual(t, int(maxWorkers.Load()), cap)
}

func TestPool_PanicHandlerCalled(t *testing.T) {
	p := gopool.New("test", 5)
	var panicked atomic.Bool
	p.SetPanicHandler(func(_ context.Context, _ any) {
		panicked.Store(true)
	})

	p.Go(func() {
		panic("oops")
	})
	p.Wait()
	assert.True(t, panicked.Load())
}

func TestPool_SetCap(t *testing.T) {
	p := gopool.New("test", 2)
	p.SetCap(10)
	var count atomic.Int64
	for i := 0; i < 50; i++ {
		p.Go(func() {
			count.Add(1)
		})
	}
	p.Wait()
	assert.Equal(t, int64(50), count.Load())
}

func TestGlobalPool_Go(t *testing.T) {
	var count atomic.Int64
	for i := 0; i < 20; i++ {
		gopool.Go(func() {
			count.Add(1)
		})
	}
	gopool.Wait()
	assert.Equal(t, int64(20), count.Load())
}

func TestPool_Wait_BlocksUntilDone(t *testing.T) {
	p := gopool.New("test", 2)
	var count atomic.Int64
	for i := 0; i < 10; i++ {
		p.Go(func() {
			time.Sleep(5 * time.Millisecond)
			count.Add(1)
		})
	}
	p.Wait()
	assert.Equal(t, int64(10), count.Load())
}

func TestPool_Close_RejectsNewTasks(t *testing.T) {
	p := gopool.New("test", 5)
	var count atomic.Int64

	for i := 0; i < 5; i++ {
		p.Go(func() {
			count.Add(1)
		})
	}
	p.Close()
	assert.Equal(t, int64(5), count.Load())

	// Tasks submitted after Close are silently dropped
	p.Go(func() {
		count.Add(100)
	})
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, int64(5), count.Load())
}

func TestPool_Shutdown_ContextTimeout(t *testing.T) {
	p := gopool.New("test", 1)
	started := make(chan struct{})
	p.Go(func() {
		close(started)
		time.Sleep(500 * time.Millisecond)
	})
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := p.Shutdown(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestPool_Shutdown_Success(t *testing.T) {
	p := gopool.New("test", 5)
	var count atomic.Int64
	for i := 0; i < 10; i++ {
		p.Go(func() {
			time.Sleep(5 * time.Millisecond)
			count.Add(1)
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := p.Shutdown(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(10), count.Load())
}
