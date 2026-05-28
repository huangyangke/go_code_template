package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimit_WithRealRedis(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(rdb, RateLimitConfig{Limit: 2, Window: time.Minute}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Third request should be rate limited
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimit_TTLIsAlwaysSet(t *testing.T) {
	// Verify that every rate limit key always has a TTL set,
	// even for the first request (where INCR+EXPIRE used to be non-atomic).
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(rdb, RateLimitConfig{Limit: 5, Window: time.Minute}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Find the rate limit key
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
	defer rdb.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(rdb, RateLimitConfig{Limit: 5, Window: 500 * time.Millisecond}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

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
