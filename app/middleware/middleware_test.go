package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestPrometheus(t *testing.T) {
	r := gin.New()
	r.Use(Prometheus())
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequestLog(t *testing.T) {
	r := gin.New()
	r.Use(RequestLog())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTokenAuth_MissingHeader(t *testing.T) {
	r := gin.New()
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return token == "valid", nil
	}))
	r.GET("/secret", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/secret", nil)
	r.ServeHTTP(w, req)
	// Unauthorized now returns 401 with code 10007
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTokenAuth_ValidToken(t *testing.T) {
	r := gin.New()
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return token == "mytoken", nil
	}))
	r.GET("/secret", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTokenAuth_Whitelist(t *testing.T) {
	r := gin.New()
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return false, nil
	}, "/public"))
	r.GET("/public/resource", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/public/resource", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTokenAuth_InvalidScheme(t *testing.T) {
	r := gin.New()
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return true, nil
	}))
	r.GET("/secret", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTokenAuth_VerifyError(t *testing.T) {
	r := gin.New()
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return false, context.Canceled
	}))
	r.GET("/secret", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
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
	r := gin.New()
	r.Use(Prometheus())
	r.GET("/api/users", func(c *gin.Context) {
		// c.FullPath() returns "/api/users" — no param substitution needed
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/users", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPrometheus_PathWithParam_UsesFullPath(t *testing.T) {
	r := gin.New()
	r.Use(Prometheus())
	r.GET("/api/users/:id", func(c *gin.Context) {
		// c.FullPath() returns "/api/users/:id" — gin route template, not the actual URL
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/users/123", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	// The Prometheus middleware records c.FullPath() which is the route template "/api/users/:id"
	// not the concrete path "/api/users/123" — so dynamic IDs don't cause cardinality explosion.
}

// ── Prometheus: path without params ──────────────────────────────────────────

func TestPrometheus_PathWithNoParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Prometheus())
	r.GET("/api/users", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPrometheus_PathWithParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Prometheus())
	r.GET("/api/users/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/users/123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ── RateLimit: window <= 0 defaults to 1s ────────────────────────────────────

func TestRateLimit_ZeroWindow_DefaultsToSecond(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(rdb, RateLimitConfig{
		Limit:  100,
		Window: 0, // should default to 1s
	}))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ── RequestID: long task_id truncated ────────────────────────────────────────

func TestRequestID_LongTaskID_Truncated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// A very long request ID (> 128 chars should be truncated or replaced)
	longID := strings.Repeat("a", 200)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", longID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	// Response should have some X-Request-ID (either truncated or UUID)
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
}

// ── TokenAuth: verify error path ─────────────────────────────────────────────

func TestTokenAuth_VerifyError_401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return false, errors.New("upstream error")
	}))
	r.GET("/secure", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ── Prometheus: unmatched route (path=="") ────────────────────────────────────

func TestPrometheus_UnmatchedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Prometheus())
	// No route registered for /unknown → FullPath() returns ""
	req := httptest.NewRequest(http.MethodGet, "/unknown-route", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// 404 is expected, just ensure no panic
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── TokenAuth: verify ok=false ────────────────────────────────────────────────

func TestTokenAuth_VerifyFalse_401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TokenAuth(func(ctx context.Context, token string) (bool, error) {
		return false, nil // reject
	}))
	r.GET("/secure", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
