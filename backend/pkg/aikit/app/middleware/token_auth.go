package middleware

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/example/go-template/pkg/aikit/app/response"
	"github.com/example/go-template/pkg/aikit/log"
)

type VerifyFunc func(ctx context.Context, token string) (bool, error)

// TokenAuth validates Bearer tokens from the Authorization header.
// Paths matching any whitelist entry (exact match or sub-path) are skipped.
func TokenAuth(verify VerifyFunc, whitelist ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if isWhitelisted(path, whitelist) {
			c.Next()
			return
		}

		auth := c.GetHeader("Authorization")
		// RFC 6750: scheme is case-insensitive
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

// OrVerify returns a VerifyFunc that passes when either a or b passes.
// Both are always called on error from a (fail-close), but b is skipped if a succeeds.
// The error from a is surfaced only if b also fails.
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
		// Both failed — surface a's error if present, otherwise b's.
		if errA != nil {
			return false, errA
		}
		return false, errB
	}
}

// WithInternalToken wraps a VerifyFunc so that a static internal token is
// accepted without calling the wrapped function. Uses constant-time comparison
// to prevent timing attacks.
// Pass nil for inner to accept only the internal token.
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

// isWhitelisted matches a path against whitelist entries.
// A path matches if it exactly equals a whitelist entry or is a sub-path
// of it (i.e. HasPrefix with "/" separator). This prevents bypass attacks
// where "/v1/articles-secret" would match a "/v1/articles" prefix.
func isWhitelisted(path string, whitelist []string) bool {
	for _, prefix := range whitelist {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
