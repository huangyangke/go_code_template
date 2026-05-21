package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func TestBadRequest(t *testing.T) {
	c, w := testContext()
	BadRequest(c, "invalid input")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeBadRequest, resp.Code)
	assert.Equal(t, "invalid input", resp.Msg)
}

func TestParamError(t *testing.T) {
	c, w := testContext()
	ParamError(c, "missing field")
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeParamError, resp.Code)
}

func TestMethodNotAllowed(t *testing.T) {
	c, w := testContext()
	MethodNotAllowed(c, "use POST")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeMethodDenied, resp.Code)
}

func TestNotFound(t *testing.T) {
	c, w := testContext()
	NotFound(c, "resource not found")
	assert.Equal(t, http.StatusNotFound, w.Code)
	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeNotFound, resp.Code)
}

func TestUserNotFound(t *testing.T) {
	c, w := testContext()
	UserNotFound(c, "user does not exist")
	assert.Equal(t, http.StatusNotFound, w.Code)
	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeUserNotFound, resp.Code)
}

func TestInternalError(t *testing.T) {
	c, w := testContext()
	InternalError(c, "something broke")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeInternalError, resp.Code)
}

func TestUnauthorized(t *testing.T) {
	c, w := testContext()
	Unauthorized(c, "login required")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeUnauthorized, resp.Code)
}

func TestForbidden(t *testing.T) {
	c, w := testContext()
	Forbidden(c, "no permission")
	assert.Equal(t, http.StatusForbidden, w.Code)
	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeForbidden, resp.Code)
}

func TestRateLimited(t *testing.T) {
	c, w := testContext()
	RateLimited(c)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeRateLimited, resp.Code)
}

func TestConflict(t *testing.T) {
	c, w := testContext()
	Conflict(c, "already exists")
	assert.Equal(t, http.StatusConflict, w.Code)
	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeConflict, resp.Code)
}
