package httpclient

import (
	"context"
	"fmt"
	"runtime/debug"
)

// RecoveryMiddleware catches panics in downstream handlers and converts them to errors.
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
