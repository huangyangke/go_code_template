package gopool

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// Pool is a goroutine pool bounded by a maximum cap of concurrent workers.
type Pool interface {
	Go(f func())
	CtxGo(ctx context.Context, f func(context.Context))
	SetCap(cap int32)
	SetPanicHandler(f func(context.Context, any))
	WorkerCount() int32
	Wait()
	Close()
	Shutdown(ctx context.Context) error
}

var defaultPool = New("default", 1024)

// Go submits a task to the default global pool.
func Go(f func()) { defaultPool.Go(f) }

// CtxGo submits a context-aware task to the default global pool.
func CtxGo(ctx context.Context, f func(context.Context)) { defaultPool.CtxGo(ctx, f) }

// Wait blocks until all tasks in the default pool have completed.
func Wait() { defaultPool.Wait() }

// ── implementation ────────────────────────────────────────────────────────────

type task struct {
	ctx  context.Context
	fn   func(context.Context)
	next *task
}

var taskPool = sync.Pool{New: func() any { return &task{} }}

type pool struct {
	name         string
	cap          atomic.Int32
	workerCount  atomic.Int32
	taskLock     sync.Mutex
	taskHead     *task
	taskTail     *task
	taskCount    atomic.Int32
	panicHandler func(context.Context, any)
	wg           sync.WaitGroup
	closed       atomic.Bool
}

// New creates a new Pool with the given name and worker cap.
func New(name string, cap int32) Pool {
	p := &pool{name: name}
	p.cap.Store(cap)
	return p
}

func (p *pool) SetCap(cap int32)                              { p.cap.Store(cap) }
func (p *pool) WorkerCount() int32                            { return p.workerCount.Load() }
func (p *pool) SetPanicHandler(f func(context.Context, any))  { p.panicHandler = f }

func (p *pool) Go(f func()) {
	p.CtxGo(context.Background(), func(_ context.Context) { f() })
}

func (p *pool) CtxGo(ctx context.Context, f func(context.Context)) {
	t := taskPool.Get().(*task)
	t.ctx = ctx
	t.fn = f
	t.next = nil

	p.taskLock.Lock()
	if p.closed.Load() {
		p.taskLock.Unlock()
		taskPool.Put(t)
		return
	}
	p.wg.Add(1)
	if p.taskHead == nil {
		p.taskHead = t
		p.taskTail = t
	} else {
		p.taskTail.next = t
		p.taskTail = t
	}
	p.taskLock.Unlock()
	p.taskCount.Add(1)

	if p.workerCount.Load() == 0 || p.workerCount.Load() < p.cap.Load() {
		p.workerCount.Add(1)
		go p.runWorker()
	}
}

// Wait blocks until all submitted tasks have completed.
func (p *pool) Wait() {
	p.wg.Wait()
}

// Close stops accepting new tasks and blocks until all pending tasks drain.
func (p *pool) Close() {
	p.taskLock.Lock()
	p.closed.Store(true)
	p.taskLock.Unlock()
	p.wg.Wait()
}

// Shutdown stops accepting new tasks and waits for pending tasks to drain,
// or until the context is cancelled.
func (p *pool) Shutdown(ctx context.Context) error {
	p.taskLock.Lock()
	p.closed.Store(true)
	p.taskLock.Unlock()

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *pool) runWorker() {
	defer p.workerCount.Add(-1)
	for {
		p.taskLock.Lock()
		t := p.taskHead
		if t != nil {
			p.taskHead = t.next
			if p.taskHead == nil {
				p.taskTail = nil
			}
			p.taskCount.Add(-1)
		}
		p.taskLock.Unlock()

		if t == nil {
			return
		}

		p.safeRun(t)
		t.ctx = nil
		t.fn = nil
		t.next = nil
		taskPool.Put(t)
		p.wg.Done()
	}
}

func (p *pool) safeRun(t *task) {
	defer func() {
		if r := recover(); r != nil {
			if p.panicHandler != nil {
				p.panicHandler(t.ctx, r)
			} else {
				debug.PrintStack()
			}
		}
	}()
	t.fn(t.ctx)
}
