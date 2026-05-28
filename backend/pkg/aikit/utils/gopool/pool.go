// Package gopool 协程池，限制并发 worker 数并复用 task 对象.
package gopool

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// Pool 协程池接口.
type Pool interface {
	// Go 提交无 context 的任务.
	// 参数：f - 任务函数.
	Go(f func())

	// CtxGo 提交带 context 的任务.
	// 参数：ctx - 上下文, f - 任务函数.
	CtxGo(ctx context.Context, f func(context.Context))

	// SetCap 设置最大并发 worker 数.
	// 参数：cap - 最大并发数.
	SetCap(cap int32)

	// SetPanicHandler 设置 panic 处理函数.
	// 参数：f - panic 处理函数，接收上下文和 panic 值.
	SetPanicHandler(f func(context.Context, any))

	// WorkerCount 获取当前活跃 worker 数.
	// 返回值：int32 - 当前活跃 worker 数.
	WorkerCount() int32

	// Wait 等待所有已提交任务完成.
	Wait()

	// Close 关闭 pool 并等待任务排空.
	Close()

	// Shutdown 关闭 pool 并等待已提交任务完成，支持 context 取消.
	// 参数：ctx - 上下文.
	// 返回值：err - context 取消时返回 context 错误.
	Shutdown(ctx context.Context) error
}

// defaultPool 默认协程池，1024 个 worker.
var defaultPool = New("default", 1024)

// Go 向默认池提交无 context 的任务.
// 参数：f - 任务函数.
func Go(f func()) { defaultPool.Go(f) }

// CtxGo 向默认池提交带 context 的任务.
// 参数：ctx - 上下文, f - 任务函数.
func CtxGo(ctx context.Context, f func(context.Context)) { defaultPool.CtxGo(ctx, f) }

// Wait 等待默认池中所有已提交任务完成.
func Wait() { defaultPool.Wait() }

// ── implementation ────────────────────────────────────────────────────────────.

type task struct {
	ctx  context.Context       // 任务上下文
	fn   func(context.Context) // 任务函数
	next *task                 // 下一个任务节点
}

// taskPool 复用 task 对象，避免频繁创建/销毁，降低 GC.
var taskPool = sync.Pool{New: func() any { return &task{} }}

type panicHandlerFunc = func(context.Context, any)

type pool struct {
	name         string                           // pool 名称
	cap          atomic.Int32                     // 最大并发 worker 数
	workerCount  atomic.Int32                     // 当前活跃 worker 数
	taskLock     sync.Mutex                       // 保护任务链表和 worker 创建/退出
	taskHead     *task                            // 任务链表头
	taskTail     *task                            // 任务链表尾
	panicHandler atomic.Pointer[panicHandlerFunc] // panic 处理函数
	wg           sync.WaitGroup                   // 已提交但未完成的任务计数
	closed       atomic.Bool                      // 是否已关闭
}

// normalizeCap 规范化并发上限，最小为 1，避免任务入队后永远没有 worker 消费.
func normalizeCap(cap int32) int32 {
	if cap < 1 {
		return 1
	}
	return cap
}

// New 创建协程池.
// 参数：name - pool 名称, cap - 最大并发 worker 数，cap<=0 时自动修正为 1.
// 返回值：Pool - 协程池实例.
func New(name string, cap int32) Pool {
	p := &pool{name: name}
	p.cap.Store(normalizeCap(cap))
	return p
}

// SetCap 动态调整最大并发 worker 数.
func (p *pool) SetCap(cap int32) { p.cap.Store(normalizeCap(cap)) }

// WorkerCount 获取当前活跃 worker 数.
func (p *pool) WorkerCount() int32 { return p.workerCount.Load() }

// SetPanicHandler 设置 panic 处理函数，任务 panic 时优先回调.
func (p *pool) SetPanicHandler(f func(context.Context, any)) {
	p.panicHandler.Store(&f)
}

// Go 提交无 context 的任务，内部包装成带 context 的任务.
func (p *pool) Go(f func()) {
	p.CtxGo(context.Background(), func(_ context.Context) { f() })
}

// CtxGo 提交带 context 的任务.
// 参数：ctx - 上下文, f - 任务函数.
func (p *pool) CtxGo(ctx context.Context, f func(context.Context)) {
	t := taskPool.Get().(*task)
	t.ctx = ctx
	t.fn = f
	t.next = nil

	p.taskLock.Lock()
	if p.closed.Load() {
		p.taskLock.Unlock()
		taskPool.Put(t) // pool 已关闭，task 不会被消费
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
	// 持锁决定是否创建 worker，避免与 worker 退出时更新 workerCount 发生竞态
	shouldSpawn := p.workerCount.Load() < p.cap.Load()
	if shouldSpawn {
		p.workerCount.Add(1)
	}
	p.taskLock.Unlock()

	if shouldSpawn {
		go p.runWorker()
	}
}

// Wait 等待当前已提交任务全部完成，Wait 本身不阻止其他协程继续提交新任务.
func (p *pool) Wait() { p.wg.Wait() }

// Close 关闭 pool 并等待任务排空.
func (p *pool) Close() { p.Shutdown(context.Background()) } //nolint:errcheck

// Shutdown 关闭 pool 并等待已提交任务完成.
// 参数：ctx - 上下文.
// 返回值：err - context 取消时返回 context 错误.
func (p *pool) Shutdown(ctx context.Context) error {
	// 持锁设置 closed，避免关闭和提交交错时状态不一致
	p.taskLock.Lock()
	p.closed.Store(true)
	p.taskLock.Unlock()

	// WaitGroup 不支持 context，这里通过 channel 桥接等待结果
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

// runWorker 循环消费任务，直到队列为空才退出.
// workerCount 在持锁状态下递减，避免任务刚入队时读到过期 worker 数而不再创建新 worker.
func (p *pool) runWorker() {
	for {
		p.taskLock.Lock()
		t := p.taskHead
		if t != nil {
			p.taskHead = t.next
			if p.taskHead == nil {
				p.taskTail = nil
			}
			p.taskLock.Unlock()
		} else {
			// 持锁下降 worker 计数，防止提交方看到过期 workerCount
			p.workerCount.Add(-1)
			p.taskLock.Unlock()
			return
		}

		p.safeRun(t)

		// 清理引用后放回对象池，避免无用对象被长时间持有
		t.ctx = nil
		t.fn = nil
		t.next = nil
		taskPool.Put(t)
		p.wg.Done()
	}
}

// safeRun 安全执行任务，统一拦截 panic.
// 若设置了 panicHandler 则回调业务处理，否则打印 panic 和堆栈.
func (p *pool) safeRun(t *task) {
	defer func() {
		if r := recover(); r != nil {
			if h := p.panicHandler.Load(); h != nil {
				(*h)(t.ctx, r)
			} else {
				fmt.Printf("gopool[%s] panic: %v\n", p.name, r)
				debug.PrintStack()
			}
		}
	}()
	t.fn(t.ctx)
}
