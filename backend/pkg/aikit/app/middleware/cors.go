package middleware

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSConfig contains the CORS configuration
type CORSConfig struct {
	AllowOrigins     []string // Allow origins
	AllowMethods     []string // Allow HTTP methods
	AllowHeaders     []string // Allow headers
	ExposeHeaders    []string // Expose headers
	AllowCredentials bool     // Allow credentials
	MaxAge           int      // Max age in seconds
}

// DefaultCORSConfig returns the default CORS configuration
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "X-Request-ID", "Authorization", "Accept", "X-Task-Priority"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           86400, // 1 day
	}
}

// CORS returns the CORS middleware
func CORS(config ...CORSConfig) gin.HandlerFunc {
	cfg := DefaultCORSConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Set Access-Control-Allow-Origin
		if len(cfg.AllowOrigins) > 0 {
			if cfg.AllowOrigins[0] == "*" {
				if cfg.AllowCredentials {
					// When credentials are enabled, the spec forbids wildcard origin.
					// Echo the request Origin instead (Starlette's behavior).
					if origin != "" {
						c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
						// Add Vary: Origin so caches don't serve this to other origins
						c.Writer.Header().Add("Vary", "Origin")
					}
				} else {
					c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
				}
			} else {
				// Check if origin is allowed
				for _, o := range cfg.AllowOrigins {
					if o == origin {
						c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
						c.Writer.Header().Add("Vary", "Origin")
						break
					}
				}
			}
		}

		// Set other headers
		if cfg.AllowCredentials {
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if len(cfg.AllowHeaders) > 0 {
			c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ","))
		}
		if len(cfg.AllowMethods) > 0 {
			c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ","))
		}
		if len(cfg.ExposeHeaders) > 0 {
			c.Writer.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ","))
		}

		if cfg.MaxAge > 0 {
			c.Writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
		}

		// Handle preflight OPTIONS request
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
