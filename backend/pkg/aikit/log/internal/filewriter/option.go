package filewriter

import (
	"fmt"
	"strings"
	"time"
)

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

type Option func(opt *option)

func RotateFormat(format string) Option {
	if strings.Contains(format, ".") {
		panic(fmt.Sprintf("rotate format can't contain '.' format: %s", format))
	}
	return func(opt *option) {
		opt.RotateFormat = format
	}
}

func MaxFile(n int) Option {
	return func(opt *option) {
		opt.MaxFile = n
	}
}

func MaxSize(n int64) Option {
	return func(opt *option) {
		opt.MaxSize = n
	}
}

func ChanSize(n int) Option {
	return func(opt *option) {
		opt.ChanSize = n
	}
}
