package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangyangke/go-aikit/internal/testutil"
)

func TestJSON_Basic(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) {
		c.Set("task_id", "test-task-123")
		JSON(c, map[string]string{"key": "value"}, "")
	})
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusOK)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeSuccess, resp.Code)
	assert.Equal(t, "success", resp.Msg)
	assert.Equal(t, "test-task-123", resp.TaskID)
}

func TestJSON_CustomMsg(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) {
		c.Set("task_id", "test-task-123")
		JSON(c, nil, "", "取消信号已发送")
	})
	w := testutil.ServeRequest(r, newGET(t, "/"))

	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "取消信号已发送", resp.Msg)
}

func TestJSON_TaskIDFallback(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) { JSON(c, nil, "custom-id") })
	w := testutil.ServeRequest(r, newGET(t, "/"))

	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "custom-id", resp.TaskID)
}

func TestJSONErr_Nil(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) {
		c.Set("task_id", "test-task-123")
		JSONErr(c, map[string]string{"result": "ok"}, nil)
	})
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusOK)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeSuccess, resp.Code)
	assert.Equal(t, "ok", resp.Data.(map[string]interface{})["result"])
}

func TestJSONErr_BizError(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) {
		c.Set("task_id", "test-task-123")
		err := &frameworkError{http.StatusBadRequest, CodeBadRequest, "请求错误"}
		JSONErr(c, nil, err)
	})
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusBadRequest)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeBadRequest, resp.Code)
	assert.Equal(t, "请求错误", resp.Msg)
}

func TestJSONErr_GeneralError(t *testing.T) {
	r := testutil.NewGinRouter(t)
	r.GET("/", func(c *gin.Context) {
		c.Set("task_id", "test-task-123")
		JSONErr(c, nil, errors.New("something went wrong"))
	})
	w := testutil.ServeRequest(r, newGET(t, "/"))

	testutil.AssertStatus(t, w, http.StatusInternalServerError)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, CodeInternalError, resp.Code)
	assert.Equal(t, "服务器内部错误", resp.Msg)
}
