package middleware

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/huangyangke/go-aikit/internal/testutil"
)

func TestCORS_DefaultWildcards(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(CORS())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://example.com")
	w := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, w, http.StatusOK)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_CredentialsWithWildcard_EchoesOrigin(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(CORS(CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	}))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://example.com")
	w := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, w, http.StatusOK)
	// When AllowCredentials=true, Access-Control-Allow-Origin must NOT be "*"
	// per the CORS spec — browsers reject it. The middleware should echo the
	// request Origin instead.
	assert.Equal(t, "http://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORS_CredentialsWithSpecificOrigin(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(CORS(CORSConfig{
		AllowOrigins:     []string{"http://example.com"},
		AllowCredentials: true,
	}))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://example.com")
	w := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, w, http.StatusOK)
	assert.Equal(t, "http://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORS_CredentialsWithWildcard_DisallowedOrigin(t *testing.T) {
	// When configured with specific origins + credentials,
	// non-matching origins should not get an Allow-Origin header.
	r := testutil.NewGinRouter(t)
	r.Use(CORS(CORSConfig{
		AllowOrigins:     []string{"http://allowed.com"},
		AllowCredentials: true,
	}))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://evil.com")
	w := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, w, http.StatusOK)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_Preflight(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.Use(CORS())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req, _ := http.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "http://example.com")
	w := testutil.ServeRequest(r, req)

	testutil.AssertStatus(t, w, http.StatusNoContent)
}
