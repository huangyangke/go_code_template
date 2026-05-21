package testutil_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/go-template/pkg/aikit/internal/testutil"
)

func TestNewMiniRedis(t *testing.T) {
	mr, client := testutil.NewMiniRedis(t)
	require.NotNil(t, mr)
	require.NotNil(t, client)

	ctx := context.Background()
	require.NoError(t, client.Set(ctx, "key", "value", 0).Err())
	val, err := client.Get(ctx, "key").Result()
	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

func TestNewRedis(t *testing.T) {
	r, mr := testutil.NewRedis(t)
	require.NotNil(t, r)
	require.NotNil(t, mr)
	assert.True(t, r.Ping(context.Background()))
}

func TestNewSQLiteDB(t *testing.T) {
	db := testutil.NewSQLiteDB(t)
	require.NotNil(t, db)

	type Item struct {
		ID   uint
		Name string
	}
	require.NoError(t, db.AutoMigrate(&Item{}))
	require.NoError(t, db.Create(&Item{Name: "test"}).Error)

	var item Item
	require.NoError(t, db.First(&item).Error)
	assert.Equal(t, "test", item.Name)
}

func TestNewGinRouter(t *testing.T) {
	r := testutil.NewGinRouter(t)
	require.NotNil(t, r)

	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	req, _ := http.NewRequest("GET", "/ping", nil)
	w := testutil.ServeRequest(r, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "pong", w.Body.String())
}

func TestNewTempConfig(t *testing.T) {
	path := testutil.NewTempConfig(t, "key: value")
	assert.FileExists(t, path)
}
