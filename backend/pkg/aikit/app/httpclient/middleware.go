package httpclient

import "context"

// Handler is the core function signature for HTTP client middleware chain.
type Handler func(ctx context.Context, req *Request) (*Response, error)

// Middleware wraps a Handler to add cross-cutting concerns (breaker, retry, metrics, etc.).
type Middleware func(Handler) Handler

// Chain builds a single Middleware from multiple, applying them outer-first.
// Chain(m1, m2, m3)(h) produces: m1(m2(m3(h)))
func Chain(middlewares ...Middleware) Middleware {
	return func(next Handler) Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
