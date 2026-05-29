package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangyangke/go-aikit/internal/testutil"
)

func TestPrometheus(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(Prometheus())
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
}

func TestRequestLog(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(RequestLog())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
}

func TestTokenAuth_MissingHeader(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return token == "valid", nil
	}))
	r.GET("/secret", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/secret", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusUnauthorized)
}

func TestTokenAuth_ValidToken(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return token == "mytoken", nil
	}))
	r.GET("/secret", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
}

func TestTokenAuth_Whitelist(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return false, nil
	}, "/public"))
	r.GET("/public/resource", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/public/resource", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
}

func TestTokenAuth_InvalidScheme(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return true, nil
	}))
	r.GET("/secret", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusUnauthorized)
}

func TestTokenAuth_VerifyError(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return false, context.Canceled
	}))
	r.GET("/secret", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusUnauthorized)
}

// ── OrVerify ──────────────────────────────────────────────────────────────────

func TestOrVerify_ASucceeds_BNotCalled(t *testing.T) {
	bCalled := false
	a := func(ctx context.Context, token string) (bool, error) {
		return true, nil
	}
	b := func(ctx context.Context, token string) (bool, error) {
		bCalled = true
		return true, nil
	}
	verify := OrVerify(a, b)
	ok, err := verify(context.Background(), "any")
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.False(t, bCalled, "b should not be called when a succeeds")
}

func TestOrVerify_AFailsBSucceeds_ReturnsTrue(t *testing.T) {
	a := func(ctx context.Context, token string) (bool, error) {
		return false, nil
	}
	b := func(ctx context.Context, token string) (bool, error) {
		return true, nil
	}
	verify := OrVerify(a, b)
	ok, err := verify(context.Background(), "any")
	assert.True(t, ok)
	assert.NoError(t, err)
}

func TestOrVerify_BothFail_ReturnsAError(t *testing.T) {
	errA := errors.New("a error")
	a := func(ctx context.Context, token string) (bool, error) {
		return false, errA
	}
	b := func(ctx context.Context, token string) (bool, error) {
		return false, errors.New("b error")
	}
	verify := OrVerify(a, b)
	ok, err := verify(context.Background(), "any")
	assert.False(t, ok)
	assert.Equal(t, errA, err)
}

func TestOrVerify_BothFailNoError_ReturnsFalseNil(t *testing.T) {
	a := func(ctx context.Context, token string) (bool, error) {
		return false, nil
	}
	b := func(ctx context.Context, token string) (bool, error) {
		return false, nil
	}
	verify := OrVerify(a, b)
	ok, err := verify(context.Background(), "any")
	assert.False(t, ok)
	assert.NoError(t, err)
}

// ── WithInternalToken ─────────────────────────────────────────────────────────

func TestWithInternalToken_MatchBypasses(t *testing.T) {
	innerCalled := false
	inner := func(ctx context.Context, token string) (bool, error) {
		innerCalled = true
		return false, nil
	}
	verify := WithInternalToken("secret", inner)
	ok, err := verify(context.Background(), "secret")
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.False(t, innerCalled, "inner should not be called on matching internal token")
}

func TestWithInternalToken_MismatchCallsInner(t *testing.T) {
	innerCalled := false
	inner := func(ctx context.Context, token string) (bool, error) {
		innerCalled = true
		return true, nil
	}
	verify := WithInternalToken("secret", inner)
	ok, err := verify(context.Background(), "other-token")
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.True(t, innerCalled, "inner should be called on mismatch")
}

func TestWithInternalToken_NilInner_MismatchReturnsFalse(t *testing.T) {
	verify := WithInternalToken("secret", nil)
	ok, err := verify(context.Background(), "wrong-token")
	assert.False(t, ok)
	assert.NoError(t, err)
}

// ── Prometheus path labeling ──────────────────────────────────────────────────

func TestPrometheus_PathNoParams(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(Prometheus())
	r.GET("/api/users", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/api/users", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
}

func TestPrometheus_PathWithParam_UsesFullPath(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(Prometheus())
	r.GET("/api/users/:id", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/api/users/123", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
}

// ── Prometheus: path without params ──────────────────────────────────────────

func TestPrometheus_PathWithNoParams(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(Prometheus())
	r.GET("/api/users", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/api/users", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
}

func TestPrometheus_PathWithParams(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(Prometheus())
	r.GET("/api/users/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/api/users/123", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
}

// ── RateLimit: window <= 0 defaults to 1s ────────────────────────────────────

func TestRateLimit_ZeroWindow_DefaultsToSecond(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	r := testutil.NewGinRouter(t)
	r.Use(RateLimit(rdb, RateLimitConfig{
		Limit:  100,
		Window: 0,
	}))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
}

// ── RequestID: long task_id truncated ────────────────────────────────────────

func TestRequestID_LongTaskID_Truncated(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(RequestID())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	longID := strings.Repeat("a", 200)
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", longID)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusOK)
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
}

// ── TokenAuth: verify error path ─────────────────────────────────────────────

func TestTokenAuth_VerifyError_401(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return false, errors.New("upstream error")
	}))
	r.GET("/secure", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusUnauthorized)
}

// ── Prometheus: unmatched route (path=="") ────────────────────────────────────

func TestPrometheus_UnmatchedRoute(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(Prometheus())

	req, _ := http.NewRequest(http.MethodGet, "/unknown-route", nil)
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusNotFound)
}

// ── TokenAuth: verify ok=false ────────────────────────────────────────────────

func TestTokenAuth_VerifyFalse_401(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return false, nil
	}))
	r.GET("/secure", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	w := testutil.ServeRequest(r, req)
	testutil.AssertStatus(t, w, http.StatusUnauthorized)
}
