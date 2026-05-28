package async_queue

import "context"

// Context 传给 handler 的任务上下文.
type Context interface {
	context.Context

	// TaskID 返回当前任务 ID.
	// 返回值：taskID - 任务 ID 字符串.
	TaskID() string
	// Endpoint 返回当前端点路径.
	// 返回值：endpoint - 端点路径字符串.
	Endpoint() string
	// Params 返回原始请求参数（JSON 反序列化后的 map）.
	// 返回值：params - 参数映射.
	Params() map[string]any
	// ReportProgress 上报任务进度.
	// 参数：progress - 进度值（0-99）, message - 可选进度提示信息.
	// 返回值：err - 上报失败时的错误.
	ReportProgress(progress int, message ...string) error
}

// taskContext 是 Context 的内部实现.
type taskContext struct {
	context.Context
	taskID      string
	endpoint    string
	params      map[string]any
	statusStore *StatusStore
}

func newTaskContext(
	ctx context.Context,
	taskID, endpoint string,
	params map[string]any,
	store *StatusStore,
) Context {
	return &taskContext{
		Context:     ctx,
		taskID:      taskID,
		endpoint:    endpoint,
		params:      params,
		statusStore: store,
	}
}

func (c *taskContext) TaskID() string         { return c.taskID }
func (c *taskContext) Endpoint() string       { return c.endpoint }
func (c *taskContext) Params() map[string]any { return c.params }

func (c *taskContext) ReportProgress(progress int, message ...string) error {
	if c.statusStore == nil {
		return nil
	}
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	return c.statusStore.UpdateProgress(c.Context, c.taskID, progress, msg)
}
