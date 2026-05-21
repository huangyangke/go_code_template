package resilience

import (
	"math/rand/v2"
	"time"
)

const defaultRetryTimes = 3

// Acceptable returns true when the error is considered acceptable (no retry needed).
type Acceptable func(err error) bool

type retryOptions struct {
	times      int
	acceptable Acceptable
}

// RetryOption customises a Retry call.
type RetryOption func(*retryOptions)

// WithTimes sets the maximum number of attempts.
func WithTimes(n int) RetryOption {
	return func(o *retryOptions) { o.times = n }
}

// WithAcceptable sets a predicate; when it returns true the error is not retried.
func WithAcceptable(fn Acceptable) RetryOption {
	return func(o *retryOptions) { o.acceptable = fn }
}

// Retry runs fn up to 3 times (or as configured) until it returns nil.
func Retry(fn func() error, opts ...RetryOption) error {
	o := &retryOptions{times: defaultRetryTimes}
	for _, opt := range opts {
		opt(o)
	}
	var last error
	for i := 0; i < o.times; i++ {
		last = fn()
		if last == nil {
			return nil
		}
		if o.acceptable != nil && !o.acceptable(last) {
			return last
		}
	}
	return last
}

// RetryWithBackoff runs fn with backoff sleep between attempts.
func RetryWithBackoff(fn func() error, b *Backoff, opts ...RetryOption) error {
	o := &retryOptions{times: defaultRetryTimes}
	for _, opt := range opts {
		opt(o)
	}
	var last error
	for i := 0; i < o.times; i++ {
		last = fn()
		if last == nil {
			return nil
		}
		if o.acceptable != nil && !o.acceptable(last) {
			return last
		}
		if i < o.times-1 {
			time.Sleep(b.Delay(i))
		}
	}
	return last
}

// ── Backoff ───────────────────────────────────────────────────────────────────

// Backoff computes exponential backoff delays with jitter.
type Backoff struct {
	baseDelay time.Duration
	maxDelay  time.Duration
	factor    float64
	jitter    float64
}

// BackoffOption customises a Backoff.
type BackoffOption func(*Backoff)

// WithBackoffBase sets the base delay (default 1s).
func WithBackoffBase(d time.Duration) BackoffOption {
	return func(b *Backoff) { b.baseDelay = d }
}

// WithBackoffMax sets the maximum delay (default 120s).
func WithBackoffMax(d time.Duration) BackoffOption {
	return func(b *Backoff) { b.maxDelay = d }
}

// WithBackoffFactor sets the growth factor (default 1.6).
func WithBackoffFactor(f float64) BackoffOption {
	return func(b *Backoff) { b.factor = f }
}

// WithBackoffJitter sets the jitter fraction in [0,1] (default 0.2).
func WithBackoffJitter(j float64) BackoffOption {
	return func(b *Backoff) { b.jitter = j }
}

// NewBackoff creates a Backoff with sensible defaults.
func NewBackoff(opts ...BackoffOption) *Backoff {
	b := &Backoff{
		baseDelay: 1 * time.Second,
		maxDelay:  120 * time.Second,
		factor:    1.6,
		jitter:    0.2,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Delay returns the wait duration for the given retry index (0-based).
func (b *Backoff) Delay(retry int) time.Duration {
	d := float64(b.baseDelay)
	max := float64(b.maxDelay)
	for i := 0; i < retry; i++ {
		d *= b.factor
		if d > max {
			d = max
			break
		}
	}
	if d > max {
		d = max
	}
	if b.jitter > 0 {
		d *= 1 + b.jitter*(rand.Float64()*2-1)
	}
	if d < 0 {
		return 0
	}
	return time.Duration(d)
}
