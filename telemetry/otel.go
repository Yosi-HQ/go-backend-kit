package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

type Config struct {
	ServiceName       string
	Environment       string
	Version           string
	OTLPEndpoint      string
	OTLPInsecure      bool
	TracesEnabled     bool
	MetricsEnabled    bool
	PrometheusEnabled bool
	SampleRatio       float64
}

type Handle struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *metric.MeterProvider
}

func Init(ctx context.Context, cfg Config) (*Handle, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "go-service"
	}
	if cfg.SampleRatio == 0 {
		cfg.SampleRatio = 1
	}
	if cfg.SampleRatio < 0 || cfg.SampleRatio > 1 {
		return nil, fmt.Errorf("sample ratio must be between 0 and 1")
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		attributeString("deployment.environment", cfg.Environment),
		attributeString("service.version", cfg.Version),
	)

	handle := &Handle{}
	if cfg.TracesEnabled {
		tp, err := newTracerProvider(ctx, cfg, res)
		if err != nil {
			return nil, err
		}
		handle.tracerProvider = tp
		otel.SetTracerProvider(tp)
	}

	if cfg.MetricsEnabled {
		mp, err := newMeterProvider(ctx, cfg, res)
		if err != nil {
			return nil, err
		}
		handle.meterProvider = mp
		otel.SetMeterProvider(mp)
	}

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	if cfg.PrometheusEnabled {
		if err := RegisterPrometheusCollectors(prometheus.DefaultRegisterer); err != nil {
			return nil, err
		}
	}

	return handle, nil
}

// InitTelemetry keeps the original simple API. It configures providers from
// OTEL_EXPORTER_OTLP_ENDPOINT and returns a shutdown function.
func InitTelemetry(serviceName string) func(context.Context) error {
	handle, err := Init(context.Background(), Config{
		ServiceName:       serviceName,
		Environment:       os.Getenv("APP_ENV"),
		Version:           os.Getenv("APP_VERSION"),
		OTLPEndpoint:      os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		OTLPInsecure:      os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") == "true",
		TracesEnabled:     true,
		MetricsEnabled:    true,
		PrometheusEnabled: true,
		SampleRatio:       1,
	})
	if err != nil {
		return func(context.Context) error { return err }
	}
	return handle.Shutdown
}

func (h *Handle) Shutdown(ctx context.Context) error {
	if h == nil {
		return nil
	}

	var errs []error
	if h.tracerProvider != nil {
		errs = append(errs, h.tracerProvider.Shutdown(ctx))
	}
	if h.meterProvider != nil {
		errs = append(errs, h.meterProvider.Shutdown(ctx))
	}

	return errors.Join(errs...)
}

func ShutdownWithTimeout(ctx context.Context, timeout time.Duration, shutdown func(context.Context) error) error {
	if shutdown == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return shutdown(shutdownCtx)
}

func newTracerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	}

	if cfg.OTLPEndpoint != "" {
		exporterOptions := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint)}
		if cfg.OTLPInsecure {
			exporterOptions = append(exporterOptions, otlptracegrpc.WithInsecure())
		}
		exporter, err := otlptracegrpc.New(ctx, exporterOptions...)
		if err != nil {
			return nil, fmt.Errorf("trace exporter setup failed: %w", err)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}

	return sdktrace.NewTracerProvider(options...), nil
}

func newMeterProvider(ctx context.Context, cfg Config, res *resource.Resource) (*metric.MeterProvider, error) {
	options := []metric.Option{metric.WithResource(res)}

	if cfg.OTLPEndpoint != "" {
		exporterOptions := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint)}
		if cfg.OTLPInsecure {
			exporterOptions = append(exporterOptions, otlpmetricgrpc.WithInsecure())
		}
		exporter, err := otlpmetricgrpc.New(ctx, exporterOptions...)
		if err != nil {
			return nil, fmt.Errorf("metric exporter setup failed: %w", err)
		}
		options = append(options, metric.WithReader(metric.NewPeriodicReader(exporter)))
	}

	return metric.NewMeterProvider(options...), nil
}

func attributeString(key string, value string) attribute.KeyValue {
	if value == "" {
		return attribute.String(key, "unknown")
	}
	return attribute.String(key, value)
}

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "go_backend_kit_http_requests_total",
			Help: "Total number of HTTP requests handled by Gin middleware.",
		},
		[]string{"method", "route", "status"},
	)
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "go_backend_kit_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)
	dbQueriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "go_backend_kit_db_queries_total",
			Help: "Total number of database operations observed by toolkit instrumentation.",
		},
		[]string{"operation", "table", "status"},
	)
	dbQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "go_backend_kit_db_query_duration_seconds",
			Help:    "Database operation duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "table", "status"},
	)
	redisCommandsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "go_backend_kit_redis_commands_total",
			Help: "Total number of Redis commands observed by toolkit instrumentation.",
		},
		[]string{"command", "status"},
	)
	redisCommandDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "go_backend_kit_redis_command_duration_seconds",
			Help:    "Redis command duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"command", "status"},
	)
)

func RegisterPrometheusCollectors(reg prometheus.Registerer) error {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	return errors.Join(
		registerCollector(reg, httpRequestsTotal),
		registerCollector(reg, httpRequestDuration),
		registerCollector(reg, dbQueriesTotal),
		registerCollector(reg, dbQueryDuration),
		registerCollector(reg, redisCommandsTotal),
		registerCollector(reg, redisCommandDuration),
	)
}

func MetricsHandler() http.Handler {
	_ = RegisterPrometheusCollectors(prometheus.DefaultRegisterer)
	return promhttp.Handler()
}

func ObserveHTTPRequest(method string, route string, status int, duration time.Duration) {
	if route == "" {
		route = "unknown"
	}
	statusLabel := strconv.Itoa(status)
	httpRequestsTotal.WithLabelValues(method, route, statusLabel).Inc()
	httpRequestDuration.WithLabelValues(method, route, statusLabel).Observe(duration.Seconds())
}

func ObserveDBQuery(operation string, table string, status string, duration time.Duration) {
	if table == "" {
		table = "unknown"
	}
	if status == "" {
		status = "ok"
	}
	dbQueriesTotal.WithLabelValues(operation, table, status).Inc()
	dbQueryDuration.WithLabelValues(operation, table, status).Observe(duration.Seconds())
}

func ObserveRedisCommand(command string, status string, duration time.Duration) {
	if command == "" {
		command = "unknown"
	}
	if status == "" {
		status = "ok"
	}
	redisCommandsTotal.WithLabelValues(command, status).Inc()
	redisCommandDuration.WithLabelValues(command, status).Observe(duration.Seconds())
}

func registerCollector(reg prometheus.Registerer, collector prometheus.Collector) error {
	err := reg.Register(collector)
	if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
		return nil
	}
	return err
}
