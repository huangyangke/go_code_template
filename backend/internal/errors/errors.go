// Package errors defines typed business errors for the application.
// Each AppError binds an HTTP status, a business code, and a message at compile time.
//
// Business code ranges (step by 100):
//
//	10100–10199  Article
//	10200–10299  User
//	10300–10399  Order
package errors

import "net/http"

// AppError is a typed error that carries HTTP status, business code, and message.
type AppError struct {
	httpStatus int
	code       int
	msg        string
}

func (e *AppError) Error() string      { return e.msg }
func (e *AppError) BizCode() int       { return e.code }
func (e *AppError) BizHTTPStatus() int { return e.httpStatus }

// Article errors (10100–10199)
var (
	ErrArticleNotFound = &AppError{http.StatusNotFound, 10100, "文章不存在"}
	ErrArticleDeleted  = &AppError{http.StatusGone, 10101, "文章已删除"}
)
