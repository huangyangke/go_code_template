package errors_test

import (
	"testing"

	apperrors "github.com/huangyangke/go_code_template/backend/internal/errors"
)

// Compile-time interface assertion.
var _ interface {
	Error() string
	BizCode() int
	BizHTTPStatus() int
} = (*apperrors.AppError)(nil)

func TestAppError_Methods(t *testing.T) {
	err := apperrors.ErrArticleNotFound
	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if err.BizCode() == 0 {
		t.Error("BizCode() should not be zero")
	}
	if err.BizHTTPStatus() == 0 {
		t.Error("BizHTTPStatus() should not be zero")
	}
}
