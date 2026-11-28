package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const cursorPromptStreamClosedMessage = "Cursor prompt stream closed: WritableIterable is closed"

func TestCursorPromptStreamClosedContinuationEvidence(t *testing.T) {
	failure := sdk.NewInternalError(map[string]any{"details": "WritableIterable is closed"})
	events, err := runCursorPromptFailure(t, cursorAgentID, failure, true)
	if err != nil {
		t.Fatalf("Prompt returned an error: %v", err)
	}

	var promptFailure *AgentEvent
	messageIndex := -1
	failureIndex := -1
	for i := range events {
		if events[i].Type == streams.EventTypeMessageChunk && events[i].Text == "partial response" {
			messageIndex = i
		}
		if events[i].Type == streams.EventTypeError {
			if promptFailure != nil {
				t.Fatal("received more than one prompt error event")
			}
			promptFailure = &events[i]
			failureIndex = i
		}
	}
	if promptFailure == nil {
		t.Fatalf("events = %+v, want one prompt error", events)
	}
	if messageIndex < 0 || messageIndex >= failureIndex {
		t.Fatalf("events = %+v, want the streamed response before the prompt error", events)
	}
	if promptFailure.ProviderError == nil ||
		promptFailure.ProviderError.Source != streams.ProviderErrorSourceCursorACP ||
		promptFailure.ProviderError.ProviderID != acpcompat.CursorAgentID ||
		promptFailure.ProviderError.Message != cursorPromptStreamClosedMessage {
		t.Fatalf("provider error = %+v, want the fixed Cursor diagnostic", promptFailure.ProviderError)
	}
	if promptFailure.ProviderError.OccurredAt.IsZero() {
		t.Fatal("provider error timestamp is empty")
	}
	if promptFailure.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
		t.Fatalf("failure disposition = %q, want retain_runtime", promptFailure.PromptFailureDisposition)
	}
	if promptFailure.ContinuationSafety == nil || !promptFailure.ContinuationSafety.SafeFor(7) ||
		promptFailure.ContinuationSafety.CompletedTools != 1 {
		t.Fatalf("continuation evidence = %+v, want one completed safe tool for generation 7", promptFailure.ContinuationSafety)
	}
	encoded, err := json.Marshal(promptFailure)
	if err != nil {
		t.Fatalf("marshal prompt failure: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode prompt failure: %v", err)
	}
	if _, ok := fields["data"]; ok {
		t.Fatalf("raw request data crossed the event boundary: %s", encoded)
	}
	if strings.Contains(string(encoded), `"details"`) {
		t.Fatalf("raw request details crossed the event boundary: %s", encoded)
	}
}

func TestCursorPromptStreamClosedNoOutputKeepsReplayEvidence(t *testing.T) {
	failure := sdk.NewInternalError(map[string]any{"details": "WritableIterable is closed"})
	events, err := runCursorPromptFailure(t, cursorAgentID, failure, false)
	if err != nil {
		t.Fatalf("Prompt returned an error: %v", err)
	}

	var promptFailure *AgentEvent
	for i := range events {
		switch events[i].Type {
		case streams.EventTypeMessageChunk, streams.EventTypeReasoning, streams.EventTypeToolCall,
			streams.EventTypeToolUpdate, streams.EventTypeShellOutputUpdate, streams.EventTypePlan,
			streams.EventTypeAgentPlan, streams.EventTypeBackgroundComplete, streams.EventTypePermissionRequest:
			t.Fatalf("no-output prompt emitted activity: %+v", events[i])
		case streams.EventTypeError:
			if promptFailure != nil {
				t.Fatal("received more than one prompt error event")
			}
			promptFailure = &events[i]
		}
	}
	if promptFailure == nil {
		t.Fatalf("events = %+v, want one prompt error", events)
	}
	if promptFailure.ContinuationSafety == nil || !promptFailure.ContinuationSafety.SafeFor(7) ||
		promptFailure.ContinuationSafety.CompletedTools != 0 || promptFailure.ContinuationSafety.Pending {
		t.Fatalf("continuation evidence = %+v, want safe empty evidence for generation 7", promptFailure.ContinuationSafety)
	}
}

func TestCursorPromptStreamClosedRejectsOtherShapes(t *testing.T) {
	var nullData any
	if err := json.Unmarshal([]byte("null"), &nullData); err != nil {
		t.Fatalf("decode null request data: %v", err)
	}
	for _, tc := range []struct {
		name    string
		agentID string
		message string
		failure error
	}{
		{name: "bare code", agentID: cursorAgentID, message: "Internal error", failure: sdk.NewInternalError(nil)},
		{name: "null data", agentID: cursorAgentID, message: "Internal error", failure: &sdk.RequestError{Code: -32603, Message: "Internal error", Data: nullData}},
		{name: "extra field", agentID: cursorAgentID, message: "Internal error", failure: sdk.NewInternalError(map[string]any{"details": "WritableIterable is closed", "extra": true})},
		{name: "other details", agentID: cursorAgentID, message: "Internal error", failure: sdk.NewInternalError(map[string]any{"details": "another stream is closed"})},
		{name: "different casing", agentID: cursorAgentID, message: "Internal error", failure: sdk.NewInternalError(map[string]any{"details": "writableIterable is closed"})},
		{name: "wrong message", agentID: cursorAgentID, message: "internal error", failure: &sdk.RequestError{Code: -32603, Message: "internal error", Data: map[string]any{"details": "WritableIterable is closed"}}},
		{name: "wrong code", agentID: cursorAgentID, message: "Internal error", failure: &sdk.RequestError{Code: -32000, Message: "Internal error", Data: map[string]any{"details": "WritableIterable is closed"}}},
		{name: "non-Cursor adapter", agentID: claudeAgentID, message: "Internal error", failure: sdk.NewInternalError(map[string]any{"details": "WritableIterable is closed"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, err := runCursorPromptFailure(t, tc.agentID, tc.failure, false)
			if err != nil {
				t.Fatalf("Prompt returned an error: %v", err)
			}
			var promptFailure *AgentEvent
			for i := range events {
				if events[i].Type == streams.EventTypeError {
					promptFailure = &events[i]
					break
				}
			}
			if promptFailure == nil || promptFailure.ProviderError == nil ||
				promptFailure.ProviderError.Message != tc.message || promptFailure.ContinuationSafety != nil {
				t.Fatalf("events = %+v, want the generic retained error without continuation evidence", events)
			}
		})
	}
}

func TestCursorPromptStreamClosedRequiresRemoteResponse(t *testing.T) {
	local := sdk.NewInternalError(map[string]any{"details": "WritableIterable is closed"})
	if cursorPromptStreamClosed(local) {
		t.Fatal("local request error was treated as peer transport evidence")
	}
}

func runCursorPromptFailure(
	t *testing.T,
	agentID string,
	failure error,
	emitEvidence bool,
) ([]AgentEvent, error) {
	t.Helper()
	a, fake, conn := setupHandoffFakeAgent(t)
	a.agentID = agentID
	a.normalizer = NewNormalizer(agentID)
	a.dialect = newACPDialect(agentID)
	if err := a.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize adapter: %v", err)
	}
	a.capabilities.LoadSession = true
	if _, err := a.NewSession(context.Background(), nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	_ = drainEvents(a)
	fake.promptFailure = failure
	done := make(chan error, 1)
	go func() { done <- a.Prompt(context.Background(), "prompt", nil, 7) }()
	select {
	case <-fake.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not reach the fake agent")
	}
	if emitEvidence {
		sendCapturedUpdate(t, conn, `{"sessionId":"session-handoff","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"partial response"}}}`)
		sendCapturedUpdate(t, conn, `{"sessionId":"session-handoff","update":{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed"}}`)
	}
	fake.releasePrompts()
	select {
	case err := <-done:
		return drainEvents(a), err
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not settle")
		return nil, nil
	}
}
