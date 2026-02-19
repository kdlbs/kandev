package shared

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	tracerName      = "kandev-agentctl"
	maxAttrValueLen = 8192 // 8KB truncation for span event payloads
)

// Tracer returns the package-level tracer for agent protocol tracing.
// Requires KANDEV_DEBUG_AGENT_MESSAGES=true in addition to the OTel endpoint.
// Returns a no-op tracer when debug mode is off.
func Tracer() trace.Tracer {
	if !debugMode {
		return noop.NewTracerProvider().Tracer(tracerName)
	}
	return tracing.Tracer(tracerName)
}

// ShutdownTracing flushes pending spans and shuts down the provider.
func ShutdownTracing(ctx context.Context) error {
	return tracing.Shutdown(ctx)
}

// TraceProtocolEvent creates a single span for a received protocol notification.
// Two events are attached: "raw" with the original protocol JSON and "normalized"
// with the serialized AgentEvent, allowing side-by-side comparison in Jaeger/Tempo.
func TraceProtocolEvent(
	ctx context.Context,
	protocol, agentID string,
	eventType string,
	rawData json.RawMessage,
	normalized *streams.AgentEvent,
) {
	tracer := Tracer()
	spanName := protocol + "." + eventType

	_, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	span.SetAttributes(
		attribute.String("protocol", protocol),
		attribute.String("agent_id", agentID),
		attribute.String("event_type", eventType),
	)

	if normalized != nil {
		span.SetAttributes(attribute.String("session_id", normalized.SessionID))
	}

	// Attach raw protocol JSON as an event
	if len(rawData) > 0 {
		span.AddEvent("raw", trace.WithAttributes(
			attribute.String("data", truncate(string(rawData), maxAttrValueLen)),
		))
	}

	// Attach normalized AgentEvent as an event
	if normalized != nil {
		if normJSON, err := json.Marshal(normalized); err == nil {
			span.AddEvent("normalized", trace.WithAttributes(
				attribute.String("data", truncate(string(normJSON), maxAttrValueLen)),
			))
		}
	} else {
		span.AddEvent("normalized", trace.WithAttributes(
			attribute.Bool("conversion_failed", true),
		))
	}
}

// TraceProtocolRequest starts a span for an outgoing protocol request.
// The caller must call span.End() when the request completes, and may add
// attributes to record response data.
func TraceProtocolRequest(
	ctx context.Context,
	protocol, agentID, name string,
) (context.Context, trace.Span) {
	tracer := Tracer()
	spanName := protocol + "." + name

	ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(
		attribute.String("protocol", protocol),
		attribute.String("agent_id", agentID),
	)

	return ctx, span
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}
