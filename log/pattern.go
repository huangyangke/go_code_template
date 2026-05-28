package log

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Render 日志格式渲染接口.
type Render interface {
	Render(io.Writer, map[string]interface{}) error
	RenderString(map[string]interface{}) string
}

var patternMap = map[string]func(map[string]interface{}) string{
	"T": longTime,
	"L": keyFactory(_level),
	"f": keyFactory(_source),
	"i": keyFactory(_instanceID),
	"e": keyFactory(_deployEnv),
	"z": keyFactory(_zone),
	"S": longSource,
	"s": shortSource,
	"M": message,
}

func newPatternRender(format string) Render {
	p := &pattern{
		bufPool: sync.Pool{New: func() interface{} { return &bytes.Buffer{} }},
	}
	b := make([]byte, 0, len(format))
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			b = append(b, format[i])
			continue
		}
		if i+1 >= len(format) {
			b = append(b, format[i])
			continue
		}
		f, ok := patternMap[string(format[i+1])]
		if !ok {
			b = append(b, format[i])
			continue
		}
		if len(b) != 0 {
			p.funcs = append(p.funcs, textFactory(string(b)))
			b = b[:0]
		}
		p.funcs = append(p.funcs, f)
		i++
	}
	if len(b) != 0 {
		p.funcs = append(p.funcs, textFactory(string(b)))
	}
	return p
}

type pattern struct {
	funcs   []func(map[string]interface{}) string
	bufPool sync.Pool
}

// Render 将日志数据按格式模板渲染并写入 io.Writer.
// 参数：w - 输出目标, d - 日志字段字典.
// 返回值：err - 写入失败时的错误.
func (p *pattern) Render(w io.Writer, d map[string]interface{}) error {
	buf := p.bufPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		p.bufPool.Put(buf)
	}()
	for _, f := range p.funcs {
		buf.WriteString(f(d))
	}
	_, err := buf.WriteTo(w)
	return err
}

// RenderString 将日志数据按格式模板渲染为字符串.
// 参数：d - 日志字段字典.
// 返回值：渲染后的字符串.
func (p *pattern) RenderString(d map[string]interface{}) string {
	buf := p.bufPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		p.bufPool.Put(buf)
	}()
	for _, f := range p.funcs {
		buf.WriteString(f(d))
	}
	return buf.String()
}

func textFactory(text string) func(map[string]interface{}) string {
	return func(map[string]interface{}) string { return text }
}

func keyFactory(key string) func(map[string]interface{}) string {
	return func(d map[string]interface{}) string {
		if v, ok := d[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
			return fmt.Sprint(v)
		}
		return ""
	}
}

func longSource(map[string]interface{}) string {
	if _, file, lineNo, ok := runtime.Caller(5); ok {
		return fmt.Sprintf("%s:%d", file, lineNo)
	}
	return "unknown:0"
}

func shortSource(map[string]interface{}) string {
	if _, file, lineNo, ok := runtime.Caller(5); ok {
		return fmt.Sprintf("%s:%d", path.Base(file), lineNo)
	}
	return "unknown:0"
}

func longTime(map[string]interface{}) string {
	return time.Now().Format(timeFormat)
}

func isInternalKey(k string) bool {
	switch k {
	case _level, _levelValue, _time, _source, _deployEnv, _zone:
		return true
	}
	return false
}

func message(d map[string]interface{}) string {
	var m string
	var s []string
	for k, v := range d {
		if k == _log {
			m = fmt.Sprint(v)
			continue
		}
		if isInternalKey(k) {
			continue
		}
		s = append(s, fmt.Sprintf("%s=%v", k, v))
	}
	s = append(s, m)
	return strings.Join(s, " ")
}
