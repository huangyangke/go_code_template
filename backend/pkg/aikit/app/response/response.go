// Package response 统一 API 响应封装.
package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/huangyangke/go-aikit/log"
)

const (
	// CodeSuccess 请求成功.
	CodeSuccess = 200
	// CodeBadRequest 请求错误.
	CodeBadRequest = 10000
	// CodeParamError 参数校验错误.
	CodeParamError = 10001
	// CodeMethodDenied 请求方法错误.
	CodeMethodDenied = 10002
	// CodeNotFound 请求路径错误.
	CodeNotFound = 10003
	// CodeRateLimited 请求过于频繁.
	CodeRateLimited = 10004
	// CodeInternalError 服务器内部错误.
	CodeInternalError = 10005
	// CodeUserNotFound 用户不存在.
	CodeUserNotFound = 10006
	// CodeUnauthorized 未登录或登录已失效.
	CodeUnauthorized = 10007
	// CodeForbidden 无权限访问.
	CodeForbidden = 10008
	// CodeConflict 资源已存在.
	CodeConflict = 10009

	// TaskIDKey RequestID 中间件在 gin 上下文中存储 task_id 的键名.
	TaskIDKey = "task_id"
)

// APIResponse 统一响应信封结构.
type APIResponse struct {
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	Data   any    `json:"data"`
	TaskID string `json:"task_id"`
}

// JSON 返回成功的统一响应.
// 参数：c - gin 上下文, data - 响应数据, taskID - 任务ID（为空时自动获取）, msg - 自定义消息（可选）.
// 返回值：无.
func JSON(c *gin.Context, data any, taskID string, msg ...string) {
	if taskID == "" {
		taskID = getTaskID(c)
	}

	m := "success"
	if len(msg) > 0 && msg[0] != "" {
		m = msg[0]
	}

	c.JSON(http.StatusOK, APIResponse{
		Code:   CodeSuccess,
		Msg:    m,
		Data:   normalizeData(data),
		TaskID: taskID,
	})
}

// BizError 业务类型错误需实现的接口.
type BizError interface {
	error
	BizCode() int
	BizHTTPStatus() int
}

// normalizeData 确保 Data 字段在 JSON 输出中不为 null.
func normalizeData(data any) any {
	if data == nil {
		return struct{}{}
	}
	return data
}

// JSONErr 根据 err 类型返回错误响应.
// 参数：c - gin 上下文, data - 响应数据, err - 错误（nil 时返回成功响应）.
// 返回值：无.
// 优先级：BizError → 通用 500.
func JSONErr(c *gin.Context, data any, err error) {
	if err == nil {
		JSON(c, data, "")
		return
	}

	var biz BizError
	if errors.As(err, &biz) {
		c.JSON(biz.BizHTTPStatus(), APIResponse{
			Code:   biz.BizCode(),
			Msg:    biz.Error(),
			Data:   normalizeData(data),
			TaskID: getTaskID(c),
		})
		return
	}

	log.Error("[response][internal_error][task_id=%s]: %v", getTaskID(c), err)
	c.JSON(http.StatusInternalServerError, APIResponse{
		Code:   CodeInternalError,
		Msg:    "服务器内部错误",
		Data:   normalizeData(data),
		TaskID: getTaskID(c),
	})
}

// getTaskID 从 gin 上下文中提取 task_id（由 RequestID 中间件设置）.
func getTaskID(c *gin.Context) string {
	if id, ok := c.Get(TaskIDKey); ok {
		if s, ok := id.(string); ok && s != "" {
			return s
		}
	}
	return "-"
}

// frameworkError 框架层错误的内部类型.
type frameworkError struct {
	httpStatus int
	code       int
	msg        string
}

func (e *frameworkError) Error() string      { return e.msg }
func (e *frameworkError) BizCode() int       { return e.code }
func (e *frameworkError) BizHTTPStatus() int { return e.httpStatus }

var (
	errBadRequest   = &frameworkError{http.StatusBadRequest, CodeBadRequest, "请求错误"}
	errParamError   = &frameworkError{http.StatusUnprocessableEntity, CodeParamError, "参数校验错误"}
	errMethodDenied = &frameworkError{http.StatusMethodNotAllowed, CodeMethodDenied, "请求方法错误"}
	errNotFound     = &frameworkError{http.StatusNotFound, CodeNotFound, "请求路径错误"}
	errUserNotFound = &frameworkError{http.StatusNotFound, CodeUserNotFound, "用户不存在"}
	errInternal     = &frameworkError{http.StatusInternalServerError, CodeInternalError, "服务器内部错误"}
	errUnauthorized = &frameworkError{http.StatusUnauthorized, CodeUnauthorized, "未登录或登录已失效"}
	errForbidden    = &frameworkError{http.StatusForbidden, CodeForbidden, "无权限访问"}
	errRateLimited  = &frameworkError{http.StatusTooManyRequests, CodeRateLimited, "请求过于频繁，请稍后重试"}
	errConflict     = &frameworkError{http.StatusConflict, CodeConflict, "资源已存在"}
)

// BadRequest 返回请求错误响应.
// 参数：c - gin 上下文.
// 返回值：无.
func BadRequest(c *gin.Context) { JSONErr(c, nil, errBadRequest) }

// ParamError 返回参数校验错误响应.
// 参数：c - gin 上下文.
// 返回值：无.
func ParamError(c *gin.Context) { JSONErr(c, nil, errParamError) }

// MethodNotAllowed 返回请求方法错误响应.
// 参数：c - gin 上下文.
// 返回值：无.
func MethodNotAllowed(c *gin.Context) { JSONErr(c, nil, errMethodDenied) }

// NotFound 返回请求路径错误响应.
// 参数：c - gin 上下文.
// 返回值：无.
func NotFound(c *gin.Context) { JSONErr(c, nil, errNotFound) }

// UserNotFound 返回用户不存在响应.
// 参数：c - gin 上下文.
// 返回值：无.
func UserNotFound(c *gin.Context) { JSONErr(c, nil, errUserNotFound) }

// InternalError 返回服务器内部错误响应.
// 参数：c - gin 上下文.
// 返回值：无.
func InternalError(c *gin.Context) { JSONErr(c, nil, errInternal) }

// Unauthorized 返回未登录或登录已失效响应.
// 参数：c - gin 上下文.
// 返回值：无.
func Unauthorized(c *gin.Context) { JSONErr(c, nil, errUnauthorized) }

// Forbidden 返回无权限访问响应.
// 参数：c - gin 上下文.
// 返回值：无.
func Forbidden(c *gin.Context) { JSONErr(c, nil, errForbidden) }

// RateLimited 返回请求过于频繁响应，并设置 Retry-After 头.
// 参数：c - gin 上下文.
// 返回值：无.
func RateLimited(c *gin.Context) {
	c.Header("Retry-After", "1")
	JSONErr(c, nil, errRateLimited)
}

// Conflict 返回资源已存在响应.
// 参数：c - gin 上下文.
// 返回值：无.
func Conflict(c *gin.Context) { JSONErr(c, nil, errConflict) }
