# log — 零分配日志

自定义零分配结构化日志，支持文件轮转、UDP sink、context-aware trace 透传。

## 用法

```go
// 初始化
log.Init(&log.Config{
    Family:     "my-service",
    Stdout:     true,
    Dir:        "logs",
    Level:      "info",       // debug | info | warn | error | fatal
    RotateSize: 100 * 1024 * 1024,
    MaxLogFile: 10,
})
defer log.Close()

// 基础日志
log.Debug("debug message %s", arg)
log.Info("info message")
log.Warn("warn: %v", err)
log.Error("error: %v", err)

// Context-aware（自动附加 WithFields 注册的字段）
log.InfoCtx(ctx, "processing request")
log.ErrorCtx(ctx, "failed: %v", err)
```

## Trace 透传

通过 `Config.WithFields` 注册字段提取 hook，`*Ctx` 系列函数会自动从 context 提取并附加到日志：

```go
log.Init(&log.Config{
    WithFields: map[string]log.WithField{
        "task_id": func(ctx context.Context) map[string]interface{} {
            if id := middleware.GetTaskID(ctx); id != "" {
                return map[string]interface{}{"task_id": id}
            }
            return nil
        },
    },
})

// 使用 InfoCtx，日志自动包含 task_id 字段
log.InfoCtx(ctx, "processing")
```

## 输出目标

| 目标 | 配置 | 说明 |
|---|---|---|
| Stdout | `Stdout: true` | 控制台输出 |
| File | `Dir: "logs"` | 文件轮转（按大小） |
| UDP Agent | `Agent: "udp://host:port"` | 远端日志采集 |

## API

| 函数 | 说明 |
|---|---|
| `Debug/Info/Warn/Error/Fatal(format, args...)` | 基础日志 |
| `DebugCtx/InfoCtx/WarnCtx/ErrorCtx/FatalCtx(ctx, format, args...)` | Context-aware 日志 |
| `Init(*Config)` | 初始化 |
| `SetLevel(string)` | 动态调整级别 |
| `SetFamily(string)` | 设置 family（FastApp 自动调用） |
| `GetFamily() string` | 获取当前 family |
| `Close()` | 关闭并刷新 |
