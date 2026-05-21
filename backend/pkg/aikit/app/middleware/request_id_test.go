package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGetTaskID_Empty(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", GetTaskID(ctx))
}

func TestWithTaskID_GetTaskID(t *testing.T) {
	ctx := WithTaskID(context.Background(), "task-abc-123")
	assert.Equal(t, "task-abc-123", GetTaskID(ctx))
}

func TestRequestID_GeneratesUUID(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		taskID, _ := c.Get("task_id")
		c.String(http.StatusOK, taskID.(string))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.String())
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
	assert.Equal(t, w.Body.String(), w.Header().Get("X-Request-ID"))
}

func TestRequestID_UsesProvidedHeader(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		taskID, _ := c.Get("task_id")
		c.String(http.StatusOK, taskID.(string))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "custom-id-456")
	r.ServeHTTP(w, req)

	assert.Equal(t, "custom-id-456", w.Body.String())
	assert.Equal(t, "custom-id-456", w.Header().Get("X-Request-ID"))
}

func TestRequestID_PropagatedToContext(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		id := GetTaskID(c.Request.Context())
		c.String(http.StatusOK, id)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "ctx-id-789")
	r.ServeHTTP(w, req)

	assert.Equal(t, "ctx-id-789", w.Body.String())
}
