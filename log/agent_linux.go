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

	"github.com/huangyangke/go-aikit/log/internal/core"
)

const (
	_agentTimeout    = 20 * time.Millisecond
	_mergeWait       = 1 * time.Second
	_maxBuffer       = 10 * 1024 * 1024
	_defaultChan     = 2048
	_defaultPriority = 176 // syslog.LOG_LOCAL6
)

// AgentHandler syslog 远程日志处理器，异步批量发送.
type AgentHandler struct {
	c      *AgentConfig
	msgs   chan []core.Field
	waiter sync.WaitGroup
	pool   sync.Pool
	enc    core.Encoder
}

// AgentConfig syslog 连接配置.
type AgentConfig struct {
	Proto    string
	Addr     string
	Chan     int
	Timeout  time.Duration
	Priority int
}

// NewAgent 创建 syslog 远程日志处理器.
// 参数：ac - syslog 连接配置.
// 返回值：a - Agent 处理器.
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

// Log 将日志入队等待异步发送.
// 参数：ctx - 上下文, lv - 日志级别, args - 结构化字段列表.
func (h *AgentHandler) Log(ctx context.Context, lv Level, args ...D) {
	if args == nil {
		return
	}
	f := h.pool.Get().([]core.Field)
	f = append(f, args...)
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
			_ = h.enc.Encode(buf, d...)
			f := d[:0]
			h.pool.Put(f) //nolint:staticcheck
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
				_ = conn.Close()
			} else {
				buf.Reset()
			}
		}
		if quit {
			if conn != nil && err == nil {
				_ = conn.Close()
			}
			return
		}
	}
}

// Close 关闭 Agent 处理器，等待缓冲日志全部发送.
// 返回值：err - 关闭失败时的错误.
func (h *AgentHandler) Close() error {
	h.msgs <- nil
	h.waiter.Wait()
	return nil
}

// SetFormat 设置输出格式（Agent 不支持，空实现）.
// 参数：format - 格式模板字符串.
func (h *AgentHandler) SetFormat(string) {}
