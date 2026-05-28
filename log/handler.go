package log

import (
	"context"
	"sync"
	"time"
)

const (
	_levelValue = "level_value"
	_level      = "level"
	_time       = "time"
	_source     = "source"
	_log        = "log"
	_appID      = "app_id"
	_instanceID = "instance_id"
	_deployEnv  = "env"
	_zone       = "zone"
)

// Handler 日志输出处理器接口.
type Handler interface {
	Log(context.Context, Level, ...D)
	SetFormat(string)
	Close() error
}

func newHandlers(filters []string, handlers ...Handler) *Handlers {
	set := make(map[string]struct{})
	for _, k := range filters {
		set[k] = struct{}{}
	}
	return &Handlers{filters: set, handlers: handlers}
}

// Handlers 日志处理器集合，支持过滤和并发安全.
type Handlers struct {
	locker   sync.RWMutex
	filters  map[string]struct{}
	handlers []Handler
}

// Log 将日志分发到所有处理器.
// 参数：ctx - 上下文, lv - 日志级别, d - 结构化字段列表.
func (hs *Handlers) Log(ctx context.Context, lv Level, d ...D) {
	var hasSource bool
	for i := range d {
		if _, ok := hs.filters[d[i].Key]; ok {
			d[i].Value = "***"
		}
		if d[i].Key == _source {
			hasSource = true
		}
	}
	if !hasSource {
		fn := funcName(3)
		d = append(d, KVString(_source, fn))
	}
	d = append(d, KVString(_appID, familyVal.Load().(string)))
	d = append(d, KVString(_instanceID, host))
	d = append(d, KV(_time, time.Now()), KVInt64(_levelValue, int64(lv)), KVString(_level, lv.String()))

	cfgGuard.RLock()
	wfs := c.WithFields
	cfgGuard.RUnlock()
	if len(wfs) > 0 {
		for _, wf := range wfs {
			for k, v := range wf(ctx) {
				d = append(d, KV(k, v))
			}
		}
	}
	hs.locker.RLock()
	hds := hs.handlers
	hs.locker.RUnlock()
	for _, h := range hds {
		h.Log(ctx, lv, d...)
	}
}

// Close 关闭所有处理器.
// 返回值：err - 关闭失败时的错误.
func (hs *Handlers) Close() (err error) {
	hs.locker.RLock()
	hds := hs.handlers
	hs.locker.RUnlock()
	for _, h := range hds {
		if e := h.Close(); e != nil {
			err = e
		}
	}
	return
}

// SetFormat 设置所有处理器的输出格式.
// 参数：format - 格式模板字符串.
func (hs *Handlers) SetFormat(format string) {
	hs.locker.RLock()
	hds := hs.handlers
	hs.locker.RUnlock()
	for _, h := range hds {
		h.SetFormat(format)
	}
}
