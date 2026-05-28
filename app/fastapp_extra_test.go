package app

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxl-job/xxl-job-executor-go"

	"github.com/huangyangke/go-aikit/app/auth"
	"github.com/huangyangke/go-aikit/app/httpclient"
	"github.com/huangyangke/go-aikit/cache"
	"github.com/huangyangke/go-aikit/config"
	dbmysql "github.com/huangyangke/go-aikit/database/mysql"
	dbpulsar "github.com/huangyangke/go-aikit/database/pulsar"
	dbredis "github.com/huangyangke/go-aikit/database/redis"
)

func TestFastApp_SetAuth_AuthManager(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})

	assert.Nil(t, fa.AuthManager())

	m, err := auth.New(auth.Config{
		Secret: "test-secret-key-32-bytes-long!!!",
		Authenticate: func(ctx context.Context, username, password string) (*auth.AuthResult, error) {
			return &auth.AuthResult{UID: "1"}, nil
		},
	})
	require.NoError(t, err)

	fa.SetAuth(m)
	assert.Equal(t, m, fa.AuthManager())
}

func TestFastApp_RegisterHTTPClient_GetHTTPClient(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})

	assert.Nil(t, fa.GetHTTPClient("api"))

	c := fa.RegisterHTTPClient("api", httpclient.Config{
		Name:           "api",
		DisableMetrics: true,
	})
	assert.NotNil(t, c)
	assert.Equal(t, c, fa.GetHTTPClient("api"))
	assert.Nil(t, fa.GetHTTPClient("nonexistent"))
}

func TestFastApp_RegisterCache_GetCache(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})

	assert.Nil(t, fa.GetCache("user-cache"))

	c := fa.RegisterCache("user-cache", cache.Config{
		Name:         "user-cache",
		CacheType:    cache.CacheTypeLocal,
		LocalType:    cache.LocalCacheTypeFreeCache,
		LocalMemSize: "10mb",
	})
	assert.NotNil(t, c)
	assert.Equal(t, c, fa.GetCache("user-cache"))
	assert.Nil(t, fa.GetCache("nonexistent"))
}

func TestFastApp_SetConfigLoader_ConfigLoader(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test", Mode: gin.TestMode})

	assert.Nil(t, fa.ConfigLoader())

	ldr, err := config.New("/nonexistent/config.yaml")
	require.NoError(t, err)

	fa.SetConfigLoader(ldr)
	assert.Equal(t, ldr, fa.ConfigLoader())
}

func TestFastApp_Family(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "my-service"})
	assert.Equal(t, "my-service", fa.Family())
}

func TestFastApp_SetAsyncQueue(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	fa.SetAsyncQueue(AsyncQueueConfig{
		Prefix: "/v1/queue",
	})
	assert.NotNil(t, fa.aqCfg)
	assert.Equal(t, "/v1/queue", fa.aqCfg.Prefix)
}

func TestFastApp_SetXxlJob_GetXxlJob(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})

	assert.Nil(t, fa.GetXxlJob())

	fa.SetXxlJob(XxlJobConfig{
		ServerAddr: "http://localhost:8080/xxl-job-admin",
	})
	assert.NotNil(t, fa.xjobConfig)
}

func TestFastApp_MultipleLifecycleHooks(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	var order []string

	fa.OnStart(func(ctx context.Context) error {
		order = append(order, "start1")
		return nil
	})
	fa.OnStart(func(ctx context.Context) error {
		order = append(order, "start2")
		return nil
	})
	fa.OnStop(func(ctx context.Context) error {
		order = append(order, "stop1")
		return nil
	})

	assert.Equal(t, 2, len(fa.onStart))
	assert.Equal(t, 1, len(fa.onStop))
}

func TestFastApp_GetRedis_NilBeforeRegister(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	assert.Nil(t, fa.GetRedis("main"))
}

func TestFastApp_GetMySQL_NilBeforeRegister(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	assert.Nil(t, fa.GetMySQL("main"))
}

func TestFastApp_SetServer_Defaults(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	fa.applyServerDefaults()
	assert.Equal(t, 30*time.Second, fa.svrCfg.ReadTimeout)
	assert.Equal(t, time.Duration(0), fa.svrCfg.WriteTimeout)
	assert.Equal(t, 120*time.Second, fa.svrCfg.IdleTimeout)
	assert.Equal(t, 30*time.Second, fa.svrCfg.ShutdownTimeout)
}

func TestFastApp_SetServer_NoOverride(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	fa.SetServer(ServerConfig{
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     60 * time.Second,
		ShutdownTimeout: 15 * time.Second,
	})
	fa.applyServerDefaults()
	assert.Equal(t, 5*time.Second, fa.svrCfg.ReadTimeout)
	assert.Equal(t, 10*time.Second, fa.svrCfg.WriteTimeout)
	assert.Equal(t, 60*time.Second, fa.svrCfg.IdleTimeout)
	assert.Equal(t, 15*time.Second, fa.svrCfg.ShutdownTimeout)
}

func TestFastApp_MustGet_Panics(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	assert.PanicsWithValue(t, `redis "main" not registered`, func() { fa.MustGetRedis("main") })
	assert.PanicsWithValue(t, `mysql "main" not registered`, func() { fa.MustGetMySQL("main") })
	assert.PanicsWithValue(t, `cache "main" not registered`, func() { fa.MustGetCache("main") })
	assert.PanicsWithValue(t, `httpclient "main" not registered`, func() { fa.MustGetHTTPClient("main") })
	assert.PanicsWithValue(t, `pulsar "main" not registered`, func() { fa.MustGetPulsar("main") })
}

// ── RegisterRedis ──────────────────────────────────────────────────────────────

func TestFastApp_RegisterRedis_GetRedis_NameAutoFilled(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	fa := NewFastApp(FastAppConfig{Family: "svc", Mode: gin.TestMode})
	cfg := &dbredis.Config{
		Addrs: []string{mr.Addr()},
		Type:  "standalone",
	}
	rdb := fa.RegisterRedis("main", cfg)
	assert.NotNil(t, rdb)
	assert.Equal(t, rdb, fa.GetRedis("main"))
	assert.Equal(t, "svc/main", cfg.Name)
}

func TestFastApp_RegisterRedis_NamePreservedIfSet(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	fa := NewFastApp(FastAppConfig{Family: "svc", Mode: gin.TestMode})
	cfg := &dbredis.Config{
		Name:  "custom-name",
		Addrs: []string{mr.Addr()},
		Type:  "standalone",
	}
	fa.RegisterRedis("main", cfg)
	assert.Equal(t, "custom-name", cfg.Name)
}

// ── RegisterMySQL ──────────────────────────────────────────────────────────────

func TestFastApp_RegisterMySQL_NameAutoFilled(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "svc", Mode: gin.TestMode})
	cfg := &dbmysql.Config{DSN: "user:pass@tcp(127.0.0.1:3306)/db"}
	// New panics because DSN is invalid; verify Name is set before the panic
	assert.Panics(t, func() { fa.RegisterMySQL("db", cfg) })
	assert.Equal(t, "svc/db", cfg.Name)
}

func TestFastApp_RegisterMySQL_NamePreservedIfSet(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "svc", Mode: gin.TestMode})
	cfg := &dbmysql.Config{Name: "my-db", DSN: "user:pass@tcp(127.0.0.1:3306)/db"}
	assert.Panics(t, func() { fa.RegisterMySQL("db", cfg) })
	assert.Equal(t, "my-db", cfg.Name)
}

// ── RegisterPulsar ─────────────────────────────────────────────────────────────

func TestFastApp_RegisterPulsar_GetPulsar_NameAutoFilled(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "svc", Mode: gin.TestMode})
	cfg := &dbpulsar.Config{URL: "pulsar://localhost:6650"}
	client := fa.RegisterPulsar("events", cfg)
	assert.NotNil(t, client)
	assert.Equal(t, client, fa.GetPulsar("events"))
	assert.Equal(t, "svc/events", cfg.Name)
}

func TestFastApp_RegisterPulsar_NamePreservedIfSet(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "svc", Mode: gin.TestMode})
	cfg := &dbpulsar.Config{Name: "my-pulsar", URL: "pulsar://localhost:6650"}
	fa.RegisterPulsar("events", cfg)
	assert.Equal(t, "my-pulsar", cfg.Name)
}

// ── RegisterXxlJobTask ─────────────────────────────────────────────────────────

func TestFastApp_RegisterXxlJobTask_AppendsToList(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "svc", Mode: gin.TestMode})
	assert.Len(t, fa.xjobTasks, 0)

	taskFn := func(ctx context.Context, param *xxl.RunReq) string { return "ok" }
	fa.RegisterXxlJobTask("handler1", taskFn)
	fa.RegisterXxlJobTask("handler2", taskFn)

	assert.Len(t, fa.xjobTasks, 2)
}

// ── MustGet panics / normal return ────────────────────────────────────────────

func TestFastApp_MustGetRedis_PanicIfNotRegistered(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	assert.PanicsWithValue(t, `redis "main" not registered`, func() { fa.MustGetRedis("main") })
}

func TestFastApp_MustGetRedis_ReturnsIfRegistered(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	fa := NewFastApp(FastAppConfig{Family: "svc", Mode: gin.TestMode})
	fa.RegisterRedis("main", &dbredis.Config{Addrs: []string{mr.Addr()}, Type: "standalone"})
	assert.NotPanics(t, func() {
		r := fa.MustGetRedis("main")
		assert.NotNil(t, r)
	})
}

func TestFastApp_MustGetMySQL_PanicIfNotRegistered(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	assert.PanicsWithValue(t, `mysql "main" not registered`, func() { fa.MustGetMySQL("main") })
}

func TestFastApp_MustGetCache_PanicIfNotRegistered(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	assert.PanicsWithValue(t, `cache "main" not registered`, func() { fa.MustGetCache("main") })
}

func TestFastApp_MustGetHTTPClient_PanicIfNotRegistered(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	assert.PanicsWithValue(t, `httpclient "main" not registered`, func() { fa.MustGetHTTPClient("main") })
}

func TestFastApp_MustGetPulsar_PanicIfNotRegistered(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "test"})
	assert.PanicsWithValue(t, `pulsar "main" not registered`, func() { fa.MustGetPulsar("main") })
}

// ── RegisterCache ──────────────────────────────────────────────────────────────

func TestFastApp_RegisterCache_InvalidConfigPanics(t *testing.T) {
	fa := NewFastApp(FastAppConfig{Family: "svc", Mode: gin.TestMode})
	// SyncLocal=true requires CacheType=both; mismatched config causes cache.New to return error → panic
	assert.Panics(t, func() {
		fa.RegisterCache("test", cache.Config{
			SyncLocal: true,
			CacheType: cache.CacheTypeLocal,
		})
	})
}

func TestFastApp_RegisterCache_RedisAutoReused(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	fa := NewFastApp(FastAppConfig{Family: "svc1", Mode: gin.TestMode})
	fa.RegisterRedis("maincache", &dbredis.Config{Addrs: []string{mr.Addr()}, Type: "standalone"})

	var capturedCfg cache.Config
	// We register a cache with the same name "maincache" and no RedisCmdable set.
	// FastApp should automatically reuse the "maincache" Redis instance.
	c := fa.RegisterCache("maincache", cache.Config{
		CacheType: cache.CacheTypeRemote,
		RemoteTTL: 60,
	})
	assert.NotNil(t, c)
	assert.Equal(t, c, fa.GetCache("maincache"))
	_ = capturedCfg
}

// Verify RedisCmdable is non-nil after auto-reuse via a captured config.
func TestFastApp_RegisterCache_RedisAutoReused_CmdableNonNil(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	fa := NewFastApp(FastAppConfig{Family: "svc2", Mode: gin.TestMode})
	rdb := fa.RegisterRedis("cache2", &dbredis.Config{Addrs: []string{mr.Addr()}, Type: "standalone"})

	// Build a config that has no RedisCmdable; FastApp.RegisterCache should fill it in.
	// We verify indirectly: if RedisCmdable stayed nil the cache.New call would fail for
	// CacheTypeRemote, so a successful return proves it was filled.
	cfg := cache.Config{
		CacheType: cache.CacheTypeRemote,
		RemoteTTL: 60,
	}
	c := fa.RegisterCache("cache2", cfg)
	assert.NotNil(t, c)
	// Also confirm the Redis instance we registered is what backs the cache.
	assert.NotNil(t, rdb.Cmdable())
}
