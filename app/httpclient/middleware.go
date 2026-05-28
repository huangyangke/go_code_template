package httpclient

import "context"

// Handler HTTP 客户端中间件链的核心函数签名.
type Handler func(ctx context.Context, req *Request) (*Response, error)

// Middleware 包裹 Handler 以添加横切关注点（熔断、重试、指标等）.
type Middleware func(Handler) Handler

// Chain 将多个中间件合并为单个中间件，外层优先应用.
// 参数：middlewares - 中间件列表.
// 返回值：Middleware 合并后的中间件.
func Chain(middlewares ...Middleware) Middleware {
	return func(next Handler) Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
