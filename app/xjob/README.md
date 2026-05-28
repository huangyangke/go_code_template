# xjob — XXL-Job 执行器

XXL-Job 执行器集成，支持任务注册、日志收集、中间件扩展。

## 用法

### 通过 FastApp

```go
fa.SetXxlJob(app.XxlJobConfig{
    ServerAddr:   "http://xxl-job-admin:8080/xxl-job-admin",
    AccessToken:  "token",
    ExecutorPort: "9999",
})
fa.RegisterXxlJobTask("myTask", func(ctx context.Context, param *xxl.RunReq) string {
    // 执行任务逻辑
    return "success"
})
```

### 独立使用

```go
executor, _ := xjob.NewExecutor(&xjob.Config{
    Family:     "my-service",
    ServerAddr: "http://xxl-job-admin:8080/xxl-job-admin",
})
executor.Run(
    xjob.NewTask("myTask", taskHandler),
)
defer executor.Stop()
```

## 配置

```yaml
family: my-service
server_addr: http://xxl-job-admin:8080/xxl-job-admin
access_token: ""
executor_port: "9999"    # 默认
log_dir: logs/xjob       # 默认
max_age: 7               # 日志保留天数，默认
```
