package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type contextKey string

const taskIDKey contextKey = "task_id"

// RequestID 返回请求 ID 中间件，从 X-Request-ID 读取或生成新 ID 并注入 context.
// 返回值：gin 中间件 HandlerFunc.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.GetHeader("X-Request-ID")
		// 限制传入 ID 长度，防止日志注入
		if len(taskID) > 128 {
			taskID = ""
		}
		if taskID == "" {
			taskID = uuid.NewString()
		}
		c.Set(string(taskIDKey), taskID)
		c.Header("X-Request-ID", taskID)
		c.Request = c.Request.WithContext(WithTaskID(c.Request.Context(), taskID))
		c.Next()
	}
}

// GetTaskID 从 context 中获取 task_id.
// 参数：ctx - 上下文.
// 返回值：taskID - 任务 ID，不存在时返回空字符串.
func GetTaskID(ctx context.Context) string {
	if v, ok := ctx.Value(taskIDKey).(string); ok {
		return v
	}
	return ""
}

// WithTaskID 将 task_id 注入 context.
// 参数：ctx - 上下文, taskID - 任务 ID.
// 返回值：携带 taskID 的新 context.
func WithTaskID(ctx context.Context, taskID string) context.Context {
	return context.WithValue(ctx, taskIDKey, taskID)
}
