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

func TestJSON_CustomMsg(t *testing.T) {
	c, w := setupTestContext()

	JSON(c, nil, "", "取消信号已发送")

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "取消信号已发送", resp.Msg)
}

func TestJSON_TaskIDFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	JSON(c, nil, "custom-id")

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "custom-id", resp.TaskID)
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

func TestJSONErr_BizError(t *testing.T) {
	c, w := setupTestContext()

	err := &frameworkError{http.StatusBadRequest, CodeBadRequest, "请求错误"}
	JSONErr(c, nil, err)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeBadRequest, resp.Code)
	assert.Equal(t, "请求错误", resp.Msg)
}

func TestJSONErr_GeneralError(t *testing.T) {
	c, w := setupTestContext()

	JSONErr(c, nil, errors.New("something went wrong"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeInternalError, resp.Code)
	assert.Equal(t, "服务器内部错误", resp.Msg)
}

