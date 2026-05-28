package httpclient

import (
	"context"
	"fmt"
	"runtime/debug"
)

// RecoveryMiddleware 返回 panic 恢复中间件.
// 参数：name - 客户端名称，用于错误信息标识.
// 返回值：Middleware 中间件实例.
func RecoveryMiddleware(name string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (resp *Response, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("httpclient [%s] panic: %v\n%s", name, r, debug.Stack())
				}
			}()
			return next(ctx, req)
		}
	}
}
