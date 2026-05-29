package middleware

import (
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangyangke/go-aikit/internal/testutil"
)

func TestRateLimit_WithRealRedis(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	r := testutil.NewGinRouter(t)
	r.Use(RateLimit(rdb, RateLimitConfig{Limit: 2, Window: time.Minute}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 前 2 次请求应成功
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "/api", nil)
		w := testutil.ServeRequest(r, req)
		testutil.AssertStatus(t, w, http.StatusOK)
	}

	// 第 3 次请求应被限流
	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusTooManyRequests)
}

func TestRateLimit_TTLIsAlwaysSet(t *testing.T) {
	// Verify that every rate limit key always has a TTL set,
	// even for the first request (where INCR+EXPIRE used to be non-atomic).
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	r := testutil.NewGinRouter(t)
	r.Use(RateLimit(rdb, RateLimitConfig{Limit: 5, Window: time.Minute}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)

	// 找到限流 key 并验证其 TTL
	keys := mr.Keys()
	found := false
	for _, key := range keys {
		if len(key) > 15 && key[:15] == "aikit:ratelimit" {
			ttl := mr.TTL(key)
			assert.True(t, ttl > 0, "rate limit key %s must have a TTL", key)
			found = true
		}
	}
	assert.True(t, found, "expected a rate_limit key to exist")
}

func TestRateLimit_PreservesSubSecondWindowTTL(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	r := testutil.NewGinRouter(t)
	r.Use(RateLimit(rdb, RateLimitConfig{Limit: 5, Window: 500 * time.Millisecond}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)

	// 验证亚秒级窗口的 TTL 保留
	keys := mr.Keys()
	found := false
	for _, key := range keys {
		if len(key) > 15 && key[:15] == "aikit:ratelimit" {
			ttl := mr.TTL(key)
			assert.True(t, ttl > 0, "rate limit key %s must have a TTL", key)
			assert.Less(t, ttl, time.Second, "rate limit key %s should preserve sub-second TTL", key)
			found = true
		}
	}
	assert.True(t, found, "expected a rate_limit key to exist")
}
