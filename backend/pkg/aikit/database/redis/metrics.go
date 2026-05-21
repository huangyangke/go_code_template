package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/go-template/pkg/aikit/metrics"
)

var (
	metricRedisCounter = metrics.NewCounterVec(&metrics.CounterVecOpts{
		Namespace: "redis",
		Name:      "requests_total",
		Help:      "Redis requests total.",
		Labels:    []string{"datasource", "success"},
	})

	metricRedisLatency = metrics.NewHistogramVec(&metrics.HistogramVecOpts{
		Namespace: "redis",
		Name:      "request_duration_seconds",
		Help:      "Redis request duration in seconds.",
		Labels:    []string{"datasource", "success"},
		Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
	})
)

type prometheusHook struct {
	name string
}

type startKey struct{}

// NewPrometheusHook returns a redis.Hook that records metrics.
func NewPrometheusHook(name string) redis.Hook {
	return &prometheusHook{name: name}
}

func (h *prometheusHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *prometheusHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmd)
		success := "1"
		if err != nil && err != redis.Nil {
			success = "0"
		}
		elapsed := time.Since(start).Seconds()
		metricRedisCounter.Inc(h.name, success)
		metricRedisLatency.Observe(elapsed, h.name, success)
		return err
	}
}

func (h *prometheusHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
