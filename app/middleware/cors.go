// Package middleware Gin HTTP 中间件（CORS、限流、认证、日志等）.
package middleware

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSConfig 跨域资源共享配置.
type CORSConfig struct {
	AllowOrigins     []string // 允许的源
	AllowMethods     []string // 允许的 HTTP 方法
	AllowHeaders     []string // 允许的请求头
	ExposeHeaders    []string // 暴露给客户端的响应头
	AllowCredentials bool     // 是否允许携带凭证
	MaxAge           int      // 预检请求缓存秒数
}

// DefaultCORSConfig 返回默认 CORS 配置.
// 返回值：默认跨域配置.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "X-Request-ID", "Authorization", "Accept", "X-Task-Priority"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           86400, // 一天
	}
}

// CORS 返回跨域资源共享中间件.
// 参数：config - CORS 配置，可选，缺省使用默认值.
// 返回值：gin 中间件 HandlerFunc.
func CORS(config ...CORSConfig) gin.HandlerFunc {
	cfg := DefaultCORSConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	// 构建时一次性扫描，使热路径无分支判断
	isWildcard := false
	for _, o := range cfg.AllowOrigins {
		if o == "*" {
			isWildcard = true
			break
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// 每次响应都设置 Access-Control-Allow-Origin
		if isWildcard {
			if cfg.AllowCredentials {
				// 规范禁止凭证模式使用通配符，回显请求源
				if origin != "" {
					c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
					c.Writer.Header().Add("Vary", "Origin")
				}
			} else {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			}
		} else {
			for _, o := range cfg.AllowOrigins {
				if o == origin {
					c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
					c.Writer.Header().Add("Vary", "Origin")
					break
				}
			}
		}

		if cfg.AllowCredentials {
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if len(cfg.ExposeHeaders) > 0 {
			c.Writer.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ","))
		}

		// 预检请求专属头（Fetch 规范）
		if c.Request.Method == "OPTIONS" {
			if len(cfg.AllowHeaders) > 0 {
				c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ","))
			}
			if len(cfg.AllowMethods) > 0 {
				c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ","))
			}
			if cfg.MaxAge > 0 {
				c.Writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
			}
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
