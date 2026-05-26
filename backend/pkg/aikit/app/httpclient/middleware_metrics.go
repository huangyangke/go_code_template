package httpclient

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/example/go-template/pkg/aikit/metrics"
)

const maxTrackedPaths = 100

// cardinalityGuard caps the number of unique path label values to prevent
// metric cardinality explosion when callers use URLs with dynamic segments.
type cardinalityGuard struct {
	paths sync.Map
	count int64
}

func (g *cardinalityGuard) safePath(path string) string {
	if _, loaded := g.paths.LoadOrStore(path, struct{}{}); loaded {
		return path
	}
	if atomic.AddInt64(&g.count, 1) > maxTrackedPaths {
		g.paths.Delete(path)
		atomic.AddInt64(&g.count, -1)
		return "high_cardinality_path"
	}
	return path
}

// MetricsMiddleware records Prometheus metrics for HTTP client requests.
func MetricsMiddleware(name string) Middleware {
	guard := &cardinalityGuard{}
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
			path := guard.safePath(req.URL.Path)

			metrics.ObserveHTTPClientRequest(name, method, path, strconv.Itoa(status), duration)

			return resp, err
		}
	}
}
