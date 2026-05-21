package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/example/go-template/pkg/aikit/metrics"
)

// Prometheus records HTTP request metrics using the predefined metric vectors.
func Prometheus() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		family := metrics.ServiceFamily()
		metrics.ObserveHTTPRequest(family, c.Request.Method, path, status, time.Since(start))
	}
}
