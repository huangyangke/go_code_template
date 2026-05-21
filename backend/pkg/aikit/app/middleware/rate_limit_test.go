package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	// Use an unavailable Redis to trigger fail-open behavior
	rdb := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:19999"})

	r := gin.New()
	r.Use(RateLimit(rdb, RateLimitConfig{Limit: 5, Window: time.Minute}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimit_FailOpen(t *testing.T) {
	// Redis unavailable → fail open, all requests pass
	rdb := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:19999"})

	r := gin.New()
	r.Use(RateLimit(rdb, RateLimitConfig{Limit: 1, Window: time.Minute}))
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
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
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return false, nil
	}))
	r.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	r.ServeHTTP(w, req)
	assert.Contains(t, w.Body.String(), "10007")
}
