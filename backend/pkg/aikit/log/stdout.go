package log

import (
	"context"
	"io"
	"os"
	"time"
)

var _defaultStdout = NewStdout()

type StdoutHandler struct {
	out    io.Writer
	render Render
}

func NewStdout() *StdoutHandler {
	return &StdoutHandler{
		out:    os.Stderr,
		render: newPatternRender("[%T] [%L] [%S] %M"),
	}
}

func (h *StdoutHandler) Log(ctx context.Context, lv Level, args ...D) {
	d := toMap(args...)
	d[_time] = time.Now().Format(timeFormat)
	h.render.Render(h.out, d)
	h.out.Write([]byte("\n"))
}

func (h *StdoutHandler) Close() error {
	return nil
}

func (h *StdoutHandler) SetFormat(format string) {
	h.render = newPatternRender(format)
}
