package httpclient

import (
	"context"
	"strconv"
	"time"

	"github.com/example/go-template/pkg/aikit/metrics"
)

// MetricsMiddleware records Prometheus metrics for HTTP client requests.
func MetricsMiddleware(name string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			duration := time.Since(start)

			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			method := req.Method
			path := req.URL.Path

			metrics.ObserveHTTPClientRequest(metrics.ServiceFamily(), name, method, path, strconv.Itoa(status), duration)

			return resp, err
		}
	}
}
