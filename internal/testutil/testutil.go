// Package testutil 测试基础设施工具，提供 Redis、SQLite、Gin 等测试辅助.
package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
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
	r := dbredis.New(&dbredis.Config{
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
