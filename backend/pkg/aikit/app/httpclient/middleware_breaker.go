package httpclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/example/go-template/pkg/aikit/resilience"
)

// responseError wraps an HTTP status code as an error for breaker evaluation.
type responseError struct {
	StatusCode int
}

func (e *responseError) Error() string {
	return fmt.Sprintf("http status %d", e.StatusCode)
}

// isAcceptableHTTPError returns true for errors that should NOT count toward
// circuit breaker opening. HTTP 4xx are business errors; only 5xx and network
// errors count as infrastructure failures.
func isAcceptableHTTPError(err error) bool {
	if err == nil {
		return true
	}
	var re *responseError
	if errors.As(err, &re) {
		return re.StatusCode < 500
	}
	// Network errors (DNS, timeout, connection refused) are NOT acceptable.
	return false
}

// BreakerMiddleware adds circuit breaker protection to HTTP requests.
func BreakerMiddleware(cfg BreakerConfig) Middleware {
	b := resilience.New(&resilience.Config{
		Name:                   cfg.Name,
		MaxRequests:            cfg.MaxRequests,
		RequestVolumeThreshold: cfg.RequestVolumeThreshold,
		SleepWindow:            cfg.SleepWindow,
		ErrorPercentThreshold:  cfg.ErrorPercentThreshold,
	})

	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			var resp *Response
			err := b.Do(func() error {
				var e error
				resp, e = next(ctx, req)
				if e != nil {
					return e
				}
				// Convert 5xx into an error for the breaker.
				if resp.StatusCode >= 500 {
					return &responseError{StatusCode: resp.StatusCode}
				}
				return nil
			}, func(err error) error {
				// Fallback: acceptable errors (4xx) → treat as success (return nil)
				// so the breaker doesn't count them as failures.
				if isAcceptableHTTPError(err) {
					return nil
				}
				return err
			})
			return resp, err
		}
	}
}
