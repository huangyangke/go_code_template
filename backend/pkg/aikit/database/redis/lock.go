package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

var (
	lockScript = NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
    return "OK"
else
    return redis.call("SET", KEYS[1], ARGV[1], "NX", "PX", ARGV[2])
end`)

	unlockScript = NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end`)
)

// LockOption 分布式锁配置函数.
type LockOption func(*Lock)

// WithWatchdog 启用看门狗自动续期，以 expire/3 为间隔刷新 TTL.
// 参数：ctx - 看门狗上下文，取消即停止续期.
// 返回值：LockOption - 配置函数.
func WithWatchdog(ctx context.Context) LockOption {
	return func(l *Lock) {
		l.watchdogCtx = ctx
	}
}

// Lock Redis 分布式锁，使用随机令牌与 Lua 脚本防止误释放.
type Lock struct {
	redis  *Redis
	key    string
	expire time.Duration
	token  string

	watchdogCtx    context.Context
	watchdogCancel context.CancelFunc
}

// NewLock 创建绑定到当前 Redis 实例的分布式锁，键名自动拼接 KeyPrefix.
// 参数：_ - 上下文（保留占位）, key - 键名, expire - 锁持有时间, opts - 配置选项.
// 返回值：*Lock - 锁实例.
func (r *Redis) NewLock(_ context.Context, key string, expire time.Duration, opts ...LockOption) *Lock {
	l := &Lock{
		redis:  r,
		key:    key,
		expire: expire,
		token:  randToken(),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// TryLock 尝试获取锁，成功时启动看门狗.
// 返回值：ok - 是否获取成功, err - 操作失败时的错误.
func (l *Lock) TryLock() (bool, error) {
	ctx := context.Background()
	val, err := l.redis.ScriptRun(ctx, lockScript, []string{l.key}, l.token, l.expire.Milliseconds())
	if err == Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("redis lock TryLock %q: %w", l.key, err)
	}
	reply, _ := val.(string)
	if reply == "OK" {
		l.startWatchdog()
		return true, nil
	}
	return false, nil
}

// Unlock 释放锁，仅当本实例仍持有锁时才删除，同时停止看门狗.
// 返回值：ok - 是否释放成功, err - 操作失败时的错误.
func (l *Lock) Unlock() (bool, error) {
	l.stopWatchdog()
	ctx := context.Background()
	val, err := l.redis.ScriptRun(ctx, unlockScript, []string{l.key}, l.token)
	if err != nil {
		return false, fmt.Errorf("redis lock Unlock %q: %w", l.key, err)
	}
	n, _ := val.(int64)
	return n == 1, nil
}

// Refresh 重置锁的 TTL 为原始过期时间.
// 返回值：err - 锁不再被本实例持有或操作失败时的错误.
func (l *Lock) Refresh() error {
	ctx := context.Background()
	val, err := l.redis.ScriptRun(ctx, lockScript, []string{l.key}, l.token, l.expire.Milliseconds())
	if err != nil {
		return fmt.Errorf("redis lock Refresh %q: %w", l.key, err)
	}
	reply, _ := val.(string)
	if reply != "OK" {
		return fmt.Errorf("redis lock Refresh %q: lock not owned", l.key)
	}
	return nil
}

func (l *Lock) startWatchdog() {
	if l.watchdogCtx == nil {
		return
	}
	ctx, cancel := context.WithCancel(l.watchdogCtx)
	l.watchdogCancel = cancel

	interval := l.expire / 3
	if interval < 50*time.Millisecond {
		interval = 50 * time.Millisecond
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := l.Refresh(); err != nil {
					return
				}
			}
		}
	}()
}

func (l *Lock) stopWatchdog() {
	if l.watchdogCancel != nil {
		l.watchdogCancel()
		l.watchdogCancel = nil
	}
}

func randToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
