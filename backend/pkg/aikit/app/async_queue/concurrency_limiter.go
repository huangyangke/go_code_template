package async_queue

import (
	"context"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

// ConcurrencyLimiter L2 端点级并发限制接口.
type ConcurrencyLimiter interface {
	// TryAcquire 尝试获取端点并发令牌.
	// 参数：ctx - 上下文，endpoint - 端点路径.
	// 返回值：acquired - 是否获取成功, err - Redis 操作失败时的错误.
	TryAcquire(ctx context.Context, endpoint string) (bool, error)
	// Release 释放端点并发令牌.
	// 参数：ctx - 上下文, endpoint - 端点路径.
	// 返回值：err - 释放失败时的错误.
	Release(ctx context.Context, endpoint string) error
}

// ================================
// NoopConcurrencyLimiter — 不限流
// ================================.

// NoopConcurrencyLimiter 不限流的空实现.
type NoopConcurrencyLimiter struct{}

func (n *NoopConcurrencyLimiter) TryAcquire(_ context.Context, _ string) (bool, error) {
	return true, nil
}
func (n *NoopConcurrencyLimiter) Release(_ context.Context, _ string) error { return nil }

// ================================
// LocalConcurrencyLimiter — 单机模式
// ================================.

// LocalConcurrencyLimiter 单机模式，基于内存计数的端点并发限制器.
type LocalConcurrencyLimiter struct {
	mu           sync.Mutex
	counts       map[string]int
	limits       map[string]int
	defaultLimit int
}

// NewLocalConcurrencyLimiter 创建单机并发限制器.
// 参数：limits - 各端点并发上限映射, defaultLimit - 未配置端点的默认上限（0 表示不限）.
// 返回值：*LocalConcurrencyLimiter - 单机并发限制器实例.
func NewLocalConcurrencyLimiter(limits map[string]int, defaultLimit int) *LocalConcurrencyLimiter {
	return &LocalConcurrencyLimiter{
		counts:       make(map[string]int),
		limits:       limits,
		defaultLimit: defaultLimit,
	}
}

func (l *LocalConcurrencyLimiter) limit(endpoint string) int {
	if v, ok := l.limits[endpoint]; ok {
		return v
	}
	return l.defaultLimit
}

func (l *LocalConcurrencyLimiter) TryAcquire(_ context.Context, endpoint string) (bool, error) {
	lim := l.limit(endpoint)
	if lim <= 0 {
		return true, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[endpoint] >= lim {
		return false, nil
	}
	l.counts[endpoint]++
	return true, nil
}

func (l *LocalConcurrencyLimiter) Release(_ context.Context, endpoint string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[endpoint] > 0 {
		l.counts[endpoint]--
	}
	return nil
}

// ================================
// RedisConcurrencyLimiter — 分布式模式（Lua 原子操作）
// ================================.

// acquireScript 原子 check-and-increment.
var acquireScript = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local cur = tonumber(redis.call('GET', key) or '0')
if cur < limit then
    redis.call('INCR', key)
    redis.call('EXPIRE', key, ttl)
    return 1
end
return 0
`)

// releaseScript 原子 decrement（不低于 0）.
var releaseScript = redis.NewScript(`
local key = KEYS[1]
local cur = tonumber(redis.call('GET', key) or '0')
if cur > 0 then
    redis.call('DECR', key)
end
return 1
`)

// RedisConcurrencyLimiter 分布式模式，基于 Redis Lua 原子操作的端点并发限制器.
type RedisConcurrencyLimiter struct {
	rdb          redis.Cmdable
	limits       map[string]int
	defaultLimit int
	keyPrefix    string
	idleTTL      int
}

// NewRedisConcurrencyLimiter 创建分布式并发限制器.
// 参数：rdb - Redis 客户端, limits - 各端点并发上限映射, defaultLimit - 未配置端点的默认上限, keyPrefix - Redis key 前缀, idleTTL - 计数器空闲 TTL（秒）.
// 返回值：*RedisConcurrencyLimiter - 分布式并发限制器实例.
func NewRedisConcurrencyLimiter(
	rdb redis.Cmdable,
	limits map[string]int,
	defaultLimit int,
	keyPrefix string,
	idleTTL int,
) *RedisConcurrencyLimiter {
	return &RedisConcurrencyLimiter{
		rdb:          rdb,
		limits:       limits,
		defaultLimit: defaultLimit,
		keyPrefix:    keyPrefix,
		idleTTL:      idleTTL,
	}
}

func (r *RedisConcurrencyLimiter) key(endpoint string) string {
	return fmt.Sprintf("%s:%s", r.keyPrefix, endpoint)
}

func (r *RedisConcurrencyLimiter) limit(endpoint string) int {
	if v, ok := r.limits[endpoint]; ok {
		return v
	}
	return r.defaultLimit
}

func (r *RedisConcurrencyLimiter) TryAcquire(ctx context.Context, endpoint string) (bool, error) {
	lim := r.limit(endpoint)
	if lim <= 0 {
		return true, nil
	}
	res, err := acquireScript.Run(ctx, r.rdb,
		[]string{r.key(endpoint)},
		lim, r.idleTTL,
	).Int()
	if err != nil {
		return false, err // return error so callers can observe Redis failures
	}
	return res == 1, nil
}

func (r *RedisConcurrencyLimiter) Release(ctx context.Context, endpoint string) error {
	_, err := releaseScript.Run(ctx, r.rdb,
		[]string{r.key(endpoint)},
	).Int()
	return err
}
