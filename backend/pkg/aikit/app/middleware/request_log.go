package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/example/go-template/pkg/aikit/log"
)

// RequestLog logs method, path, status, and duration for each request.
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
