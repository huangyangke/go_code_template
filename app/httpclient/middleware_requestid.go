package httpclient

import (
	"context"

	"github.com/huangyangke/go-aikit/app/middleware"
)

// RequestIDMiddleware 返回请求 ID 传播中间件.
// 返回值：Middleware 中间件实例.
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
