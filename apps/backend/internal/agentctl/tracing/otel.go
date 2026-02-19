// Package tracing provides shared OTel tracer initialization for agentctl
// communication layers (both agent protocol and backend<>agentctl transport).
//
// Real tracing requires OTEL_EXPORTER_OTLP_ENDPOINT to be set.
// Without it a no-op tracer is used (zero overhead).
package tracing

import (
	"context"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const serviceName = "kandev-agentctl"

var (
	initOnce       sync.Once
	tracerProvider trace.TracerProvider = noop.NewTracerProvider()
	sdkProvider    *sdktrace.TracerProvider
)

func initTracing() {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return
	}

	ctx := context.Background()

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpointHost(endpoint)),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		res = resource.Default()
	}

	sdkProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	tracerProvider = sdkProvider
	otel.SetTracerProvider(tracerProvider)
}

// endpointHost strips the scheme from the endpoint URL for otlptracehttp.
func endpointHost(endpoint string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(endpoint, prefix) {
			return endpoint[len(prefix):]
		}
	}
	return endpoint
}

// Tracer returns a named tracer. No-op when tracing is disabled.
func Tracer(name string) trace.Tracer {
	initOnce.Do(initTracing)
	return tracerProvider.Tracer(name)
}

// Shutdown flushes pending spans and shuts down the provider.
func Shutdown(ctx context.Context) error {
	if sdkProvider != nil {
		return sdkProvider.Shutdown(ctx)
	}
	return nil
}
