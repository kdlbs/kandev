package acp

import (
	"context"
	"encoding/json"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func codexResponseAttemptResetNotification(t *testing.T) acpsdk.SessionNotification {
	t.Helper()
	var notification acpsdk.SessionNotification
	raw := []byte(`{"sessionId":"session-1","update":{"sessionUpdate":"session_info_update","_meta":{"codex":{"error":{"willRetry":true,"codexErrorInfo":{"responseStreamDisconnected":{}}}}}}}`)
	if err := json.Unmarshal(raw, &notification); err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	return notification
}

func TestCodexResponseAttemptResetRequiresExactStructuredMetadata(t *testing.T) {
	valid := map[string]any{
		"codex": map[string]any{
			"error": map[string]any{
				"willRetry": true,
				"codexErrorInfo": map[string]any{
					"responseStreamDisconnected": map[string]any{},
				},
			},
		},
	}
	if !codexResponseAttemptResetMeta(valid) {
		t.Fatal("exact Codex response-disconnect retry metadata was not recognized")
	}

	tests := map[string]map[string]any{
		"false retry flag": {
			"codex": map[string]any{"error": map[string]any{
				"willRetry": false,
				"codexErrorInfo": map[string]any{
					"responseStreamDisconnected": map[string]any{},
				},
			}},
		},
		"string retry flag": {
			"codex": map[string]any{"error": map[string]any{
				"willRetry": "true",
				"codexErrorInfo": map[string]any{
					"responseStreamDisconnected": map[string]any{},
				},
			}},
		},
		"missing disconnect object": {
			"codex": map[string]any{"error": map[string]any{
				"willRetry":      true,
				"codexErrorInfo": map[string]any{},
			}},
		},
		"string disconnect value": {
			"codex": map[string]any{"error": map[string]any{
				"willRetry": true,
				"codexErrorInfo": map[string]any{
					"responseStreamDisconnected": "connection reset",
				},
			}},
		},
	}
	for name, meta := range tests {
		t.Run(name, func(t *testing.T) {
			if codexResponseAttemptResetMeta(meta) {
				t.Fatal("malformed metadata was recognized as a response-attempt reset")
			}
		})
	}
}

func TestObserveResponseAttemptResetRequiresCurrentPromptGeneration(t *testing.T) {
	a := newTestAdapter()
	a.dialect = newCodexACPDialect()
	t.Cleanup(func() { _ = a.Close() })
	_, turn := a.registerPromptTurn(context.Background(), 7)
	t.Cleanup(func() { a.clearPromptTurn(turn) })

	event := a.convertNotification(codexResponseAttemptResetNotification(t))
	if a.observesResponseAttemptReset(0, event) {
		t.Fatal("generation-zero metadata produced a response-attempt reset")
	}
	if a.observesResponseAttemptReset(6, event) {
		t.Fatal("stale metadata produced a response-attempt reset")
	}
	if !a.observesResponseAttemptReset(7, event) {
		t.Fatal("current prompt metadata did not produce a response-attempt reset")
	}
	if (acpDialect{}).resetsResponseAttempt(event.SessionMeta) {
		t.Fatal("unsupported dialect recognized Codex retry metadata")
	}
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.26
func TestHandleACPUpdateCodexResponseAttemptReset(t *testing.T) {
	a := newTestAdapter()
	a.agentID = codexAgentID
	a.dialect = newCodexACPDialect()
	t.Cleanup(func() { _ = a.Close() })

	_, turn := a.registerPromptTurn(context.Background(), 7)
	t.Cleanup(func() { a.clearPromptTurn(turn) })

	a.handleACPUpdate(codexResponseAttemptResetNotification(t), 7)
	a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{
		AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{
			Content: acpsdk.TextBlock("replacement"),
		},
	}), 7)

	events := drainEvents(a)
	if len(events) != 3 {
		t.Fatalf("events = %+v, want reset boundary, session info, then replacement", events)
	}
	if events[0].Type != streams.EventTypeResponseAttemptReset {
		t.Fatalf("first event type = %q, want %q", events[0].Type, streams.EventTypeResponseAttemptReset)
	}
	if events[0].PromptGeneration != 7 {
		t.Fatalf("reset generation = %d, want 7", events[0].PromptGeneration)
	}
	if events[0].SessionMeta != nil || events[0].Data != nil {
		t.Fatalf("reset exposed provider metadata: %+v", events[0])
	}
	if events[1].Type != streams.EventTypeSessionInfo {
		t.Fatalf("second event type = %q, want %q", events[1].Type, streams.EventTypeSessionInfo)
	}
	if events[2].Type != streams.EventTypeMessageChunk || events[2].Text != "replacement" {
		t.Fatalf("third event = %+v, want replacement message chunk", events[2])
	}
}
