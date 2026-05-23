package redis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/go-template/pkg/aikit/metrics"
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
		success := err == nil || errors.Is(err, redis.Nil)
		metrics.ObserveRedis(h.name, success, time.Since(start))
		return err
	}
}

func (h *prometheusHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmds)
		success := err == nil
		metrics.ObserveRedis(h.name, success, time.Since(start))
		return err
	}
}
