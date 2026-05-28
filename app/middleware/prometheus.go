package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huangyangke/go-aikit/metrics"
)

// Prometheus 返回 HTTP 请求指标采集中间件.
// 返回值：gin 中间件 HandlerFunc.
func Prometheus() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		metrics.ObserveHTTPRequest(c.Request.Method, path, status, time.Since(start))
	}
}
