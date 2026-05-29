package testutil

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssertStatus(t *testing.T) {
	router := NewGinRouter(t)
	router.GET("/ok", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := NewJSONRequest(t, "GET", "/ok", nil)
	w := ServeRequest(router, req)

	AssertStatus(t, w, http.StatusOK)
}

func TestAssertJSONContains(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		router := NewGinRouter(t)
		router.GET("/data", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"id":   123,
				"name": "test",
				"tags": []string{"a", "b"},
			})
		})

		req := NewJSONRequest(t, "GET", "/data", nil)
		w := ServeRequest(router, req)

		AssertJSONContains(t, w, map[string]any{
			"id":   float64(123), // JSON 数字解析为 float64
			"name": "test",
		})
	})

	t.Run("missing key", func(t *testing.T) {
		router := NewGinRouter(t)
		router.GET("/data", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"id": 123})
		})

		req := NewJSONRequest(t, "GET", "/data", nil)
		w := ServeRequest(router, req)

		// 这个测试预期会失败，用 mockT 捕获
		mockT := &testing.T{}
		AssertJSONContains(mockT, w, map[string]any{"missing": "key"})
		assert.True(t, mockT.Failed(), "should fail when key is missing")
	})

	t.Run("wrong value", func(t *testing.T) {
		router := NewGinRouter(t)
		router.GET("/data", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"id": 123})
		})

		req := NewJSONRequest(t, "GET", "/data", nil)
		w := ServeRequest(router, req)

		mockT := &testing.T{}
		AssertJSONContains(mockT, w, map[string]any{"id": 456})
		assert.True(t, mockT.Failed(), "should fail when value mismatch")
	})
}

func TestParseJSONResponse(t *testing.T) {
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	router := NewGinRouter(t)
	router.GET("/user", func(c *gin.Context) {
		c.JSON(http.StatusOK, User{ID: 42, Name: "Alice"})
	})

	req := NewJSONRequest(t, "GET", "/user", nil)
	w := ServeRequest(router, req)

	user := ParseJSONResponse[User](t, w)
	assert.Equal(t, 42, user.ID)
	assert.Equal(t, "Alice", user.Name)
}

func TestNewMockHTTPServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	server := NewMockHTTPServer(t, handler)
	require.NotNil(t, server)
	require.NotEmpty(t, server.URL)

	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
}

func TestNewMockHTTPServer_MultipleRequests(t *testing.T) {
	requestCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = json.NewEncoder(w).Encode(map[string]int{"count": requestCount})
	})

	server := NewMockHTTPServer(t, handler)

	for i := 1; i <= 3; i++ {
		resp, err := http.Get(server.URL)
		require.NoError(t, err)

		var body map[string]int
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		require.NoError(t, err)

		assert.Equal(t, i, body["count"])
	}

	assert.Equal(t, 3, requestCount)
}

func TestNewMockHTTPServer_PostRequest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	})

	server := NewMockHTTPServer(t, handler)

	req, err := http.NewRequest("POST", server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRandomString(t *testing.T) {
	t.Run("length", func(t *testing.T) {
		s := RandomString(16)
		assert.Len(t, s, 32) // hex 编码后是 2 倍长度
	})

	t.Run("uniqueness", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			s := RandomString(8)
			assert.False(t, seen[s], "duplicate random string generated")
			seen[s] = true
		}
	})

	t.Run("format", func(t *testing.T) {
		s := RandomString(4)
		assert.Regexp(t, "^[0-9a-f]+$", s)
	})
}

func TestUniqueID(t *testing.T) {
	t.Run("format", func(t *testing.T) {
		id := UniqueID()
		assert.Regexp(t, "^test_[0-9a-f]{16}$", id)
	})

	t.Run("uniqueness", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := UniqueID()
			assert.False(t, seen[id], "duplicate unique ID generated")
			seen[id] = true
		}
	})
}
