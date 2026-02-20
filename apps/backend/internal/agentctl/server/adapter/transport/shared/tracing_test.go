package shared

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "returns unchanged when under limit",
			input:    "short",
			maxLen:   10,
			expected: "short",
		},
		{
			name:     "returns unchanged when exactly at limit",
			input:    "exact",
			maxLen:   5,
			expected: "exact",
		},
		{
			name:     "truncates with suffix when over limit",
			input:    "this is a long string that exceeds the limit",
			maxLen:   10,
			expected: "this is a ...(truncated)",
		},
		{
			name:     "handles empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
		})
	}

	// Verify the suffix is correct for truncated output
	t.Run("truncated output ends with expected suffix", func(t *testing.T) {
		got := truncate(strings.Repeat("x", 100), 10)
		if !strings.HasSuffix(got, "...(truncated)") {
			t.Errorf("truncated output should end with '...(truncated)', got %q", got)
		}
		if !strings.HasPrefix(got, strings.Repeat("x", 10)) {
			t.Errorf("truncated output should start with the first 10 chars")
		}
	})
}

func TestTraceProtocolEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("nil normalized event does not panic", func(t *testing.T) {
		TraceProtocolEvent(ctx, "acp", "agent-1", "message_chunk",
			json.RawMessage(`{"text":"hello"}`), nil)
	})

	t.Run("valid normalized event with raw data does not panic", func(t *testing.T) {
		event := &streams.AgentEvent{
			Type:      "message_chunk",
			SessionID: "sess-123",
			Text:      "hello world",
		}
		rawData := json.RawMessage(`{"type":"message_chunk","text":"hello world"}`)

		TraceProtocolEvent(ctx, "acp", "agent-1", "message_chunk", rawData, event)
	})

	t.Run("empty raw data does not panic", func(t *testing.T) {
		event := &streams.AgentEvent{
			Type:      "complete",
			SessionID: "sess-456",
		}

		TraceProtocolEvent(ctx, "acp", "agent-1", "complete", nil, event)
	})
}

func TestTracer_ReturnsSameNoopInstance(t *testing.T) {
	// When debug mode is off (default), Tracer() should return the same cached
	// noop tracer instance to avoid garbage on every call.
	t1 := Tracer()
	t2 := Tracer()
	if t1 != t2 {
		t.Error("expected Tracer() to return the same noop tracer instance when debug mode is off")
	}
}

func TestTraceProtocolRequest(t *testing.T) {
	t.Run("returns non-nil context and span", func(t *testing.T) {
		ctx := context.Background()

		returnedCtx, span := TraceProtocolRequest(ctx, "acp", "agent-1", "session/new")

		if returnedCtx == nil {
			t.Error("expected non-nil context, got nil")
		}
		if span == nil {
			t.Error("expected non-nil span, got nil")
		}

		// Clean up the span
		span.End()
	})
}
