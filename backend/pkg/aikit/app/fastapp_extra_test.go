package app

import (
	"context"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/go-template/pkg/aikit/app/auth"
	"github.com/example/go-template/pkg/aikit/app/httpclient"
	"github.com/example/go-template/pkg/aikit/cache"
	"github.com/example/go-template/pkg/aikit/config"
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
	assert.Equal(t, 30*time.Second, fa.svrCfg.WriteTimeout)
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
