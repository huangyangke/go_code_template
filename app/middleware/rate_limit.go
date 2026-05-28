package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/huangyangke/go-aikit/app/response"
	"github.com/huangyangke/go-aikit/log"
)

// RateLimitConfig 固定窗口限流配置.
type RateLimitConfig struct {
	Limit     int
	Window    time.Duration
	KeyFunc   func(*gin.Context) string
	KeyPrefix string // Redis 键前缀，缺省 "aikit:ratelimit".
}

// rateLimitScript 单次 Redis 调用原子递增并设置 TTL.
var rateLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

// RateLimit 返回基于 Redis 的固定窗口限流中间件.
// 使用 Lua 脚本原子 INCR+PEXPIRE（毫秒精度）.
// Redis 不可用时放通请求并记录日志.
// 参数：rdb - Redis 客户端, cfg - 限流配置.
// 返回值：gin 中间件 HandlerFunc.
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
		windowKey := fmt.Sprintf("%s:%s:%d",
			cfg.KeyPrefix,
			cfg.KeyFunc(c),
			time.Now().UnixNano()/int64(window),
		)
		// 使用 background context 而非请求 context，确保客户端断开后计数仍完成
		ctx := context.Background()
		count, err := rateLimitScript.Run(ctx, rdb,
			[]string{windowKey},
			windowMs,
		).Int()
		if err != nil {
			// 放通：Redis 异常时允许请求，但记录日志以便观察
			log.Warn("[RateLimit][redis_error]: %v", err)
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
