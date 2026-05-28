package filewriter

import (
	"fmt"
	"strings"
	"time"
)

// RotateDaily 每日轮转的时间格式.
const RotateDaily = "2006-01-02"

var defaultOption = option{
	RotateFormat:   RotateDaily,
	MaxFile:        30,
	MaxSize:        1 << 30,
	ChanSize:       1024 * 8,
	RotateInterval: 10 * time.Second,
}

type option struct {
	RotateFormat   string
	MaxFile        int
	MaxSize        int64
	ChanSize       int
	RotateInterval time.Duration
	WriteTimeout   time.Duration
}

// Option FileWriter 配置选项函数.
type Option func(opt *option)

// RotateFormat 设置日志轮转时间格式.
// 参数：format - Go 时间格式字符串.
// 返回值：Option 函数.
func RotateFormat(format string) Option {
	if strings.Contains(format, ".") {
		panic(fmt.Sprintf("rotate format can't contain '.' format: %s", format))
	}
	return func(opt *option) {
		opt.RotateFormat = format
	}
}

// MaxFile 设置最大保留轮转文件数.
// 参数：n - 最大文件数.
// 返回值：Option 函数.
func MaxFile(n int) Option {
	return func(opt *option) {
		opt.MaxFile = n
	}
}

// MaxSize 设置单文件最大字节数.
// 参数：n - 最大字节数.
// 返回值：Option 函数.
func MaxSize(n int64) Option {
	return func(opt *option) {
		opt.MaxSize = n
	}
}

// ChanSize 设置写入通道大小.
// 参数：n - 通道容量.
// 返回值：Option 函数.
func ChanSize(n int) Option {
	return func(opt *option) {
		opt.ChanSize = n
	}
}
