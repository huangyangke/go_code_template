package cache

import (
	jetlogger "github.com/mgtv-tech/jetcache-go/logger"

	"github.com/huangyangke/go-aikit/log"
)

type jetLogger struct{}

func (l *jetLogger) Debug(format string, v ...any) { log.Debug(format, v...) }
func (l *jetLogger) Info(format string, v ...any)  { log.Info(format, v...) }
func (l *jetLogger) Warn(format string, v ...any)  { log.Warn(format, v...) }
func (l *jetLogger) Error(format string, v ...any) { log.Error(format, v...) }

func init() {
	jetlogger.SetDefaultLogger(&jetLogger{})
}
