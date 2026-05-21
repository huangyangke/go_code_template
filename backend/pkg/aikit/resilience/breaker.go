package resilience

import (
	"errors"
	"time"

	"github.com/sony/gobreaker"
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
}

type gobreakerBreaker struct {
	cb *gobreaker.CircuitBreaker
}

func New(c *Config) Breaker {
	if c.Name == "" {
		panic("resilience: breaker Name is required")
	}
	if c.MaxRequests <= 0 {
		c.MaxRequests = 1
	}
	if c.RequestVolumeThreshold <= 0 {
		c.RequestVolumeThreshold = 20
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

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
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
	})

	return &gobreakerBreaker{cb: cb}
}

var ErrCircuitOpen = gobreaker.ErrOpenState

func IsCircuitOpen(err error) bool {
	return errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)
}

func (b *gobreakerBreaker) Do(run func() error, fallback func(error) error) error {
	_, err := b.cb.Execute(func() (interface{}, error) {
		return nil, run()
	})
	if err != nil && fallback != nil {
		return fallback(err)
	}
	return err
}
