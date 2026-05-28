package log

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetFamily(t *testing.T) {
	old := GetFamily()
	defer SetFamily(old)

	SetFamily("my-service")
	assert.Equal(t, "my-service", GetFamily())
}

func TestStdoutHandler_Log(t *testing.T) {
	var buf bytes.Buffer
	h := NewStdout()
	h.out = &buf

	h.Log(context.Background(), _infoLevel, KVString(_log, "hello world"))
	assert.Contains(t, buf.String(), "hello world")
}

func TestStdoutHandler_SetFormat(t *testing.T) {
	h := NewStdout()
	assert.NotPanics(t, func() {
		h.SetFormat("[%L] %M")
	})
}

func TestStdoutHandler_Close(t *testing.T) {
	h := NewStdout()
	assert.NoError(t, h.Close())
}

func TestHandlers_Log(t *testing.T) {
	var buf bytes.Buffer
	sh := NewStdout()
	sh.out = &buf

	hs := newHandlers(nil, sh)
	hs.Log(context.Background(), _infoLevel, KVString(_log, "test message"))
	assert.Contains(t, buf.String(), "test message")
}

func TestHandlers_Filter(t *testing.T) {
	hs := newHandlers([]string{"password"})

	d := []D{KVString("password", "secret123")}
	hs.Log(context.Background(), _infoLevel, d...)

	for _, field := range d {
		if field.Key == "password" {
			assert.Equal(t, "***", field.Value)
		}
	}
}

func TestHandlers_Close(t *testing.T) {
	sh := NewStdout()
	hs := newHandlers(nil, sh)
	assert.NoError(t, hs.Close())
}

func TestHandlers_SetFormat(t *testing.T) {
	sh := NewStdout()
	hs := newHandlers(nil, sh)
	assert.NotPanics(t, func() {
		hs.SetFormat("[%L] %M")
	})
}

func TestContextLogging(t *testing.T) {
	var buf bytes.Buffer
	sh := NewStdout()
	sh.out = &buf
	Init(&Config{Stdout: true, Level: "debug"})
	h.locker.Lock()
	h.handlers = h.handlers[:0]
	h.handlers = append(h.handlers, sh)
	h.locker.Unlock()

	ctx := context.Background()
	DebugCtx(ctx, "debug msg %d", 1)
	InfoCtx(ctx, "info msg %d", 2)
	WarnCtx(ctx, "warn msg %d", 3)
	ErrorCtx(ctx, "error msg %d", 4)

	output := buf.String()
	assert.Contains(t, output, "debug msg 1")
	assert.Contains(t, output, "info msg 2")
	assert.Contains(t, output, "warn msg 3")
	assert.Contains(t, output, "error msg 4")
}

func TestClose(t *testing.T) {
	Init(&Config{Stdout: true, Level: "info"})
	assert.NoError(t, Close())
}

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{_debugLevel, "DEBUG"},
		{_infoLevel, "INFO"},
		{_warnLevel, "WARN"},
		{_errorLevel, "ERROR"},
		{_fatalLevel, "FATAL"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.level.String())
	}
}

func TestSetLevel(t *testing.T) {
	var buf bytes.Buffer
	sh := NewStdout()
	sh.out = &buf
	Init(&Config{Stdout: true, Level: "debug"})
	h.locker.Lock()
	h.handlers = h.handlers[:0]
	h.handlers = append(h.handlers, sh)
	h.locker.Unlock()

	SetLevel("error")
	Debug("should not appear")
	Info("should not appear either")
	Error("visible error")

	output := buf.String()
	assert.NotContains(t, output, "should not appear")
	assert.Contains(t, output, "visible error")
}

func TestLogWithFields(t *testing.T) {
	var buf bytes.Buffer
	sh := NewStdout()
	sh.out = &buf

	Init(&Config{
		Stdout: true,
		Level:  "debug",
		WithFields: map[string]WithField{
			"trace": func(ctx context.Context) map[string]interface{} {
				return map[string]interface{}{"trace_id": "abc123"}
			},
		},
	})
	h.locker.Lock()
	h.handlers = h.handlers[:0]
	h.handlers = append(h.handlers, sh)
	h.locker.Unlock()

	Info("with fields test")
	output := buf.String()
	assert.True(t, strings.Contains(output, "with fields test"))
}
