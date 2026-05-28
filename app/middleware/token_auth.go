package middleware

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/huangyangke/go-aikit/app/response"
	"github.com/huangyangke/go-aikit/log"
)

// VerifyFunc Bearer 令牌验证函数.
type VerifyFunc func(ctx context.Context, token string) (bool, error)

// TokenAuth 返回 Bearer 令牌认证中间件.
// 白名单路径（精确匹配或子路径）跳过验证.
// 参数：verify - 令牌验证函数, whitelist - 跳过认证的路径列表.
// 返回值：gin 中间件 HandlerFunc.
func TokenAuth(verify VerifyFunc, whitelist ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if isWhitelisted(path, whitelist) {
			c.Next()
			return
		}

		auth := c.GetHeader("Authorization")
		// RFC 6750：scheme 不区分大小写
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") || parts[1] == "" {
			response.Unauthorized(c)
			c.Abort()
			return
		}
		token := parts[1]

		ok, err := verify(c.Request.Context(), token)
		if err != nil {
			log.Warn("[TokenAuth][verify_error]: %v", err)
			response.Unauthorized(c)
			c.Abort()
			return
		}
		if !ok {
			response.Unauthorized(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

// OrVerify 返回组合验证函数，a 或 b 任一通过即通过.
// a 失败时始终调用 b（fail-close），a 成功时跳过 b.
// 仅当两者都失败时才返回 a 的错误.
// 参数：a - 主验证函数, b - 备验证函数.
// 返回值：组合验证函数.
func OrVerify(a, b VerifyFunc) VerifyFunc {
	return func(ctx context.Context, token string) (bool, error) {
		ok, errA := a(ctx, token)
		if ok {
			return true, nil
		}
		ok, errB := b(ctx, token)
		if ok {
			return true, nil
		}
		// 两者均失败，优先返回 a 的错误
		if errA != nil {
			return false, errA
		}
		return false, errB
	}
}

// WithInternalToken 返回增强验证函数，内部令牌使用常量时间比较直接通过.
// 传入 nil 表示仅接受内部令牌.
// 参数：internalToken - 内部静态令牌, inner - 原验证函数.
// 返回值：增强后的验证函数.
func WithInternalToken(internalToken string, inner VerifyFunc) VerifyFunc {
	tokenBytes := []byte(internalToken)
	return func(ctx context.Context, token string) (bool, error) {
		if subtle.ConstantTimeCompare([]byte(token), tokenBytes) == 1 {
			return true, nil
		}
		if inner == nil {
			return false, nil
		}
		return inner(ctx, token)
	}
}

// isWhitelisted 匹配路径是否在白名单中.
// 精确匹配或以 "/" 分隔的子路径匹配，防止 "/v1/articles-secret" 误匹配 "/v1/articles".
func isWhitelisted(path string, whitelist []string) bool {
	for _, prefix := range whitelist {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
