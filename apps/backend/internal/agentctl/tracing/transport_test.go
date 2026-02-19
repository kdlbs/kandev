package tracing

import (
	"context"
	"fmt"
	"testing"
)

func TestEndpointHost(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strips http prefix",
			input:    "http://localhost:4318",
			expected: "localhost:4318",
		},
		{
			name:     "strips https prefix",
			input:    "https://otel.example.com:4318",
			expected: "otel.example.com:4318",
		},
		{
			name:     "returns unchanged when no scheme",
			input:    "localhost:4318",
			expected: "localhost:4318",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := endpointHost(tt.input)
			if got != tt.expected {
				t.Errorf("endpointHost(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTracer(t *testing.T) {
	t.Run("returns non-nil tracer", func(t *testing.T) {
		tracer := Tracer("test-tracer")
		if tracer == nil {
			t.Error("expected non-nil tracer")
		}
	})

	t.Run("returns no-op tracer without env vars", func(t *testing.T) {
		// Without KANDEV_DEBUG_AGENT_MESSAGES=true, we get a no-op tracer
		tracer := Tracer("test-noop")
		if tracer == nil {
			t.Error("expected non-nil tracer")
		}
	})
}

func TestTraceHTTPRequest(t *testing.T) {
	ctx := context.Background()

	t.Run("returns non-nil context and span", func(t *testing.T) {
		returnedCtx, span := TraceHTTPRequest(ctx, "POST", "/api/v1/start", "exec-123")
		if returnedCtx == nil {
			t.Error("expected non-nil context")
		}
		if span == nil {
			t.Error("expected non-nil span")
		}
		span.End()
	})
}

func TestTraceHTTPResponse(t *testing.T) {
	ctx := context.Background()

	t.Run("records success", func(t *testing.T) {
		_, span := TraceHTTPRequest(ctx, "GET", "/api/v1/status", "exec-123")
		TraceHTTPResponse(span, 200, nil)
		span.End()
	})

	t.Run("records error", func(t *testing.T) {
		_, span := TraceHTTPRequest(ctx, "POST", "/api/v1/stop", "exec-123")
		TraceHTTPResponse(span, 500, fmt.Errorf("server error"))
		span.End()
	})
}

func TestTraceWSRequest(t *testing.T) {
	ctx := context.Background()

	t.Run("returns non-nil context and span", func(t *testing.T) {
		returnedCtx, span := TraceWSRequest(ctx, "agent.initialize", "msg-123", "exec-456")
		if returnedCtx == nil {
			t.Error("expected non-nil context")
		}
		if span == nil {
			t.Error("expected non-nil span")
		}
		TraceWSResponse(span, "response", nil)
		span.End()
	})
}

func TestTraceAgentEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("does not panic", func(t *testing.T) {
		TraceAgentEvent(ctx, "message_chunk", "sess-123", "exec-456")
	})

	t.Run("handles empty values", func(t *testing.T) {
		TraceAgentEvent(ctx, "", "", "")
	})
}

func TestTraceWorkspaceEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("does not panic", func(t *testing.T) {
		TraceWorkspaceEvent(ctx, "git_status", "exec-123")
	})
}

func TestTraceMCPDispatch(t *testing.T) {
	ctx := context.Background()

	t.Run("returns non-nil context and span", func(t *testing.T) {
		returnedCtx, span := TraceMCPDispatch(ctx, "tools/call", "msg-789", "exec-123")
		if returnedCtx == nil {
			t.Error("expected non-nil context")
		}
		if span == nil {
			t.Error("expected non-nil span")
		}
		TraceMCPResponse(span, nil)
		span.End()
	})

	t.Run("records error", func(t *testing.T) {
		_, span := TraceMCPDispatch(ctx, "tools/call", "msg-789", "exec-123")
		TraceMCPResponse(span, fmt.Errorf("dispatch failed"))
		span.End()
	})
}

func TestShutdown(t *testing.T) {
	t.Run("no-op shutdown does not error", func(t *testing.T) {
		if err := Shutdown(context.Background()); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})
}
