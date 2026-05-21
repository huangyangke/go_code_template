//go:build linux
// +build linux

package log

import (
	"context"
	"log"
	"log/syslog"
	"strings"
	"sync"
	"time"

	"github.com/example/go-template/pkg/aikit/log/internal/core"
)

const (
	_agentTimeout    = 20 * time.Millisecond
	_mergeWait       = 1 * time.Second
	_maxBuffer       = 10 * 1024 * 1024
	_defaultChan     = 2048
	_defaultPriority = 176 // syslog.LOG_LOCAL6
)

type AgentHandler struct {
	c      *AgentConfig
	msgs   chan []core.Field
	waiter sync.WaitGroup
	pool   sync.Pool
	enc    core.Encoder
}

type AgentConfig struct {
	Proto    string
	Addr     string
	Chan     int
	Timeout  time.Duration
	Priority int
}

func NewAgent(ac *AgentConfig) (a *AgentHandler) {
	a = &AgentHandler{
		c: ac,
		enc: core.NewJSONEncoder(core.EncoderConfig{
			EncodeTime:     core.TimeEncoderOfLayout(timeFormat),
			EncodeDuration: core.SecondsDurationEncoder,
		}, core.NewBuffer(0)),
	}
	a.pool.New = func() interface{} {
		return make([]core.Field, 0, 16)
	}
	if ac.Chan == 0 {
		ac.Chan = _defaultChan
	}
	a.msgs = make(chan []core.Field, ac.Chan)
	if ac.Timeout == 0 {
		ac.Timeout = _agentTimeout
	}
	if ac.Priority == 0 {
		ac.Priority = _defaultPriority
	}
	a.waiter.Add(1)
	go a.writeproc()
	return
}

func (h *AgentHandler) Log(ctx context.Context, lv Level, args ...D) {
	if args == nil {
		return
	}
	f := h.pool.Get().([]core.Field)
	for i := range args {
		f = append(f, args[i])
	}
	select {
	case h.msgs <- f:
	default:
	}
}

func (h *AgentHandler) writeproc() {
	var (
		conn *syslog.Writer
		err  error
		quit bool
	)
	buf := core.NewBuffer(2048)

	defer h.waiter.Done()
	tick := time.NewTicker(_mergeWait)
	defer tick.Stop()
	for {
		select {
		case d := <-h.msgs:
			if d == nil {
				quit = true
				goto DUMP
			}
			if buf.Len() >= _maxBuffer {
				buf.Reset()
			}
			h.enc.Encode(buf, d...)
			f := d[:0]
			h.pool.Put(f)
		case <-tick.C:
		}
		if conn == nil || err != nil {
			if conn, err = syslog.Dial(h.c.Proto, h.c.Addr, syslog.Priority(h.c.Priority), familyVal.Load().(string)); err != nil {
				log.Printf("syslog.Dial(%s:%s) error(%v)\n", h.c.Proto, h.c.Addr, err)
				continue
			}
		}
	DUMP:
		if conn != nil && buf.Len() > 0 {
			if _, err = conn.Write(buf.Bytes()); err != nil {
				if strings.HasSuffix(err.Error(), "message too long") {
					buf.Reset()
				}
				log.Printf("conn.Write(%d bytes) error(%v)\n", buf.Len(), err)
				conn.Close()
			} else {
				buf.Reset()
			}
		}
		if quit {
			if conn != nil && err == nil {
				conn.Close()
			}
			return
		}
	}
}

func (h *AgentHandler) Close() error {
	h.msgs <- nil
	h.waiter.Wait()
	return nil
}

func (h *AgentHandler) SetFormat(string) {}
