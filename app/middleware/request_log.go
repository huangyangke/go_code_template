package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huangyangke/go-aikit/log"
)

// RequestLog 返回请求日志中间件，记录方法、路径、状态码和耗时.
// 返回值：gin 中间件 HandlerFunc.
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		elapsed := time.Since(start)
		taskID := GetTaskID(c.Request.Context())
		log.Info("%s %s %d %s %s",
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			fmt.Sprintf("%.3fms", float64(elapsed.Microseconds())/1000),
			taskID,
		)
	}
}
