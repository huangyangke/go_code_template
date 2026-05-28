package httpclient

import (
	"context"
	"fmt"
	"time"

	"github.com/huangyangke/go-aikit/log"
)

// AccessLogMiddleware 返回访问日志中间件.
// 参数：name - 客户端名称，用于日志标识.
// 返回值：Middleware 中间件实例.
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

			msg := fmt.Sprintf("[httpclient][%s] %s %s %d %s",
				name, req.Method, req.URL.Path, status, duration.Round(time.Microsecond))

			if err != nil || status >= 500 {
				log.Warn("%s", msg)
			} else {
				log.Info("%s", msg)
			}

			return resp, err
		}
	}
}
