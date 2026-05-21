package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type contextKey string

const taskIDKey contextKey = "task_id"

// RequestID 从 X-Request-ID header 读取或生成 task_id，注入 context。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.GetHeader("X-Request-ID")
		if taskID == "" {
			taskID = uuid.NewString()
		}
		c.Set(string(taskIDKey), taskID)
		c.Header("X-Request-ID", taskID)
		c.Request = c.Request.WithContext(WithTaskID(c.Request.Context(), taskID))
		c.Next()
	}
}

// GetTaskID 从 context 获取 task_id。
func GetTaskID(ctx context.Context) string {
	if v, ok := ctx.Value(taskIDKey).(string); ok {
		return v
	}
	return ""
}

// WithTaskID 将 task_id 注入 context。
func WithTaskID(ctx context.Context, taskID string) context.Context {
	return context.WithValue(ctx, taskIDKey, taskID)
}
