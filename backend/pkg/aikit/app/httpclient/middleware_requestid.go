package httpclient

import (
	"context"

	"github.com/example/go-template/pkg/aikit/app/middleware"
)

func RequestIDMiddleware() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			if id := middleware.GetTaskID(ctx); id != "" {
				if req.Header.Get("X-Request-ID") == "" {
					req.Header.Set("X-Request-ID", id)
				}
			}
			return next(ctx, req)
		}
	}
}
