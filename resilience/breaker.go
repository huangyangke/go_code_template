// Package resilience 断路器与重试等容错机制.
package resilience

import (
	"errors"
	"time"

	"github.com/sony/gobreaker"

	"github.com/huangyangke/go-aikit/metrics"
)

// Config 断路器配置.
type Config struct {
	Name                   string        `yaml:"name"`
	MaxRequests            uint32        `yaml:"max_requests"`
	Interval               time.Duration `yaml:"interval"`
	RequestVolumeThreshold int           `yaml:"request_volume_threshold"`
	SleepWindow            time.Duration `yaml:"sleep_window"`
	ErrorPercentThreshold  int           `yaml:"error_percent_threshold"`
}

// Breaker 断路器接口，提供同步执行和手动报告两种使用模式.
type Breaker interface {
	// Do 执行 run 函数，失败时调用 fallback 降级.
	// 参数：run - 业务函数, fallback - 降级函数（可为 nil）.
	// 返回值：err - 执行或降级失败时的错误.
	Do(run func() error, fallback func(error) error) error
	// Allow 手动获取执行许可，返回的 done 回调用于报告成功或失败.
	// 参数：无.
	// 返回值：done - 报告结果的回调（传入 true 表示成功, false 表示失败）, err - 断路器打开时返回错误.
	Allow() (done func(success bool), err error)
}

type gobreakerBreaker struct {
	cb *gobreaker.TwoStepCircuitBreaker
}

// New 根据配置创建断路器实例.
// 参数：c - 断路器配置，Name 为必填项.
// 返回值：Breaker - 断路器实例.
func New(c *Config) Breaker {
	if c.Name == "" {
		panic("resilience: breaker Name is required")
	}
	if c.MaxRequests <= 0 {
		c.MaxRequests = 1
	}
	if c.RequestVolumeThreshold <= 0 {
		c.RequestVolumeThreshold = 5
	}
	if c.SleepWindow <= 0 {
		c.SleepWindow = 5 * time.Second
	}
	if c.Interval <= 0 {
		c.Interval = 10 * time.Second
	}
	if c.ErrorPercentThreshold <= 0 {
		c.ErrorPercentThreshold = 50
	}

	threshold := uint32(c.RequestVolumeThreshold)
	errPct := float64(c.ErrorPercentThreshold) / 100.0

	cb := gobreaker.NewTwoStepCircuitBreaker(gobreaker.Settings{
		Name:        c.Name,
		MaxRequests: c.MaxRequests,
		Interval:    c.Interval,
		Timeout:     c.SleepWindow,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < threshold {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= errPct
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			metrics.ObserveCircuitBreakerState(name, int(to))
		},
	})

	return &gobreakerBreaker{cb: cb}
}

// ErrCircuitOpen 断路器打开状态错误.
var ErrCircuitOpen = gobreaker.ErrOpenState

// IsCircuitOpen 判断错误是否由断路器打开引起.
// 参数：err - 待检查的错误.
// 返回值：bool - 为 true 表示断路器处于打开或半打开过量状态.
func IsCircuitOpen(err error) bool {
	return errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)
}

func (b *gobreakerBreaker) Allow() (func(success bool), error) {
	done, err := b.cb.Allow()
	if err != nil {
		if IsCircuitOpen(err) {
			metrics.ObserveCircuitBreakerCall(b.cb.Name(), "rejected")
		}
		return nil, err
	}
	return func(success bool) {
		if success {
			metrics.ObserveCircuitBreakerCall(b.cb.Name(), "success")
		} else {
			metrics.ObserveCircuitBreakerCall(b.cb.Name(), "failure")
		}
		done(success)
	}, nil
}

func (b *gobreakerBreaker) Do(run func() error, fallback func(error) error) error {
	done, err := b.Allow()
	if err != nil {
		if fallback != nil {
			return fallback(err)
		}
		return err
	}
	if err := run(); err != nil {
		done(false)
		if fallback != nil {
			return fallback(err)
		}
		return err
	}
	done(true)
	return nil
}
