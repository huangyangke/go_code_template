package log

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

type (
	WithField func(ctx context.Context) map[string]interface{}

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

func SetLevel(level string) {
	v = ParseLevel(level, v)
}

func SetFamily(f string) {
	familyVal.Store(f)
}

func GetFamily() string {
	return familyVal.Load().(string)
}

func Debug(format string, args ...interface{}) {
	if _debugLevel >= v {
		h.Log(context.Background(), _debugLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

func Info(format string, args ...interface{}) {
	if _infoLevel >= v {
		h.Log(context.Background(), _infoLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

func Warn(format string, args ...interface{}) {
	if _warnLevel >= v {
		h.Log(context.Background(), _warnLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

func Error(format string, args ...interface{}) {
	if _errorLevel >= v {
		h.Log(context.Background(), _errorLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

func Fatal(format string, args ...interface{}) {
	if _fatalLevel >= v {
		h.Log(context.Background(), _fatalLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

func DebugCtx(ctx context.Context, format string, args ...interface{}) {
	if _debugLevel >= v {
		h.Log(ctx, _debugLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

func InfoCtx(ctx context.Context, format string, args ...interface{}) {
	if _infoLevel >= v {
		h.Log(ctx, _infoLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

func WarnCtx(ctx context.Context, format string, args ...interface{}) {
	if _warnLevel >= v {
		h.Log(ctx, _warnLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

func ErrorCtx(ctx context.Context, format string, args ...interface{}) {
	if _errorLevel >= v {
		h.Log(ctx, _errorLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

func FatalCtx(ctx context.Context, format string, args ...interface{}) {
	if _fatalLevel >= v {
		h.Log(ctx, _fatalLevel, KVString(_log, fmt.Sprintf(format, args...)))
	}
}

func Debugv(ctx context.Context, args ...D) {
	if _debugLevel >= v {
		h.Log(ctx, _debugLevel, args...)
	}
}

func Infov(ctx context.Context, args ...D) {
	if _infoLevel >= v {
		h.Log(ctx, _infoLevel, args...)
	}
}

func Warnv(ctx context.Context, args ...D) {
	if _warnLevel >= v {
		h.Log(ctx, _warnLevel, args...)
	}
}

func Errorv(ctx context.Context, args ...D) {
	if _errorLevel >= v {
		h.Log(ctx, _errorLevel, args...)
	}
}

func Fatalv(ctx context.Context, args ...D) {
	if _fatalLevel >= v {
		h.Log(ctx, _fatalLevel, args...)
	}
}

func Close() error {
	err := h.Close()
	h = newHandlers(nil, _defaultStdout)
	return err
}
