package httpclient

import (
	"context"
	"time"

	"github.com/example/go-template/pkg/aikit/resilience"
)

// RetryMiddleware retries requests on 5xx or network errors with exponential backoff.
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
				if resp != nil && resp.Response != nil && resp.Response.Body != nil {
					_ = resp.Response.Body.Close()
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

				// Wait before next retry
				time.Sleep(backoff.Delay(attempt))
			}

			return resp, lastErr
		}
	}
}
