package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Pipeliner = redis.Pipeliner
type Script = redis.Script

func (r *Redis) SetEx(ctx context.Context, key string, value interface{}, expire time.Duration) error {
	return r.client.SetEx(ctx, r.key(key), value, expire).Err()
}

func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, r.key(key)).Result()
}

func (r *Redis) GetNoNil(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, r.key(key)).Result()
	if err == redis.Nil {
		return val, nil
	}
	return val, err
}

func (r *Redis) Del(ctx context.Context, key string) (int, error) {
	v, err := r.client.Del(ctx, r.key(key)).Result()
	return int(v), err
}

func (r *Redis) Exists(ctx context.Context, key string) (bool, error) {
	v, err := r.client.Exists(ctx, r.key(key)).Result()
	return v == 1, err
}

func (r *Redis) Expire(ctx context.Context, key string, expire time.Duration) error {
	return r.client.Expire(ctx, r.key(key), expire).Err()
}

func (r *Redis) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, r.key(key)).Result()
}

func (r *Redis) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return r.client.IncrBy(ctx, r.key(key), n).Result()
}

func (r *Redis) SetNx(ctx context.Context, key string, value interface{}, expire time.Duration) (bool, error) {
	return r.client.SetNX(ctx, r.key(key), value, expire).Result()
}

func (r *Redis) HSet(ctx context.Context, key, field, value string) error {
	return r.client.HSet(ctx, r.key(key), field, value).Err()
}

func (r *Redis) HGet(ctx context.Context, key, field string) (string, error) {
	return r.client.HGet(ctx, r.key(key), field).Result()
}

func (r *Redis) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, r.key(key)).Result()
}

func (r *Redis) HDel(ctx context.Context, key string, fields ...string) (bool, error) {
	v, err := r.client.HDel(ctx, r.key(key), fields...).Result()
	return v > 0, err
}

func (r *Redis) Publish(ctx context.Context, channel string, message any) (int, error) {
	v, err := r.client.Publish(ctx, r.key(channel), message).Result()
	return int(v), err
}

func (r *Redis) Pipeline() Pipeliner {
	return r.client.Pipeline()
}

func NewScript(script string) *Script {
	return redis.NewScript(script)
}

func (r *Redis) ScriptRun(ctx context.Context, script *Script, keys []string, args ...any) (any, error) {
	return script.Run(ctx, r.client, r.keys(keys...), args...).Result()
}

func (r *Redis) Ping(ctx context.Context) bool {
	v, err := r.client.Ping(ctx).Result()
	return err == nil && v == "PONG"
}

// PubSub represents a Redis PubSub connection
type PubSub struct {
	pubsub *redis.PubSub
}

// NewPubSub creates a new PubSub instance
func NewPubSub(client redis.Cmdable, channel string) *PubSub {
	switch c := client.(type) {
	case *redis.Client:
		return &PubSub{
			pubsub: c.Subscribe(context.Background(), channel),
		}
	case *redis.ClusterClient:
		return &PubSub{
			pubsub: c.Subscribe(context.Background(), channel),
		}
	default:
		return nil
	}
}

// Close closes the PubSub connection
func (ps *PubSub) Close() error {
	if ps.pubsub == nil {
		return nil
	}
	return ps.pubsub.Close()
}

// Channel returns the subscription channel
func (ps *PubSub) Channel() <-chan *redis.Message {
	if ps.pubsub == nil {
		return nil
	}
	return ps.pubsub.Channel()
}
