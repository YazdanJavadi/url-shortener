// Package tracing initializes the OpenTelemetry tracer provider that exports
// spans to a collector over OTLP. Jaeger all-in-one accepts OTLP (HTTP :4318 or
// gRPC :4317) natively since Jaeger 1.35, so no Jaeger-specific client is
// needed — we speak OTLP and Jaeger ingests it.
//
// OpenTelemetry (OTel) is the CNCF standard merging OpenTracing and
// OpenCensus (both created ~2016, merged into OTel ~2019). It decouples
// instrumentation from the backend: code emits spans via the OTel API, and
// the exporter sends them to whichever collector is configured (Jaeger,
// Tempo, Zipkin, Datadog, …) without code changes.
package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config controls tracer setup.
type Config struct {
	ServiceName string
	Endpoint    string // OTLP HTTP endpoint host:port, e.g. "jaeger:4318"
	Insecure    bool   // true for plaintext (container network)
}

// Init configures a global TracerProvider exporting spans via OTLP HTTP to the
// collector. The returned shutdown function flushes and stops the provider and
// MUST be called on process exit.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceNameKey.String(cfg.ServiceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("create trace resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter), // async batching for low overhead
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(1.0)), // sample 100% for the demo
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Graceful shutdown: flush pending spans.
	shutdown := func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		return tp.Shutdown(ctx)
	}

	return shutdown, nil
}
