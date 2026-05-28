package xjob

import (
	"strings"

	"github.com/huangyangke/go-aikit/log"
)

type logger struct{}

func (l *logger) Info(format string, a ...interface{}) {
	if strings.HasPrefix(format, "执行器注册成功") {
		return
	}
	log.Info("[XxlJob] "+format, a...)
}

func (l *logger) Error(format string, a ...interface{}) {
	log.Error("[XxlJob] "+format, a...)
}
