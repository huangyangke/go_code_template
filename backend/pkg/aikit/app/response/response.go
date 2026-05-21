package response

import (
	"errors"
	"net/http"
	"strconv"

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

// ResponseOption configures response behavior
type ResponseOption func(*responseConfig)

type responseConfig struct {
	statusCodeHeader bool
	convertCode      func(code int) int
}

// WithStatusCode writes the business error code to the X-Status-Code response header
func WithStatusCode() ResponseOption {
	return func(c *responseConfig) {
		c.statusCodeHeader = true
	}
}

// WithConvertCode converts a business error code to an HTTP status code.
// When set on Fail(), the returned HTTP status overrides the httpStatus parameter.
func WithConvertCode(fn func(code int) int) ResponseOption {
	return func(c *responseConfig) {
		c.convertCode = fn
	}
}

func applyOpts(opts ...ResponseOption) *responseConfig {
	cfg := &responseConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func JSON(c *gin.Context, data any, taskID string, opts ...ResponseOption) {
	cfg := applyOpts(opts...)

	if taskID == "" {
		taskID = getTaskID(c)
	}

	if cfg.statusCodeHeader {
		c.Header("X-Status-Code", strconv.Itoa(CodeSuccess))
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:   CodeSuccess,
		Msg:    "success",
		Data:   data,
		TaskID: taskID,
	})
}

func Fail(c *gin.Context, httpStatus int, code int, msg string, opts ...ResponseOption) {
	cfg := applyOpts(opts...)

	if cfg.convertCode != nil {
		httpStatus = cfg.convertCode(code)
	}
	if cfg.statusCodeHeader {
		c.Header("X-Status-Code", strconv.Itoa(code))
	}

	c.JSON(httpStatus, ApiResponse{
		Code:   code,
		Msg:    msg,
		Data:   []any{},
		TaskID: getTaskID(c),
	})
}

// JSONErr sends a response that automatically determines the business code from err.
// If err is nil, returns 200 with success. If err is not nil, maps it to a business code.
func JSONErr(c *gin.Context, data any, err error, opts ...ResponseOption) {
	if err == nil {
		JSON(c, data, "", opts...)
		return
	}

	code := CodeInternalError
	msg := err.Error()
	httpStatus := http.StatusInternalServerError

	if errors.Is(err, gorm.ErrRecordNotFound) {
		code = CodeNotFound
		msg = "record not found"
		httpStatus = http.StatusNotFound
	}

	Fail(c, httpStatus, code, msg, opts...)
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

func BadRequest(c *gin.Context, msg string) {
	Fail(c, http.StatusBadRequest, CodeBadRequest, msg)
}

func ParamError(c *gin.Context, msg string) {
	Fail(c, http.StatusUnprocessableEntity, CodeParamError, msg)
}

func MethodNotAllowed(c *gin.Context, msg string) {
	Fail(c, http.StatusMethodNotAllowed, CodeMethodDenied, msg)
}

func NotFound(c *gin.Context, msg string) {
	Fail(c, http.StatusNotFound, CodeNotFound, msg)
}

func UserNotFound(c *gin.Context, msg string) {
	Fail(c, http.StatusNotFound, CodeUserNotFound, msg)
}

func InternalError(c *gin.Context, msg string) {
	Fail(c, http.StatusInternalServerError, CodeInternalError, msg)
}

func Unauthorized(c *gin.Context, msg string) {
	Fail(c, http.StatusUnauthorized, CodeUnauthorized, msg)
}

func Forbidden(c *gin.Context, msg string) {
	Fail(c, http.StatusForbidden, CodeForbidden, msg)
}

func RateLimited(c *gin.Context) {
	Fail(c, http.StatusTooManyRequests, CodeRateLimited, "请求过于频繁，请稍后重试")
}

func Conflict(c *gin.Context, msg string) {
	Fail(c, http.StatusConflict, CodeConflict, msg)
}
