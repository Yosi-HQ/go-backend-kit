package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           time.Duration
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type", "X-Request-Id"},
		ExposeHeaders: []string{
			"X-Request-Id",
			"X-Trace-Id",
		},
		MaxAge: 12 * time.Hour,
	}
}

func CORS() gin.HandlerFunc {
	return CORSWithConfig(DefaultCORSConfig())
}

func CORSWithConfig(cfg CORSConfig) gin.HandlerFunc {
	if len(cfg.AllowOrigins) == 0 {
		cfg.AllowOrigins = []string{"*"}
	}
	if len(cfg.AllowMethods) == 0 {
		cfg.AllowMethods = DefaultCORSConfig().AllowMethods
	}
	if len(cfg.AllowHeaders) == 0 {
		cfg.AllowHeaders = DefaultCORSConfig().AllowHeaders
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if allowedOrigin := allowedOrigin(origin, cfg.AllowOrigins, cfg.AllowCredentials); allowedOrigin != "" {
				c.Header("Access-Control-Allow-Origin", allowedOrigin)
				c.Header("Vary", "Origin")
			}
		} else if contains(cfg.AllowOrigins, "*") && !cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Origin", "*")
		}

		c.Header("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
		c.Header("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
		if len(cfg.ExposeHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
		}
		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if cfg.MaxAge > 0 {
			c.Header("Access-Control-Max-Age", strconv.Itoa(int(cfg.MaxAge.Seconds())))
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func allowedOrigin(origin string, allowed []string, credentials bool) string {
	for _, candidate := range allowed {
		if candidate == "*" {
			if credentials {
				return origin
			}
			return "*"
		}
		if candidate == origin {
			return origin
		}
	}
	return ""
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
