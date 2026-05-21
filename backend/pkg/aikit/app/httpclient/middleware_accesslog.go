package httpclient

import (
	"context"
	"time"

	"github.com/example/go-template/pkg/aikit/log"
)

// AccessLogMiddleware logs HTTP client request details.
func AccessLogMiddleware(name string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			start := time.Now()
			resp, err := next(ctx, req)

			duration := time.Since(start)
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}

			log.Info("[httpclient][%s] %s %s %d %s",
				name, req.Method, req.URL.Path, status, duration.Round(time.Microsecond))

			return resp, err
		}
	}
}
