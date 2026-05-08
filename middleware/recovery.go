package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/yosi-hq/go-backend-kit/logger"
)

func RecoveryMiddleware(base logger.Logger) gin.HandlerFunc {
	if base == nil {
		base = logger.New("production")
	}

	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log := base
				if requestLogger := LoggerFromContext(c); requestLogger != nil {
					log = requestLogger
				}

				log.ErrorContext(
					c.Request.Context(),
					"panic recovered",
					"panic", recovered,
					"request_id", RequestIDFromContext(c),
					"trace_id", TraceIDFromContext(c),
					"stack", string(debug.Stack()),
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			}
		}()

		c.Next()
	}
}
