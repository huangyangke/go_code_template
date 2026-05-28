// Package xjob XXL-Job 执行器封装，提供任务注册、日志管理和中间件.
package xjob

import (
	"sync"

	"github.com/xxl-job/xxl-job-executor-go"

	"github.com/huangyangke/go-aikit/log"
)

// Executor 封装 xxl-job-executor-go，适配 go-aikit 约定.
type Executor struct {
	executor   xxl.Executor
	config     *Config
	logHandler *LogHandler
	stopOnce   sync.Once
}

// Task 注册的 XXL-Job 任务.
type Task struct {
	pattern  string
	taskFunc xxl.TaskFunc
}

// Option xxl-job 执行器配置选项.
type Option func(e xxl.Executor)

// NewTask 创建 XXL-Job 任务.
// 参数：pattern - 任务处理器标识, taskFunc - 任务执行函数.
// 返回值：task - 新创建的任务.
func NewTask(pattern string, taskFunc xxl.TaskFunc) *Task {
	return &Task{pattern: pattern, taskFunc: taskFunc}
}

// NewExecutor 创建 XXL-Job 执行器.
// 参数：cfg - 执行器配置, opts - 可选配置项.
// 返回值：executor - 执行器, err - 配置校验失败时的错误.
func NewExecutor(cfg *Config, opts ...Option) (*Executor, error) {
	cfg.Fix()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	lh := NewLogHandler(cfg.LogDir, cfg.MaxAge)

	e := xxl.NewExecutor(
		xxl.ServerAddr(cfg.ServerAddr),
		xxl.AccessToken(cfg.AccessToken),
		xxl.ExecutorIp(cfg.ExecutorIp),
		xxl.ExecutorPort(cfg.ExecutorPort),
		xxl.RegistryKey(cfg.Family),
		xxl.SetLogger(&logger{}),
	)
	e.Init()
	e.LogHandler(lh.TaskLogHandler)

	for _, opt := range opts {
		opt(e)
	}

	return &Executor{
		executor:   e,
		config:     cfg,
		logHandler: lh,
	}, nil
}

// Run 注册任务并在后台启动执行器.
// 参数：tasks - 任务列表.
func (e *Executor) Run(tasks ...*Task) {
	if e.config.JobDisabled {
		log.Info("[XxlJob][disabled][family=%s]", e.config.Family)
		return
	}

	for _, t := range tasks {
		e.executor.RegTask(t.pattern, t.taskFunc)
	}

	go func() {
		if err := e.executor.Run(); err != nil {
			log.Error("[XxlJob][run_error][family=%s]: %v", e.config.Family, err)
		}
	}()

	log.Info("[XxlJob][started][family=%s][port=%s]", e.config.Family, e.config.ExecutorPort)
}

// Stop 停止执行器，可安全多次调用.
func (e *Executor) Stop() {
	e.stopOnce.Do(func() {
		e.executor.Stop()
		if e.logHandler != nil {
			e.logHandler.Close()
		}
		log.Info("[XxlJob][stopped][family=%s]", e.config.Family)
	})
}

// LogHandler 返回日志处理器.
// 返回值：日志处理器.
func (e *Executor) LogHandler() *LogHandler {
	return e.logHandler
}

// LogInfo 通过当前执行器的日志处理器写入日志.
// 参数：logID - 日志 ID, format - 格式字符串, args - 格式参数.
func (e *Executor) LogInfo(logID int64, format string, args ...interface{}) {
	if e.logHandler != nil {
		e.logHandler.Info(logID, format, args...)
	}
}

// Get 返回底层 xxl.Executor.
// 返回值：底层执行器实例.
func (e *Executor) Get() xxl.Executor {
	return e.executor
}

// GetConfig 返回执行器配置.
// 返回值：执行器配置，为 nil 时返回 nil.
func (e *Executor) GetConfig() *Config {
	if e == nil {
		return nil
	}
	return e.config
}

// WithMiddleware 返回添加中间件的 Option.
// 参数：m - 中间件列表.
// 返回值：配置选项.
func WithMiddleware(m ...xxl.Middleware) Option {
	return func(e xxl.Executor) {
		e.Use(m...)
	}
}
