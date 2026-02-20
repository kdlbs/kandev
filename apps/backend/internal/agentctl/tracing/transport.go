package tracing

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const transportTracerName = "kandev-transport"

func transportTracer() trace.Tracer {
	return Tracer(transportTracerName)
}

// TraceSessionStart creates a long-lived span for a task session.
// The caller must call span.End() when the session completes.
// All operations for the session should be created as children of this span's context.
func TraceSessionStart(ctx context.Context, taskID, sessionID, executionID string) (context.Context, trace.Span) {
	ctx, span := transportTracer().Start(ctx, "session",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(
		attribute.String("task_id", taskID),
		attribute.String("session_id", sessionID),
		attribute.String("execution_id", executionID),
	)
	return ctx, span
}

// TraceSessionRecovered creates a session span for a recovered execution (after backend restart).
func TraceSessionRecovered(ctx context.Context, taskID, sessionID, executionID string) (context.Context, trace.Span) {
	ctx, span := transportTracer().Start(ctx, "session.recovered",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(
		attribute.String("task_id", taskID),
		attribute.String("session_id", sessionID),
		attribute.String("execution_id", executionID),
		attribute.Bool("recovered", true),
	)
	return ctx, span
}

// TraceSessionInit creates a short-lived span covering session initialization.
// This span is a child of the session span and ends when init completes,
// ensuring init-phase operations are visible in trace backends while the
// parent session span is still open.
func TraceSessionInit(ctx context.Context, taskID, sessionID, executionID string) (context.Context, trace.Span) {
	ctx, span := transportTracer().Start(ctx, "session.init",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(
		attribute.String("task_id", taskID),
		attribute.String("session_id", sessionID),
		attribute.String("execution_id", executionID),
	)
	return ctx, span
}

// TraceHTTPRequest starts a span for an HTTP call to agentctl.
// Caller must call span.End() when the response is received.
func TraceHTTPRequest(ctx context.Context, method, path, executionID string) (context.Context, trace.Span) {
	ctx, span := transportTracer().Start(ctx, "http."+method+" "+path,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	span.SetAttributes(
		attribute.String("http.method", method),
		attribute.String("http.path", path),
		attribute.String("execution_id", executionID),
	)
	return ctx, span
}

// TraceHTTPResponse records response attributes on the span.
func TraceHTTPResponse(span trace.Span, statusCode int, err error) {
	span.SetAttributes(attribute.Int("http.status_code", statusCode))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// TraceWSRequest starts a span for an outgoing WebSocket request.
// Caller must call span.End() when the response arrives.
func TraceWSRequest(ctx context.Context, action, msgID, executionID, sessionID string) (context.Context, trace.Span) {
	ctx, span := transportTracer().Start(ctx, "ws."+action,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	span.SetAttributes(
		attribute.String("ws.action", action),
		attribute.String("ws.msg_id", msgID),
		attribute.String("execution_id", executionID),
		attribute.String("session_id", sessionID),
	)
	return ctx, span
}

// TraceWSResponse records response attributes on the span.
func TraceWSResponse(span trace.Span, responseType string, err error) {
	span.SetAttributes(attribute.String("ws.response_type", responseType))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// TraceAgentEvent creates a single span for a received agent event.
// The raw JSON message is attached as a span event for full observability.
func TraceAgentEvent(ctx context.Context, eventType, sessionID, executionID string, rawMessage json.RawMessage) {
	_, span := transportTracer().Start(ctx, "agent.event."+eventType,
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	span.SetAttributes(
		attribute.String("event_type", eventType),
		attribute.String("session_id", sessionID),
		attribute.String("execution_id", executionID),
	)

	if len(rawMessage) > 0 {
		data := string(rawMessage)
		if len(data) > maxEventDataLen {
			data = data[:maxEventDataLen] + "...(truncated)"
		}
		span.AddEvent("event_data", trace.WithAttributes(
			attribute.String("data", data),
		))
	}
}

const maxEventDataLen = 8192

// TraceWorkspaceEvent creates a single span for a workspace stream event.
func TraceWorkspaceEvent(ctx context.Context, msgType, executionID, sessionID string) {
	_, span := transportTracer().Start(ctx, "workspace."+msgType,
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	span.SetAttributes(
		attribute.String("ws.msg_type", msgType),
		attribute.String("execution_id", executionID),
		attribute.String("session_id", sessionID),
	)
}

// TraceTurnEnd creates a span marking the end of an agent turn.
// The caller should set additional attributes (stop_reason, is_error) before ending the span.
func TraceTurnEnd(ctx context.Context, executionID, sessionID string) (context.Context, trace.Span) {
	ctx, span := transportTracer().Start(ctx, "turn_end",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(
		attribute.String("execution_id", executionID),
		attribute.String("session_id", sessionID),
	)
	return ctx, span
}

// TraceMCPDispatch starts a span for an MCP request relay.
// Caller must call span.End() when the dispatch completes.
func TraceMCPDispatch(ctx context.Context, action, msgID, executionID string) (context.Context, trace.Span) {
	ctx, span := transportTracer().Start(ctx, "mcp.dispatch."+action,
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(
		attribute.String("mcp.action", action),
		attribute.String("mcp.msg_id", msgID),
		attribute.String("execution_id", executionID),
	)
	return ctx, span
}

// TraceMCPResponse records the result of an MCP dispatch on the span.
func TraceMCPResponse(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}
