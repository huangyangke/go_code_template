package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangyangke/go-aikit/app/async_queue"
	"github.com/huangyangke/go-aikit/app/httpclient"
	"github.com/huangyangke/go-aikit/app/middleware"
	"github.com/huangyangke/go-aikit/cache"
	dbmysql "github.com/huangyangke/go-aikit/database/mysql"
	dbpulsar "github.com/huangyangke/go-aikit/database/pulsar"
	dbredis "github.com/huangyangke/go-aikit/database/redis"
)

func TestNewFastApp(t *testing.T) {
	fa := NewFastApp(FastAppConfig{
		Family: "test-service",
		Host:   "127.0.0.1",
		Port:   9090,
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

// ── buildMiddlewareChain panics ───────────────────────────────────────────────

func TestFastApp_BuildMiddlewareChain_Panics_RateLimitZero(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{
		EnableRateLimit: true,
		RateLimitRDB:    rdb,
		RateLimitConfig: middleware.RateLimitConfig{Limit: 0},
	})
	assert.Panics(t, func() { fa.buildMiddlewareChain() })
}

func TestFastApp_BuildMiddlewareChain_Panics_RateLimitNilRDB(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{
		EnableRateLimit: true,
		RateLimitRDB:    nil,
		RateLimitConfig: middleware.RateLimitConfig{Limit: 10},
	})
	assert.Panics(t, func() { fa.buildMiddlewareChain() })
}

func TestFastApp_BuildMiddlewareChain_Panics_TokenAuthNoVerifier(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{
		EnableTokenAuth: true,
		// No TokenVerifyFunc, no ExtraVerifyFunc, no InternalToken
	})
	assert.Panics(t, func() { fa.buildMiddlewareChain() })
}

// ── buildMiddlewareChain normal branches ──────────────────────────────────────

func TestFastApp_BuildMiddlewareChain_RequestID(t *testing.T) {
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

func TestFastApp_BuildMiddlewareChain_TokenAuth_Whitelist(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{
		EnableTokenAuth: true,
		TokenVerifyFunc: func(ctx context.Context, token string) (bool, error) {
			return token == "valid-token", nil
		},
		TokenWhitelist: []string{"/public"},
	})
	fa.buildMiddlewareChain()

	fa.Engine().GET("/public/resource", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	fa.Engine().GET("/secret", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Whitelisted path should pass without token
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/public/resource", nil)
	fa.Engine().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Non-whitelisted path without token should return 401
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/secret", nil)
	fa.Engine().ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

func TestFastApp_BuildMiddlewareChain_TokenAuth_InternalToken(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{
		EnableTokenAuth: true,
		InternalToken:   "my-internal-secret",
	})
	fa.buildMiddlewareChain()

	fa.Engine().GET("/api/data", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Request with internal token should pass
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Authorization", "Bearer my-internal-secret")
	fa.Engine().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Request without token should fail
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	fa.Engine().ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

func TestFastApp_BuildMiddlewareChain_TokenAuth_ExtraVerifyFunc(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{
		EnableTokenAuth: true,
		TokenVerifyFunc: func(ctx context.Context, token string) (bool, error) {
			return false, nil // primary always fails
		},
		ExtraVerifyFunc: func(ctx context.Context, token string) (bool, error) {
			return token == "extra-token", nil // extra passes on "extra-token"
		},
	})
	fa.buildMiddlewareChain()

	fa.Engine().GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Token accepted by extra verifier should succeed (OR logic)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer extra-token")
	fa.Engine().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Token rejected by both verifiers should fail
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("Authorization", "Bearer unknown-token")
	fa.Engine().ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

// ── setupAsyncQueue ───────────────────────────────────────────────────────────

func TestFastApp_SetupAsyncQueue_DefaultStreamKey(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	fa := NewFastApp(FastAppConfig{Family: "myapp", Mode: gin.TestMode})
	fa.SetAsyncQueue(AsyncQueueConfig{
		RedisClient: rdb,
	})

	err := fa.setupAsyncQueue()
	require.NoError(t, err)
	// stream key is now derived internally from Family via buildStreamKey
}

func TestFastApp_SetupAsyncQueue_DefaultPrefix(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	fa := NewFastApp(FastAppConfig{Family: "myapp", Mode: gin.TestMode})
	fa.SetAsyncQueue(AsyncQueueConfig{
		RedisClient: rdb,
	})

	err := fa.setupAsyncQueue()
	require.NoError(t, err)
	assert.Equal(t, "/v1/async_queue", fa.aqCfg.Prefix)
}

// ── printRoutes ───────────────────────────────────────────────────────────────

func TestFastApp_PrintRoutes_NoPanic(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.Engine().GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	fa.Engine().POST("/users", func(c *gin.Context) { c.Status(http.StatusCreated) })
	assert.NotPanics(t, func() { fa.printRoutes() })
}

// ── buildMiddlewareChain: uncovered branches ──────────────────────────────────

func TestBuildMiddlewareChain_EnablePrometheus(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{EnablePrometheus: true})
	assert.NotPanics(t, func() { fa.buildMiddlewareChain() })
}

func TestBuildMiddlewareChain_EnableRequestLog(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{EnableRequestLog: true})
	assert.NotPanics(t, func() { fa.buildMiddlewareChain() })
}

func TestBuildMiddlewareChain_EnableRateLimit_OK(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{
		EnableRateLimit: true,
		RateLimitRDB:    rdb,
		RateLimitConfig: middleware.RateLimitConfig{Limit: 100, Window: time.Second},
	})
	assert.NotPanics(t, func() { fa.buildMiddlewareChain() })
}

func TestBuildMiddlewareChain_EnableSwagger_Whitelisted(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.SetMiddlewares(MiddlewareConfig{
		EnableTokenAuth: true,
		InternalToken:   "secret",
		EnableSwagger:   true,
	})
	assert.NotPanics(t, func() { fa.buildMiddlewareChain() })
}

// ── healthCheckHandler branches ───────────────────────────────────────────────

func TestHealthCheckHandler_AllHealthy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	rdb := fa.RegisterRedis("main", &dbredis.Config{
		Addrs: []string{mr.Addr()},
		Type:  "standalone",
	})
	_ = rdb

	handler := fa.healthCheckHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/healthz", nil)
	handler(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHealthCheckHandler_RedisUnhealthy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())

	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	fa.RegisterRedis("main", &dbredis.Config{
		Addrs: []string{mr.Addr()},
		Type:  "standalone",
	})

	mr.Close() // close after registration so ping fails

	handler := fa.healthCheckHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/healthz", nil)
	handler(c)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// ── setupAsyncQueue: SchedulerConfig and PelConfig branches ──────────────────

func TestSetupAsyncQueue_WithSchedulerAndPelConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	fa := NewFastApp(FastAppConfig{Family: "svc", Mode: gin.TestMode})
	fa.SetAsyncQueue(AsyncQueueConfig{
		RedisClient:     rdb,
		Endpoints:       map[string]async_queue.EndpointConfig{},
		SchedulerConfig: &async_queue.SchedulerConfig{WorkerCapacity: 5},
		PelConfig:       &async_queue.PelConfig{MinIdle: 10 * time.Minute, MaxRetries: 2},
	})
	require.NotPanics(t, func() {
		require.NoError(t, fa.setupAsyncQueue())
	})
	assert.NotNil(t, fa.consumer)
}

// ── MustGet happy path ─────────────────────────────────────────────────────────

func TestMustGetMySQL_HappyPath(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	cfg := &dbmysql.Config{DSN: "user:pass@tcp(127.0.0.1:3306)/db"}
	assert.Panics(t, func() { fa.RegisterMySQL("db", cfg) }) // panics on connect
	// After panic cfg.Name is set but not registered; MustGet should panic
	assert.Panics(t, func() { fa.MustGetMySQL("db") })
}

func TestMustGetCache_HappyPath(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	c := fa.RegisterCache("mc", cache.Config{
		Name:         "mc",
		CacheType:    cache.CacheTypeLocal,
		LocalType:    cache.LocalCacheTypeFreeCache,
		LocalMemSize: "1mb",
	})
	assert.NotNil(t, c)
	assert.Equal(t, c, fa.MustGetCache("mc"))
}

func TestMustGetHTTPClient_HappyPath(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	c := fa.RegisterHTTPClient("api", httpclient.Config{Name: "api", DisableMetrics: true})
	assert.Equal(t, c, fa.MustGetHTTPClient("api"))
}

func TestMustGetPulsar_HappyPath(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})
	c := fa.RegisterPulsar("p", &dbpulsar.Config{URL: "pulsar://localhost:6650"})
	assert.Equal(t, c, fa.MustGetPulsar("p"))
}
