package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

const TraceIDKey = "trace_id"

// TelemetryMiddleware starts a span per request and records basic metrics.
func TelemetryMiddleware(serviceName string) gin.HandlerFunc {
	tracer := otel.Tracer(serviceName)
	meter := otel.Meter(serviceName)

	requestCount, _ := meter.Int64Counter(
		"http.server.request.count",
	)
	requestLatency, _ := meter.Float64Histogram(
		"http.server.request.duration_ms",
	)

	return func(c *gin.Context) {
		start := time.Now()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		spanName := fmt.Sprintf("%s %s", c.Request.Method, path)
		ctx, span := tracer.Start(c.Request.Context(), spanName)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)

		traceID := span.SpanContext().TraceID().String()
		c.Set(TraceIDKey, traceID)
		c.Writer.Header().Set("X-Trace-Id", traceID)

		if logger := LoggerFromContext(c); logger != nil {
			c.Set(LoggerKey, logger.With("trace_id", traceID))
		}

		span.SetAttributes(
			semconv.HTTPMethodKey.String(c.Request.Method),
			semconv.HTTPRouteKey.String(path),
			semconv.UserAgentOriginalKey.String(c.Request.UserAgent()),
			semconv.ClientAddressKey.String(c.ClientIP()),
		)

		c.Next()

		status := c.Writer.Status()
		durationMs := float64(time.Since(start).Milliseconds())

		span.SetAttributes(semconv.HTTPStatusCodeKey.Int(status))
		switch {
		case status >= http.StatusInternalServerError:
			span.SetStatus(codes.Error, http.StatusText(status))
		case status >= http.StatusBadRequest:
			span.SetStatus(codes.Error, http.StatusText(status))
		}

		attrs := []attribute.KeyValue{
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.route", path),
			attribute.Int("http.status_code", status),
		}

		requestCount.Add(ctx, 1, metric.WithAttributes(attrs...))
		requestLatency.Record(ctx, durationMs, metric.WithAttributes(attrs...))
	}
}

// TraceIDFromContext returns the trace ID, if present.
func TraceIDFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}

	if v, ok := c.Get(TraceIDKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}

	return ""
}
