package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("task_id", "test-task-123")
	return c, w
}

func TestJSON_Basic(t *testing.T) {
	c, w := setupTestContext()

	JSON(c, map[string]string{"key": "value"}, "")

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeSuccess, resp.Code)
	assert.Equal(t, "success", resp.Msg)
	assert.Equal(t, "test-task-123", resp.TaskID)
}

func TestJSON_WithStatusCode(t *testing.T) {
	c, w := setupTestContext()

	JSON(c, nil, "", WithStatusCode())

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "200", w.Header().Get("X-Status-Code"))
}

func TestJSON_TaskIDFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// No task_id set in context → should fallback to "-"

	JSON(c, nil, "custom-id")

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "custom-id", resp.TaskID)
}

func TestFail_Basic(t *testing.T) {
	c, w := setupTestContext()

	Fail(c, http.StatusBadRequest, CodeBadRequest, "bad input")

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeBadRequest, resp.Code)
	assert.Equal(t, "bad input", resp.Msg)
}

func TestFail_WithStatusCode(t *testing.T) {
	c, w := setupTestContext()

	Fail(c, http.StatusBadRequest, CodeBadRequest, "bad input", WithStatusCode())

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "10000", w.Header().Get("X-Status-Code"))
}

func TestFail_WithConvertCode(t *testing.T) {
	c, w := setupTestContext()

	convertFn := func(code int) int {
		return http.StatusServiceUnavailable // override any status
	}

	Fail(c, http.StatusBadRequest, CodeBadRequest, "bad input", WithConvertCode(convertFn))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestJSONErr_Nil(t *testing.T) {
	c, w := setupTestContext()

	JSONErr(c, map[string]string{"result": "ok"}, nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeSuccess, resp.Code)
	assert.Equal(t, "ok", resp.Data.(map[string]interface{})["result"])
}

func TestJSONErr_GeneralError(t *testing.T) {
	c, w := setupTestContext()

	JSONErr(c, nil, errors.New("something went wrong"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeInternalError, resp.Code)
	assert.Equal(t, "something went wrong", resp.Msg)
}

func TestJSONErr_RecordNotFound(t *testing.T) {
	c, w := setupTestContext()

	JSONErr(c, nil, gorm.ErrRecordNotFound)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeNotFound, resp.Code)
	assert.Equal(t, "record not found", resp.Msg)
}

func TestJSONErr_WithStatusCode(t *testing.T) {
	c, w := setupTestContext()

	JSONErr(c, nil, errors.New("fail"), WithStatusCode())

	assert.Equal(t, "10005", w.Header().Get("X-Status-Code"))
}
