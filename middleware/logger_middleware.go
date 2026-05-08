package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	kitlog "github.com/yosi-hq/go-backend-kit/log"
)

const (
	LoggerKey    = "logger"
	RequestIDKey = "request_id"
)

// LoggerMiddleware creates a Gin logging middleware with a default logger.
func LoggerMiddleware() gin.HandlerFunc {
	return LoggerMiddlewareWith(nil)
}

func LoggerMiddlewareWith(base kitlog.Logger) gin.HandlerFunc {
	if base == nil {
		base = kitlog.New("production")
	}

	return func(c *gin.Context) {
		start := time.Now()
		requestID := generateRequestID()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		logger := base.With(
			"request_id", requestID,
			"method", c.Request.Method,
			"path", path,
			"user_agent", c.Request.UserAgent(),
			"client_ip", c.ClientIP(),
		)

		c.Set(LoggerKey, logger)
		c.Set(RequestIDKey, requestID)
		c.Writer.Header().Set("X-Request-Id", requestID)

		c.Next()

		status := c.Writer.Status()
		size := c.Writer.Size()
		if size < 0 {
			size = 0
		}

		fields := []any{
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"bytes", size,
		}

		if len(c.Errors) > 0 {
			fields = append(fields, "errors", c.Errors.Errors())
		}

		switch {
		case status >= http.StatusInternalServerError:
			logger.Error("request completed", fields...)
		case status >= http.StatusBadRequest:
			logger.Warn("request completed", fields...)
		default:
			logger.Info("request completed", fields...)
		}
	}
}

func generateRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(buf)
}

func LoggerFromContext(c *gin.Context) kitlog.Logger {
	if c == nil {
		return nil
	}

	if v, ok := c.Get(LoggerKey); ok {
		if l, ok := v.(kitlog.Logger); ok {
			return l
		}
	}

	return nil
}

func RequestIDFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}

	if v, ok := c.Get(RequestIDKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}

	return ""
}
