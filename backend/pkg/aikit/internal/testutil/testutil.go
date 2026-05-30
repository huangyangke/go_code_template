// Package testutil 测试基础设施工具，提供 Redis、SQLite、Gin 等测试辅助.
package testutil

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/benbjohnson/clock"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	dbredis "github.com/huangyangke/go-aikit/database/redis"
)

// NewMiniRedis 创建 miniredis 服务器，测试结束时自动关闭.
// 参数：t - 测试实例.
// 返回值：mr - miniredis 服务器, client - go-redis 客户端.
func NewMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return mr, client
}

// NewRedis 创建基于 miniredis 的 dbredis.Redis 实例.
// 参数：t - 测试实例.
// 返回值：r - dbredis.Redis 实例, mr - miniredis 服务器.
func NewRedis(t *testing.T) (*dbredis.Redis, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	r := dbredis.MustNew(&dbredis.Config{
		Name:  "testutil",
		Type:  dbredis.StandaloneType,
		Addrs: []string{mr.Addr()},
	})
	t.Cleanup(func() { _ = r.Close() })
	return r, mr
}

// NewSQLiteDB 创建基于 GORM 的内存 SQLite 数据库.
// 参数：t - 测试实例.
// 返回值：gorm.DB 实例，测试结束时自动关闭连接.
func NewSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("testutil: open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// NewGinRouter 创建测试模式的 Gin 引擎（含 Recovery 中间件）.
// 参数：t - 测试实例.
// 返回值：gin.Engine 实例.
func NewGinRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	return r
}

// ServeRequest 将 HTTP 请求发送到 Gin 引擎并返回响应记录器.
// 参数：engine - Gin 引擎, req - HTTP 请求.
// 返回值：httptest.ResponseRecorder 响应记录器.
func ServeRequest(engine *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// NewTempConfig 将 YAML 内容写入临时文件并返回路径.
// 参数：t - 测试实例, content - YAML 内容.
// 返回值：临时文件路径.
func NewTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("testutil: write config: %v", err)
	}
	return path
}

// NewSQLiteDBWithModels 创建内存 SQLite 数据库并自动迁移指定模型.
// 参数：t - 测试实例, models - 需要迁移的模型列表.
// 返回值：gorm.DB 实例.
func NewSQLiteDBWithModels(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db := NewSQLiteDB(t)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("testutil: auto migrate: %v", err)
	}
	return db
}

// NewJSONRequest 构造带 JSON 请求体和 Content-Type 头的 HTTP 请求.
// 参数：t - 测试实例, method - HTTP 方法, path - 请求路径, body - 请求体 (nil 时无 body).
// 返回值：http.Request 实例.
func NewJSONRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("testutil: marshal request body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, path, r)
	if err != nil {
		t.Fatalf("testutil: new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

// ---- Context 工具.

// NewTestContext 创建带 cancel 的测试上下文，测试结束自动 cancel.
// 参数：t - 测试实例.
// 返回值：ctx - 上下文, cancel - 取消函数（可主动调用提前取消）.
func NewTestContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx, cancel
}

// NewContextWithTimeout 创建带超时的测试上下文，测试结束自动清理.
// 参数：t - 测试实例, d - 超时时长.
// 返回值：ctx - 上下文, cancel - 取消函数.
func NewContextWithTimeout(t *testing.T, d time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx, cancel
}

// ---- 时间控制.

// NewClock 创建起始时间为 now 的 mock 时钟.
// 参数：t - 测试实例, now - 时钟起始时间.
// 返回值：mock 时钟实例，可使用 Add/Set 控制时间流逝.
func NewClock(t *testing.T, now time.Time) *clock.Mock {
	t.Helper()
	m := clock.NewMock()
	m.Set(now)
	return m
}

// ---- 随机数据.

// UniqueID 生成唯一测试标识，格式 "test_<16 位随机 hex>".
func UniqueID() string {
	b := make([]byte, 8)
	_, _ = cryptorand.Read(b)
	return "test_" + hex.EncodeToString(b)
}

// RandomString 生成 n 字节长度的随机 hex 字符串（2n 字符）.
// 参数：n - 随机字节数.
func RandomString(n int) string {
	b := make([]byte, n)
	_, _ = cryptorand.Read(b)
	return hex.EncodeToString(b)
}

// ---- HTTP 响应断言 ----.

// AssertStatus 断言响应状态码.
// 参数：t - 测试实例, w - 响应记录器, expected - 期望状态码.
func AssertStatus(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	t.Helper()
	assert.Equal(t, expected, w.Code, "unexpected HTTP status")
}

// AssertJSONContains 断言响应 JSON 包含指定字段（仅校验键存在与值相等）.
// 参数：t - 测试实例, w - 响应记录器, expected - 期望包含的键值对.
func AssertJSONContains(t *testing.T, w *httptest.ResponseRecorder, expected map[string]any) {
	t.Helper()
	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err, "response body is not valid JSON")
	for k, v := range expected {
		got, ok := body[k]
		if !ok {
			t.Errorf("response JSON missing key %q", k)
			continue
		}
		assert.Equalf(t, v, got, "mismatch on key %q", k)
	}
}

// ParseJSONResponse 将响应 body 解析为目标类型.
// 类型参数：T - 期望的响应结构类型.
// 参数：t - 测试实例, w - 响应记录器.
// 返回值：解析后的 T 实例.
func ParseJSONResponse[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	err := json.Unmarshal(w.Body.Bytes(), &v)
	require.NoError(t, err, "response body is not valid JSON")
	return v
}

// ---- Mock HTTP 服务 ----.

// NewMockHTTPServer 创建带指定 handler 的测试 HTTP 服务器，测试结束自动关闭.
// 参数：t - 测试实例, handler - HTTP handler.
// 返回值：httptest.Server 实例（.URL 可获取服务器地址）.
func NewMockHTTPServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}
