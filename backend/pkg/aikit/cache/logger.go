package cache

import (
	"github.com/example/go-template/pkg/aikit/log"
	jetlogger "github.com/mgtv-tech/jetcache-go/logger"
)

type jetLogger struct{}

func (l *jetLogger) Debug(format string, v ...any) { log.Debug(format, v...) }
func (l *jetLogger) Info(format string, v ...any)  { log.Info(format, v...) }
func (l *jetLogger) Warn(format string, v ...any)  { log.Warn(format, v...) }
func (l *jetLogger) Error(format string, v ...any) { log.Error(format, v...) }

func init() {
	jetlogger.SetDefaultLogger(&jetLogger{})
}
