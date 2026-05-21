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

type Handlers struct {
	locker   sync.RWMutex
	filters  map[string]struct{}
	handlers []Handler
}

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

func (hs *Handlers) SetFormat(format string) {
	hs.locker.RLock()
	hds := hs.handlers
	hs.locker.RUnlock()
	for _, h := range hds {
		h.SetFormat(format)
	}
}

func (hs *Handlers) addHandler(hr Handler) {
	hs.locker.Lock()
	defer hs.locker.Unlock()
	hs.handlers = append(hs.handlers, hr)
}
