package httpclient

import (
	"context"
	"fmt"
	"time"

	"github.com/huangyangke/go-aikit/resilience"
)

// RetryMiddleware 返回重试中间件，对 5xx 或网络错误进行指数退避重试.
// 参数：cfg - 重试配置.
// 返回值：Middleware 中间件实例.
func RetryMiddleware(cfg RetryConfig) Middleware {
	backoff := resilience.NewBackoff(
		resilience.WithBackoffBase(cfg.WaitBetween),
		resilience.WithBackoffMax(120*time.Second),
		resilience.WithBackoffFactor(1.6),
		resilience.WithBackoffJitter(cfg.JitterFraction),
	)

	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			var resp *Response
			var lastErr error

			for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
				// Drain and close the previous response body before retrying,
				// so the connection can be reused.
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}

				// Reset request body for retry (POST/PUT with body)
				if attempt > 0 && req.GetBody != nil {
					body, err := req.GetBody()
					if err != nil {
						return resp, fmt.Errorf("httpclient: failed to reset request body: %w", err)
					}
					req.Body = body
				}

				resp, lastErr = next(ctx, req)

				// Success: no error and status < 500
				if lastErr == nil && resp != nil && resp.StatusCode < 500 {
					return resp, nil
				}

				// Context cancelled, stop retrying
				if ctx.Err() != nil {
					return resp, ctx.Err()
				}

				// Last attempt, return whatever we have
				if attempt == cfg.MaxRetries {
					break
				}

				// Wait before next retry (context-aware)
				delay := backoff.Delay(attempt)
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
					// Continue to next attempt
				case <-ctx.Done():
					timer.Stop()
					return resp, ctx.Err()
				}
			}

			return resp, lastErr
		}
	}
}
