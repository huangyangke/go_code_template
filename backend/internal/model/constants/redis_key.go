package constants

import (
	"fmt"
	"time"
)

type RedisKey struct {
	tmpl string
	TTL  time.Duration
}

func NewKey(tmpl string, ttl time.Duration) RedisKey {
	return RedisKey{tmpl: tmpl, TTL: ttl}
}

func (k RedisKey) Format(args ...any) string {
	return fmt.Sprintf(k.tmpl, args...)
}

var (
	KeyArticle = NewKey("go-template:article:%d", 5*time.Minute)
)
