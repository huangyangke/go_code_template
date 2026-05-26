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

// LockOption configures Lock behavior.
type LockOption func(*Lock)

// WithWatchdog enables automatic TTL renewal. The watchdog goroutine refreshes
// the lock at expire/3 intervals and stops when Unlock is called or the context
// is cancelled.
func WithWatchdog(ctx context.Context) LockOption {
	return func(l *Lock) {
		l.watchdogCtx = ctx
	}
}

// Lock is a Redis-backed distributed lock using random token + Lua scripts
// to prevent accidental release by non-owners.
type Lock struct {
	redis  *Redis
	key    string
	expire time.Duration
	token  string

	watchdogCtx    context.Context
	watchdogCancel context.CancelFunc
}

// NewLock returns a Lock bound to this Redis instance.
// key will be prefixed by Redis config KeyPrefix (same as other commands).
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

// TryLock attempts to acquire the lock. Returns (true, nil) on success,
// (false, nil) if the lock is already held, or (false, err) on error.
// If watchdog is enabled and acquisition succeeds, the watchdog goroutine starts.
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

// Unlock releases the lock only if this instance still owns it.
// Returns (true, nil) on success, (false, nil) if not the owner.
// Stops the watchdog if running.
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

// Refresh resets the TTL of the lock to its original expire duration.
// Returns an error if the lock is no longer owned by this instance.
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
