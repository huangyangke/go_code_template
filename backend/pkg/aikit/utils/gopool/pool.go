package gopool

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

type Pool interface {
	Go(f func())                                        // 提交无context任务
	CtxGo(ctx context.Context, f func(context.Context)) // 提交有context任务
	SetCap(cap int32)                                   // 设置最大并发数
	SetPanicHandler(f func(context.Context, any))       // 设置panic处理函数
	WorkerCount() int32                                 // 获取当前活动worker数
	Wait()                                              // 等待所有任务完成
	Close()                                             // 关闭pool
	Shutdown(ctx context.Context) error                 // 关闭pool并等待所有任务完成
}

var defaultPool = New("default", 1024) // 默认pool 1024个worker

// 封装全局方法，直接调用 gopool.Go(...) 即可使用
func Go(f func())                                        { defaultPool.Go(f) }
func CtxGo(ctx context.Context, f func(context.Context)) { defaultPool.CtxGo(ctx, f) }
func Wait()                                              { defaultPool.Wait() }

// ── implementation ────────────────────────────────────────────────────────────

type task struct {
	ctx  context.Context       // 任务上下文
	fn   func(context.Context) // 任务函数
	next *task                 // 下一个任务节点
}

// sync.Pool：复用task对象，避免频繁创建/销毁，降低GC
var taskPool = sync.Pool{New: func() any { return &task{} }}

type panicHandlerFunc = func(context.Context, any)

type pool struct {
	name         string                           // pool名称
	cap          atomic.Int32                     // 最大并发worker数
	workerCount  atomic.Int32                     // 当前活跃worker数
	taskLock     sync.Mutex                       // 保护任务链表和worker创建/退出
	taskHead     *task                            // 任务链表头
	taskTail     *task                            // 任务链表尾
	panicHandler atomic.Pointer[panicHandlerFunc] // panic处理函数
	wg           sync.WaitGroup                   // 已提交但未完成的任务计数
	closed       atomic.Bool                      // 是否已关闭
}

// 规范化cap，最小为1，避免任务入队后永远没有worker消费
func normalizeCap(cap int32) int32 {
	if cap < 1 {
		return 1
	}
	return cap
}

// 创建pool，cap<=0时自动修正为1
func New(name string, cap int32) Pool {
	p := &pool{name: name}
	p.cap.Store(normalizeCap(cap))
	return p
}

func (p *pool) SetCap(cap int32)   { p.cap.Store(normalizeCap(cap)) } // 动态调整最大并发数
func (p *pool) WorkerCount() int32 { return p.workerCount.Load() }    // 获取当前活跃worker数

// 设置panic处理函数，任务panic时优先回调这里
func (p *pool) SetPanicHandler(f func(context.Context, any)) {
	p.panicHandler.Store(&f)
}

// 无context版本，内部包装成带context的任务
func (p *pool) Go(f func()) {
	p.CtxGo(context.Background(), func(_ context.Context) { f() })
}

// 提交任务：
// 1. 从对象池取task，减少分配
// 2. 加锁入队
// 3. 如当前worker数小于cap，则拉起一个新worker消费队列
func (p *pool) CtxGo(ctx context.Context, f func(context.Context)) {
	t := taskPool.Get().(*task)
	t.ctx = ctx
	t.fn = f
	t.next = nil

	p.taskLock.Lock()
	if p.closed.Load() {
		p.taskLock.Unlock()
		taskPool.Put(t) // 放回对象池
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
	// 持锁决定是否创建worker，避免与worker退出时更新workerCount发生竞态
	shouldSpawn := p.workerCount.Load() < p.cap.Load()
	if shouldSpawn {
		p.workerCount.Add(1)
	}
	p.taskLock.Unlock()

	if shouldSpawn {
		go p.runWorker()
	}
}

// 等待当前已提交任务全部完成
// 注意：Wait本身不阻止其他协程继续提交新任务
func (p *pool) Wait() { p.wg.Wait() }

// 关闭pool并等待任务排空
func (p *pool) Close() { p.Shutdown(context.Background()) } //nolint:errcheck

// Shutdown关闭pool：
// 1. 标记closed，后续新任务直接丢弃
// 2. 等待已提交任务执行完成
// 3. 若ctx先取消，则返回ctx错误
func (p *pool) Shutdown(ctx context.Context) error {
	// 持锁设置closed，避免关闭和提交交错时状态不一致
	p.taskLock.Lock()
	p.closed.Store(true)
	p.taskLock.Unlock()

	// WaitGroup不支持context，这里通过channel桥接等待结果
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

// worker循环消费任务，直到队列为空才退出
// 注意：workerCount在持锁状态下递减，避免任务刚入队时读到过期worker数而不再创建新worker
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
			// Drop the worker slot while holding taskLock so submitters never see
			// a stale workerCount after the queue has gone idle.
			p.workerCount.Add(-1)
			p.taskLock.Unlock()
			return
		}

		p.safeRun(t) // 执行任务并统一处理panic

		// 清理引用后放回对象池，避免无用对象被长时间持有
		t.ctx = nil
		t.fn = nil
		t.next = nil
		taskPool.Put(t)
		p.wg.Done()
	}
}

// 安全执行任务，统一拦截panic
// 若设置了panicHandler则回调业务处理，否则打印panic和堆栈
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
