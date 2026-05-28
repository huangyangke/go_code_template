package xjob

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogger_Info_SilentPrefix(t *testing.T) {
	l := &logger{}
	// "执行器注册成功" prefix should be silently dropped — must not panic
	assert.NotPanics(t, func() {
		l.Info("执行器注册成功: %s", "some-executor")
	})
}

func TestLogger_Info_NormalMessage(t *testing.T) {
	l := &logger{}
	// Normal messages should be logged without panic
	assert.NotPanics(t, func() {
		l.Info("some info message: %s %d", "value", 42)
	})
}

func TestLogger_Error_NoPanic(t *testing.T) {
	l := &logger{}
	assert.NotPanics(t, func() {
		l.Error("some error message: %v", "error detail")
	})
}
