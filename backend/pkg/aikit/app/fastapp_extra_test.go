package app

import (
	"context"
	"testing"

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

	c, err := fa.RegisterCache("user-cache", cache.Config{
		Name:      "user-cache",
		CacheType: cache.CacheTypeLocal,
		LocalType: cache.LocalCacheTypeFreeCache,
		LocalMemSize: "10mb",
	})
	require.NoError(t, err)
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
