package cache

import (
	"errors"
	"strings"

	"github.com/mgtv-tech/jetcache-go/local"
)

var unitMap = map[string]uint64{
	"b":    uint64(local.Byte),
	"byte": uint64(local.Byte),
	"kb":   uint64(local.KB),
	"mb":   uint64(local.MB),
	"gb":   uint64(local.GB),
	"tb":   uint64(local.TB),
}

// ParseSize 将大小字符串（如 "256MB"）解析为 local.Size.
// 参数：s - 大小字符串.
// 返回值：size - 解析后的字节大小, err - 解析失败时的错误.
func ParseSize(s string) (local.Size, error) {
	orig := s
	var d uint64
	s = strings.ToLower(s)

	if s == "0" {
		return 0, nil
	}
	if s == "" {
		return 0, errors.New("parseSize: invalid size " + orig)
	}
	for s != "" {
		var (
			v, f  uint64
			scale float64 = 1
		)

		if s[0] != '.' && (s[0] < '0' || s[0] > '9') {
			return 0, errors.New("parseSize: invalid size " + orig)
		}

		pl := len(s)
		v, s, _ = leadingInt(s)
		pre := pl != len(s)

		post := false
		if s != "" && s[0] == '.' {
			s = s[1:]
			pl := len(s)
			f, scale, s = leadingFraction(s)
			post = pl != len(s)
		}
		if !pre && !post {
			return 0, errors.New("parseSize: invalid size " + orig)
		}

		i := 0
		for ; i < len(s); i++ {
			c := s[i]
			if c == '.' || ('0' <= c && c <= '9') {
				break
			}
		}
		if i == 0 {
			return 0, errors.New("parseSize: missing unit in size " + orig)
		}
		u := s[:i]
		s = s[i:]
		unit, ok := unitMap[u]
		if !ok {
			return 0, errors.New("parseSize: unknown unit " + u + " in size " + orig)
		}
		if v > 1<<63/unit {
			return 0, errors.New("parseSize: invalid size " + orig)
		}
		v *= unit
		if f > 0 {
			v += uint64(float64(f) * (float64(unit) / scale))
			if v > 1<<63 {
				return 0, errors.New("parseSize: invalid size " + orig)
			}
		}
		d += v
		if d > 1<<63 {
			return 0, errors.New("parseSize: invalid size " + orig)
		}
	}
	return local.Size(d), nil
}

func leadingInt(s string) (x uint64, rem string, err error) {
	i := 0
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		if x > 1<<63/10 {
			return 0, "", errLeadingInt
		}
		x = x*10 + uint64(c) - '0'
		if x > 1<<63 {
			return 0, "", errLeadingInt
		}
	}
	return x, s[i:], nil
}

func leadingFraction(s string) (x uint64, scale float64, rem string) {
	i := 0
	scale = 1
	overflow := false
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		if overflow {
			continue
		}
		if x > (1<<63-1)/10 {
			overflow = true
			continue
		}
		y := x*10 + uint64(c) - '0'
		if y > 1<<63 {
			overflow = true
			continue
		}
		x = y
		scale *= 10
	}
	return x, scale, s[i:]
}

var errLeadingInt = errors.New("parseSize: bad [0-9]*")
