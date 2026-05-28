# gopool — 有界 goroutine 池

有界 goroutine 池，防止 goroutine 泄漏，支持 panic 恢复。

## 用法

```go
// 使用全局池
gopool.Go(func() {
    doWork()
})
gopool.CtxGo(ctx, func(ctx context.Context) {
    doWorkWithCtx(ctx)
})

// 创建自定义池
pool := gopool.New("worker-pool", 100) // 容量 100
pool.Go(func() { doWork() })
pool.SetPanicHandler(func(ctx context.Context, r any) {
    log.Error("panic: %v", r)
})

count := pool.WorkerCount()
```

## 优雅关闭

```go
// Wait: 阻塞直到所有已提交任务完成
pool.Wait()

// Close: 停止接受新任务，等待现有任务排空
pool.Close()

// Shutdown: 带超时的优雅关闭
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := pool.Shutdown(ctx); err != nil {
    log.Error("pool shutdown timeout: %v", err)
}
```

全局池同样支持 `gopool.Wait()`。
