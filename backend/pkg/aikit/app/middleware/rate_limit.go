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
	Limit   int
	Window  time.Duration
	KeyFunc func(*gin.Context) string
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

// RateLimit implements a sliding window rate limiter backed by Redis.
// Uses a Lua script for atomic INCR+EXPIRE.
// Fails open if Redis is unavailable.
func RateLimit(rdb redis.Cmdable, cfg RateLimitConfig) gin.HandlerFunc {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *gin.Context) string { return c.ClientIP() }
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
		windowKey := fmt.Sprintf("aikit:ratelimit:%s:%d",
			cfg.KeyFunc(c),
			time.Now().UnixNano()/int64(window),
		)
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
