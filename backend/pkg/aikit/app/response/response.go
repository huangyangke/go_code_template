package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	CodeSuccess       = 200
	CodeBadRequest    = 10000
	CodeParamError    = 10001
	CodeMethodDenied  = 10002
	CodeNotFound      = 10003
	CodeRateLimited   = 10004
	CodeInternalError = 10005
	CodeUserNotFound  = 10006
	CodeUnauthorized  = 10007
	CodeForbidden     = 10008
	CodeConflict      = 10009
)

// ApiResponse 统一响应结构
type ApiResponse struct {
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	Data   any    `json:"data"`
	TaskID string `json:"task_id"`
}

func JSON(c *gin.Context, data any, taskID string, msg ...string) {
	if taskID == "" {
		taskID = getTaskID(c)
	}

	m := "success"
	if len(msg) > 0 && msg[0] != "" {
		m = msg[0]
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:   CodeSuccess,
		Msg:    m,
		Data:   data,
		TaskID: taskID,
	})
}

// bizError is the interface that typed business errors must implement.
type bizError interface {
	error
	BizCode() int
	BizHTTPStatus() int
}

// JSONErr sends a response determined by err.
// If err is nil, returns 200 with success.
// Priority: bizError → gorm.ErrRecordNotFound → generic 500.
func JSONErr(c *gin.Context, data any, err error) {
	if err == nil {
		JSON(c, data, "")
		return
	}

	var biz bizError
	if errors.As(err, &biz) {
		c.JSON(biz.BizHTTPStatus(), ApiResponse{
			Code:   biz.BizCode(),
			Msg:    biz.Error(),
			Data:   []any{},
			TaskID: getTaskID(c),
		})
		return
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, ApiResponse{
			Code:   CodeNotFound,
			Msg:    "记录不存在",
			Data:   []any{},
			TaskID: getTaskID(c),
		})
		return
	}

	c.JSON(http.StatusInternalServerError, ApiResponse{
		Code:   CodeInternalError,
		Msg:    "服务器内部错误",
		Data:   []any{},
		TaskID: getTaskID(c),
	})
}

// getTaskID extracts task_id from gin context (set by RequestID middleware)
func getTaskID(c *gin.Context) string {
	if id, ok := c.Get("task_id"); ok {
		if s, ok := id.(string); ok && s != "" {
			return s
		}
	}
	return "-"
}

// frameworkError is the package-private AppError type for framework-layer errors.
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

func BadRequest(c *gin.Context)       { JSONErr(c, nil, errBadRequest) }
func ParamError(c *gin.Context)       { JSONErr(c, nil, errParamError) }
func MethodNotAllowed(c *gin.Context) { JSONErr(c, nil, errMethodDenied) }
func NotFound(c *gin.Context)         { JSONErr(c, nil, errNotFound) }
func UserNotFound(c *gin.Context)     { JSONErr(c, nil, errUserNotFound) }
func InternalError(c *gin.Context)    { JSONErr(c, nil, errInternal) }
func Unauthorized(c *gin.Context)     { JSONErr(c, nil, errUnauthorized) }
func Forbidden(c *gin.Context)        { JSONErr(c, nil, errForbidden) }
func RateLimited(c *gin.Context)      { JSONErr(c, nil, errRateLimited) }
func Conflict(c *gin.Context)         { JSONErr(c, nil, errConflict) }
