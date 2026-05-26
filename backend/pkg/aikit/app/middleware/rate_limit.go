package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/example/go-template/pkg/aikit/app/response"
)

type RateLimitConfig struct {
	Limit     int
	Window    time.Duration
	KeyFunc   func(*gin.Context) string
	KeyPrefix string // Optional prefix for Redis key. Defaults to "aikit:ratelimit".
}

// rateLimitScript atomically increments and sets TTL in a single Redis call.
// Returns the new count after increment.
var rateLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

// RateLimit implements a fixed-window rate limiter backed by Redis.
// Uses a Lua script for atomic INCR+PEXPIRE (millisecond precision).
// Fails open if Redis is unavailable.
func RateLimit(rdb redis.Cmdable, cfg RateLimitConfig) gin.HandlerFunc {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *gin.Context) string { return c.ClientIP() }
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "aikit:ratelimit"
	}
	return func(c *gin.Context) {
		window := cfg.Window
		if window <= 0 {
			window = time.Second
		}
		windowMs := window.Milliseconds()
		if windowMs <= 0 {
			windowMs = 1
		}
		windowKey := fmt.Sprintf("%s:%s:%d",
			cfg.KeyPrefix,
			cfg.KeyFunc(c),
			time.Now().UnixNano()/int64(window),
		)
		// Use background context instead of request context to ensure the
		// rate limit count completes even if the client disconnects early.
		// This prevents users from bypassing rate limits by terminating requests.
		ctx := context.Background()
		count, err := rateLimitScript.Run(ctx, rdb,
			[]string{windowKey},
			windowMs,
		).Int()
		if err != nil {
			// fail-open: Redis error → allow request
			c.Next()
			return
		}
		if count > cfg.Limit {
			response.RateLimited(c)
			c.Abort()
			return
		}
		c.Next()
	}
}
