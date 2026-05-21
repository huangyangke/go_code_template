package xjob

import (
	"github.com/xxl-job/xxl-job-executor-go"

	"github.com/example/go-template/pkg/aikit/log"
)

// Executor wraps xxl-job-executor-go with go-aikit conventions.
type Executor struct {
	executor xxl.Executor
	config   *Config
	logHandler *LogHandler
}

// Task represents a registered XXL-Job task.
type Task struct {
	pattern  string
	taskFunc xxl.TaskFunc
}

// Option configures the xxl-job executor.
type Option func(e xxl.Executor)

// NewTask creates a task with a handler pattern and function.
func NewTask(pattern string, taskFunc xxl.TaskFunc) *Task {
	return &Task{pattern: pattern, taskFunc: taskFunc}
}

// NewExecutor creates a new XXL-Job executor.
func NewExecutor(cfg *Config, opts ...Option) (*Executor, error) {
	cfg.Fix()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	lh := NewLogHandler(cfg.LogDir, cfg.MaxAge)
	setDefaultLogHandler(lh)

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

// Run registers tasks and starts the executor in a background goroutine.
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
			e.executor.Stop()
		}
	}()

	log.Info("[XxlJob][started][family=%s][port=%s]", e.config.Family, e.config.ExecutorPort)
}

// Stop stops the executor.
func (e *Executor) Stop() {
	e.executor.Stop()
	if e.logHandler != nil {
		e.logHandler.Close()
	}
	log.Info("[XxlJob][stopped][family=%s]", e.config.Family)
}

// Get returns the underlying xxl.Executor.
func (e *Executor) Get() xxl.Executor {
	return e.executor
}

// GetConfig returns the config.
func (e *Executor) GetConfig() *Config {
	if e == nil {
		return nil
	}
	return e.config
}

// WithMiddleware returns an Option that adds middleware to the executor.
func WithMiddleware(m ...xxl.Middleware) Option {
	return func(e xxl.Executor) {
		e.Use(m...)
	}
}
