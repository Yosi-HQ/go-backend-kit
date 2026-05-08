package telemetry

import (
	"context"
	"errors"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// InitTelemetry configures tracing and metrics for the service.
func InitTelemetry(serviceName string) func(context.Context) error {
	ctx := context.Background()

	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		log.Fatal(err)
	}

	metricExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		log.Fatal(err)
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	)

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter)),
		metric.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return func(ctx context.Context) error {
		return errors.Join(
			tracerProvider.Shutdown(ctx),
			meterProvider.Shutdown(ctx),
		)
	}
}
