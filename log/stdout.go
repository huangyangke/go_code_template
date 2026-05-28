package log

import (
	"context"
	"io"
	"os"
	"time"
)

var _defaultStdout = NewStdout()

// StdoutHandler 标准输出日志处理器.
type StdoutHandler struct {
	out    io.Writer
	render Render
}

// NewStdout 创建标准输出日志处理器.
// 返回值：stdout 日志处理器.
func NewStdout() *StdoutHandler {
	return &StdoutHandler{
		out:    os.Stderr,
		render: newPatternRender("[%T] [%L] [%S] %M"),
	}
}

// Log 将日志输出到标准错误流.
// 参数：ctx - 上下文, lv - 日志级别, args - 结构化字段列表.
func (h *StdoutHandler) Log(ctx context.Context, lv Level, args ...D) {
	d := toMap(args...)
	d[_time] = time.Now().Format(timeFormat)
	h.render.Render(h.out, d) //nolint:errcheck
	h.out.Write([]byte("\n")) //nolint:errcheck
}

// Close 关闭 stdout 处理器（无操作）.
// 返回值：nil - 无错误.
func (h *StdoutHandler) Close() error {
	return nil
}

// SetFormat 设置日志输出格式模板.
// 参数：format - 格式模板字符串.
func (h *StdoutHandler) SetFormat(format string) {
	h.render = newPatternRender(format)
}
