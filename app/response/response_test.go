package response

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangyangke/go-aikit/internal/testutil"
)

func TestBadRequest(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { BadRequest(c) })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusBadRequest)
	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeBadRequest, resp.Code)
	assert.Equal(t, "请求错误", resp.Msg)
}

func TestParamError(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { ParamError(c) })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusUnprocessableEntity)
	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeParamError, resp.Code)
}

func TestMethodNotAllowed(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { MethodNotAllowed(c) })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusMethodNotAllowed)
	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeMethodDenied, resp.Code)
}

func TestNotFound(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { NotFound(c) })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusNotFound)
	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeNotFound, resp.Code)
}

func TestUserNotFound(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { UserNotFound(c) })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusNotFound)
	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeUserNotFound, resp.Code)
}

func TestInternalError(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { InternalError(c) })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusInternalServerError)
	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeInternalError, resp.Code)
}

func TestUnauthorized(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { Unauthorized(c) })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusUnauthorized)
	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeUnauthorized, resp.Code)
}

func TestForbidden(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { Forbidden(c) })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusForbidden)
	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeForbidden, resp.Code)
}

func TestRateLimited(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { RateLimited(c) })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusTooManyRequests)
	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeRateLimited, resp.Code)
}

func TestConflict(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { Conflict(c) })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusConflict)
	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeConflict, resp.Code)
}

// newGET 构造一个简单的 GET 测试请求.
func newGET(t *testing.T, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, path, nil)
	require.NoError(t, err)
	return req
}
