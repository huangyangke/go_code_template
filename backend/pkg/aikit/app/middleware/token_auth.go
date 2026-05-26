package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/example/go-template/pkg/aikit/app/response"
)

type VerifyFunc func(ctx context.Context, token string) (bool, error)

// TokenAuth validates Bearer tokens from the Authorization header.
// Paths matching any whitelist prefix are skipped.
func TokenAuth(verify VerifyFunc, whitelist ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		for _, prefix := range whitelist {
			if strings.HasPrefix(path, prefix) {
				c.Next()
				return
			}
		}

		auth := c.GetHeader("Authorization")
		// Explicitly require "Bearer " scheme; reject non-Bearer or bare tokens.
		if !strings.HasPrefix(auth, "Bearer ") {
			response.Unauthorized(c)
			c.Abort()
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		ok, err := verify(c.Request.Context(), token)
		if err != nil || !ok {
			response.Unauthorized(c)
			c.Abort()
			return
		}
		c.Next()
	}
}
