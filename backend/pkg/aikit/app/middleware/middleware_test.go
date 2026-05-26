package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
