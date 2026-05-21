package testutil

import (
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

	dbredis "github.com/example/go-template/pkg/aikit/database/redis"
)

// NewMiniRedis creates a miniredis server that auto-closes when the test ends.
// Returns both the miniredis server (for time manipulation etc.) and a go-redis Client.
func NewMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return mr, client
}

// NewRedis creates a miniredis-backed dbredis.Redis instance.
// Returns both the dbredis.Redis and the underlying miniredis server.
func NewRedis(t *testing.T) (*dbredis.Redis, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	r := dbredis.New(&dbredis.Config{
		Type:  dbredis.StandaloneType,
		Addrs: []string{mr.Addr()},
	})
	t.Cleanup(func() { r.Close() })
	return r, mr
}

// NewSQLiteDB creates an in-memory SQLite database via GORM.
// Automatically closes the underlying sql.DB when the test ends.
func NewSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("testutil: open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

// NewGinRouter creates a Gin engine in test mode with recovery middleware.
func NewGinRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	return r
}

// ServeRequest sends an HTTP request through a Gin engine and returns the recorder.
func ServeRequest(engine *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// NewTempConfig writes YAML content to a temporary file and returns its path.
func NewTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("testutil: write config: %v", err)
	}
	return path
}
