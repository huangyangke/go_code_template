package resilience

import (
	"errors"
	"time"

	"github.com/sony/gobreaker"

	"github.com/example/go-template/pkg/aikit/metrics"
)

type Config struct {
	Name                   string        `yaml:"name"`
	MaxRequests            uint32        `yaml:"max_requests"`
	Interval               time.Duration `yaml:"interval"`
	RequestVolumeThreshold int           `yaml:"request_volume_threshold"`
	SleepWindow            time.Duration `yaml:"sleep_window"`
	ErrorPercentThreshold  int           `yaml:"error_percent_threshold"`
}

type Breaker interface {
	Do(run func() error, fallback func(error) error) error
	Allow() (done func(success bool), err error)
}

type gobreakerBreaker struct {
	cb *gobreaker.TwoStepCircuitBreaker
}

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

var ErrCircuitOpen = gobreaker.ErrOpenState

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
