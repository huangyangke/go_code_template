package testutil_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangyangke/go-aikit/internal/testutil"
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

func TestNewSQLiteDBWithModels(t *testing.T) {
	type Product struct {
		ID    uint
		Title string
		Price float64
	}
	db := testutil.NewSQLiteDBWithModels(t, &Product{})

	require.NoError(t, db.Create(&Product{Title: "widget", Price: 9.99}).Error)

	var p Product
	require.NoError(t, db.First(&p).Error)
	assert.Equal(t, "widget", p.Title)
	assert.InDelta(t, 9.99, p.Price, 0.001)
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

func TestServeRequest_NotFound(t *testing.T) {
	r := testutil.NewGinRouter(t)
	req, _ := http.NewRequest("GET", "/no-such-route", nil)
	w := testutil.ServeRequest(r, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestNewTempConfig(t *testing.T) {
	content := "key: value\nother: 123"
	path := testutil.NewTempConfig(t, content)

	assert.FileExists(t, path)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(got))
}

func TestNewJSONRequest(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	r := testutil.NewGinRouter(t)
	r.POST("/echo", func(c *gin.Context) {
		var p payload
		if err := c.ShouldBindJSON(&p); err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		c.String(http.StatusOK, p.Name)
	})

	req := testutil.NewJSONRequest(t, http.MethodPost, "/echo", payload{Name: "hello"})
	w := testutil.ServeRequest(r, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello", w.Body.String())
}

func TestNewJSONRequest_NilBody(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.DELETE("/item", func(c *gin.Context) {
		c.String(http.StatusNoContent, "")
	})

	req := testutil.NewJSONRequest(t, http.MethodDelete, "/item", nil)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	w := testutil.ServeRequest(r, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}
