package middleware

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangyangke/go-aikit/internal/testutil"
)

func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	// 使用不可达的 Redis 触发 fail-open 行为.
	rdb := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:19999"})

	r := testutil.NewGinRouter(t)
	r.Use(RateLimit(rdb, RateLimitConfig{Limit: 5, Window: time.Minute}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
}

func TestRateLimit_FailOpen(t *testing.T) {
	// Redis 不可用时 fail open，所有请求放行.
	rdb := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:19999"})

	r := testutil.NewGinRouter(t)
	r.Use(RateLimit(rdb, RateLimitConfig{Limit: 1, Window: time.Minute}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodGet, "/api", nil)
		w := testutil.ServeRequest(r, req)
		testutil.AssertStatus(t, w, http.StatusOK)
	}
}

func TestRateLimit_CustomKeyPrefix(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	t.Cleanup(mr.Close)

	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	r := testutil.NewGinRouter(t)
	r.Use(RateLimit(rdb, RateLimitConfig{
		Limit:     1,
		Window:    time.Minute,
		KeyPrefix: "myapp:rl",
	}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 首次请求放行，后续限流.
	for i := 0; i < 1; i++ {
		req, _ := http.NewRequest(http.MethodGet, "/api", nil)
		w := testutil.ServeRequest(r, req)
		testutil.AssertStatus(t, w, http.StatusOK)
	}
	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusTooManyRequests)

	// 校验 Redis 中使用了自定义前缀.
	keys := mr.Keys()
	assert.NotEmpty(t, keys)
	assert.True(t, len(keys[0]) > 8 && keys[0][:8] == "myapp:rl",
		"expected key with prefix 'myapp:rl', got %q", keys[0])
}

func TestRateLimit_DefaultKeyFunc(t *testing.T) {
	cfg := RateLimitConfig{Limit: 10, Window: time.Minute}
	assert.Nil(t, cfg.KeyFunc)
	// After middleware creation, KeyFunc is set internally — just verify handler creation doesn't panic
	rdb := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:19999"})
	assert.NotPanics(t, func() {
		RateLimit(rdb, cfg)
	})
}

func TestTokenAuth_NoHeader_Returns401Body(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return false, nil
	}))
	r.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	w := testutil.ServeRequest(r, req)
	assert.Contains(t, w.Body.String(), "10007")
}
