package resilience

import (
	"math/rand/v2"
	"time"
)

const defaultRetryTimes = 3

// Acceptable 判断错误是否可接受（可接受则不再重试）.
type Acceptable func(err error) bool

type retryOptions struct {
	times      int
	acceptable Acceptable
}

// RetryOption 重试配置选项.
type RetryOption func(*retryOptions)

// WithTimes 设置最大重试次数.
// 参数：n - 最大尝试次数.
// 返回值：RetryOption - 重试选项.
func WithTimes(n int) RetryOption {
	return func(o *retryOptions) { o.times = n }
}

// WithAcceptable 设置可接受错误判定函数，返回 true 表示该错误无需重试.
// 参数：fn - 错误判定函数.
// 返回值：RetryOption - 重试选项.
func WithAcceptable(fn Acceptable) RetryOption {
	return func(o *retryOptions) { o.acceptable = fn }
}

// Retry 重复执行 fn 直到成功或达到最大次数.
// 参数：fn - 待执行的函数, opts - 重试选项.
// 返回值：err - 最后一次执行的错误，成功时为 nil.
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

// RetryWithBackoff 带退避间隔的重试，每次失败后等待一定时间再重试.
// 参数：fn - 待执行的函数, b - 退避策略, opts - 重试选项.
// 返回值：err - 最后一次执行的错误，成功时为 nil.
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

// ── Backoff ────────────────────────────────────────────────────────────────────.

// Backoff 指数退避策略，支持抖动.
type Backoff struct {
	baseDelay time.Duration
	maxDelay  time.Duration
	factor    float64
	jitter    float64
}

// BackoffOption 退避策略配置选项.
type BackoffOption func(*Backoff)

// WithBackoffBase 设置初始延迟时间（默认 1s）.
// 参数：d - 初始延迟时长.
// 返回值：BackoffOption - 退避选项.
func WithBackoffBase(d time.Duration) BackoffOption {
	return func(b *Backoff) { b.baseDelay = d }
}

// WithBackoffMax 设置最大延迟时间（默认 120s）.
// 参数：d - 最大延迟时长.
// 返回值：BackoffOption - 退避选项.
func WithBackoffMax(d time.Duration) BackoffOption {
	return func(b *Backoff) { b.maxDelay = d }
}

// WithBackoffFactor 设置指数增长因子（默认 1.6）.
// 参数：f - 增长因子.
// 返回值：BackoffOption - 退避选项.
func WithBackoffFactor(f float64) BackoffOption {
	return func(b *Backoff) { b.factor = f }
}

// WithBackoffJitter 设置抖动比例，范围 [0,1]（默认 0.2）.
// 参数：j - 抖动比例.
// 返回值：BackoffOption - 退避选项.
func WithBackoffJitter(j float64) BackoffOption {
	return func(b *Backoff) { b.jitter = j }
}

// NewBackoff 创建带有默认值的退避策略实例.
// 参数：opts - 退避选项.
// 返回值：*Backoff - 退避策略实例.
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

// Delay 计算指定重试次数对应的等待时长.
// 参数：retry - 重试序号（从 0 开始）.
// 返回值：time.Duration - 等待时长.
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
