package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestNewFastApp(t *testing.T) {
	fa := NewFastApp(FastAppConfig{
		Family: "test-service",
		Host:        "127.0.0.1",
		Port:        9090,
	})

	assert.NotNil(t, fa)
	assert.Equal(t, "test-service", fa.Family())
	assert.NotNil(t, fa.Engine())
}

func TestNewFastApp_Defaults(t *testing.T) {
	fa := NewFastApp(FastAppConfig{
		Family: "test",
	})

	assert.Equal(t, "0.0.0.0", fa.cfg.Host)
	assert.Equal(t, 8080, fa.cfg.Port)
}

func TestFastApp_MiddlewareConfig(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	fa.SetMiddlewares(MiddlewareConfig{
		EnableRequestID:  true,
		EnableRequestLog: true,
		EnablePrometheus: true,
	})

	assert.True(t, fa.mwCfg.EnableRequestID)
	assert.True(t, fa.mwCfg.EnablePrometheus)
}

func TestFastApp_LifecycleHooks(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})

	fa.OnStart(func(ctx context.Context) error {
		return nil
	})
	fa.OnStop(func(ctx context.Context) error {
		return nil
	})

	assert.Equal(t, 1, len(fa.onStart))
	assert.Equal(t, 1, len(fa.onStop))
}

func TestFastApp_RouteRegistrar(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})

	fa.SetRouteRegistrar(func(r *gin.Engine) {
		r.GET("/custom", func(c *gin.Context) {})
	})

	assert.NotNil(t, fa.routeRegistrar)
}

func TestFastApp_BuildMiddlewareBeforeRoutes(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{EnableRequestID: true})

	fa.buildMiddlewareChain()
	fa.registerBuiltinEndpoints()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	fa.Engine().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
}

func TestFastApp_HealthCheck_NoInstances(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.buildMiddlewareChain()
	fa.registerBuiltinEndpoints()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	fa.Engine().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Response should contain health status structure
	assert.Contains(t, w.Body.String(), "healthy")
}

func TestFastApp_PprofEnabled(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{EnablePprof: true})

	fa.buildMiddlewareChain()
	fa.registerBuiltinEndpoints()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	fa.Engine().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestFastApp_PprofDisabled(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{EnablePprof: false})

	fa.buildMiddlewareChain()
	fa.registerBuiltinEndpoints()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	fa.Engine().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFastApp_SwaggerEnabled(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{EnableSwagger: true})

	fa.buildMiddlewareChain()
	fa.registerBuiltinEndpoints()

	req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
	w := httptest.NewRecorder()
	fa.Engine().ServeHTTP(w, req)

	// Swagger returns 200 even without doc files (renders UI)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestFastApp_SwaggerDisabled(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{EnableSwagger: false})

	fa.buildMiddlewareChain()
	fa.registerBuiltinEndpoints()

	req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
	w := httptest.NewRecorder()
	fa.Engine().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFastApp_Pulsar_NotRegistered(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	// GetPulsar returns nil before registration
	assert.Nil(t, fa.GetPulsar("demo"))
}
