package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCORS_DefaultWildcards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://example.com")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_CredentialsWithWildcard_EchoesOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	}))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://example.com")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// When AllowCredentials=true, Access-Control-Allow-Origin must NOT be "*"
	// per the CORS spec — browsers reject it. The middleware should echo the
	// request Origin instead.
	assert.Equal(t, "http://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORS_CredentialsWithSpecificOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(CORSConfig{
		AllowOrigins:     []string{"http://example.com"},
		AllowCredentials: true,
	}))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://example.com")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "http://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORS_CredentialsWithWildcard_DisallowedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// When configured with specific origins + credentials,
	// non-matching origins should not get an Allow-Origin header.
	r := gin.New()
	r.Use(CORS(CORSConfig{
		AllowOrigins:     []string{"http://allowed.com"},
		AllowCredentials: true,
	}))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://evil.com")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_Preflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "http://example.com")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}
