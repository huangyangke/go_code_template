// Package log 结构化日志库，支持文件、stdout、syslog 多输出.
package log

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

// WithField 从上下文中提取附加字段的函数.
type (
	WithField func(ctx context.Context) map[string]interface{}

	// Config 日志初始化配置.
	Config struct {
		Family     string
		Host       string
		Stdout     bool
		Dir        string
		Agent      string // e.g. udp://host:port?chan=1024&timeout=100ms
		MaxLogFile int
		RotateSize int64
		TimeFormat string
		Level      string // debug | info | warn | error | fatal
		Filter     []string
		WithFields map[string]WithField
	}
)

var (
	v          Level
	familyVal  atomic.Value
	host       string
	timeFormat = "2006-01-02 15:04:05.999999"
	c          *Config
	cfgGuard   sync.RWMutex
	h          *Handlers
)

func init() {
	familyVal.Store("dev")
	if hostname, err := os.Hostname(); err == nil {
		host = hostname
	} else {
		host = "unknown"
	}
	c = &Config{WithFields: map[string]WithField{}}
	h = newHandlers([]string{}, NewStdout())
}

// Init 初始化日志系统.
// 参数：conf - 日志配置, 为 nil 时仅启用 stdout.
func Init(conf *Config) {
	var isNil bool
	if conf == nil {
		isNil = true
		conf = c
	}
	if len(conf.Family) > 0 {
		familyVal.Store(conf.Family)
	}
	if len(conf.Host) > 0 {
		host = conf.Host
	}
	if len(conf.TimeFormat) > 0 {
		timeFormat = conf.TimeFormat
	}
	if len(conf.Level) > 0 {
		v = ParseLevel(conf.Level)
	}
	c = conf
	var hs []Handler
	if isNil || conf.Stdout {
		hs = append(hs, NewStdout())
	}
	if conf.Dir != "" {
		hs = append(hs, NewFile(conf.Dir, conf.RotateSize, conf.MaxLogFile))
	}
	if conf.Agent != "" {
		hs = append(hs, NewAgent(parseDSN(conf.Agent)))
	}

	set := make(map[string]struct{})
	for _, k := range conf.Filter {
		set[k] = struct{}{}
	}
	h.filters = set
	h.locker.Lock()
	defer h.locker.Unlock()
	h.handlers = h.handlers[:0]
	h.handlers = append(h.handlers, hs...)
}

// SetLevel 动态设置日志级别.
// 参数：level - 级别字符串 (debug/info/warn/error/fatal).
func SetLevel(level string) {
	v = ParseLevel(level, v)
}

// SetFamily 设置应用标识名称.
// 参数：f - 应用名称.
func SetFamily(f string) {
	familyVal.Store(f)
}

// GetFamily 获取当前应用标识名称.
// 返回值：应用名称字符串.
func GetFamily() string {
	return familyVal.Load().(string)
}

// Debug 输出调试级别日志.
// 参数：format - 格式字符串, args - 格式参数.
func Debug(format string, args ...interface{}) {
	if _debugLevel >= v {
		h.Log(context.Background(), _debugLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

// Info 输出信息级别日志.
// 参数：format - 格式字符串, args - 格式参数.
func Info(format string, args ...interface{}) {
	if _infoLevel >= v {
		h.Log(context.Background(), _infoLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

// Warn 输出警告级别日志.
// 参数：format - 格式字符串, args - 格式参数.
func Warn(format string, args ...interface{}) {
	if _warnLevel >= v {
		h.Log(context.Background(), _warnLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

// Error 输出错误级别日志.
// 参数：format - 格式字符串, args - 格式参数.
func Error(format string, args ...interface{}) {
	if _errorLevel >= v {
		h.Log(context.Background(), _errorLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

// Fatal 输出致命级别日志.
// 参数：format - 格式字符串, args - 格式参数.
func Fatal(format string, args ...interface{}) {
	if _fatalLevel >= v {
		h.Log(context.Background(), _fatalLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

// DebugCtx 输出带上下文的调试级别日志.
// 参数：ctx - 上下文, format - 格式字符串, args - 格式参数.
func DebugCtx(ctx context.Context, format string, args ...interface{}) {
	if _debugLevel >= v {
		h.Log(ctx, _debugLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

// InfoCtx 输出带上下文的信息级别日志.
// 参数：ctx - 上下文, format - 格式字符串, args - 格式参数.
func InfoCtx(ctx context.Context, format string, args ...interface{}) {
	if _infoLevel >= v {
		h.Log(ctx, _infoLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

// WarnCtx 输出带上下文的警告级别日志.
// 参数：ctx - 上下文, format - 格式字符串, args - 格式参数.
func WarnCtx(ctx context.Context, format string, args ...interface{}) {
	if _warnLevel >= v {
		h.Log(ctx, _warnLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

// ErrorCtx 输出带上下文的错误级别日志.
// 参数：ctx - 上下文, format - 格式字符串, args - 格式参数.
func ErrorCtx(ctx context.Context, format string, args ...interface{}) {
	if _errorLevel >= v {
		h.Log(ctx, _errorLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

// FatalCtx 输出带上下文的致命级别日志.
// 参数：ctx - 上下文, format - 格式字符串, args - 格式参数.
func FatalCtx(ctx context.Context, format string, args ...interface{}) {
	if _fatalLevel >= v {
		h.Log(ctx, _fatalLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

// Debugv 输出结构化调试日志.
// 参数：ctx - 上下文, args - 结构化字段列表.
func Debugv(ctx context.Context, args ...D) {
	if _debugLevel >= v {
		h.Log(ctx, _debugLevel, args...)
	}
}

// Infov 输出结构化信息日志.
// 参数：ctx - 上下文, args - 结构化字段列表.
func Infov(ctx context.Context, args ...D) {
	if _infoLevel >= v {
		h.Log(ctx, _infoLevel, args...)
	}
}

// Warnv 输出结构化警告日志.
// 参数：ctx - 上下文, args - 结构化字段列表.
func Warnv(ctx context.Context, args ...D) {
	if _warnLevel >= v {
		h.Log(ctx, _warnLevel, args...)
	}
}

// Errorv 输出结构化错误日志.
// 参数：ctx - 上下文, args - 结构化字段列表.
func Errorv(ctx context.Context, args ...D) {
	if _errorLevel >= v {
		h.Log(ctx, _errorLevel, args...)
	}
}

// Fatalv 输出结构化致命日志.
// 参数：ctx - 上下文, args - 结构化字段列表.
func Fatalv(ctx context.Context, args ...D) {
	if _fatalLevel >= v {
		h.Log(ctx, _fatalLevel, args...)
	}
}

// Close 关闭所有日志输出处理器.
// 返回值：err - 关闭失败时的错误.
func Close() error {
	err := h.Close()
	h = newHandlers(nil, _defaultStdout)
	return err
}
