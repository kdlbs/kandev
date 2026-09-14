package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

const cursorTransportLostDiagnostic = "Error: RetriableError: HTTP/2 stream closed with error code CANCEL (0x8)"

func TestHandleTransientFailure_ReplayRequiresPromptAttemptEvidence(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)

	data := watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		EvidenceKnown:    true,
		ErrorMessage:     cursorTransportLostDiagnostic,
	}
	if svc.handleTransientFailure(context.Background(), data) {
		t.Fatal("handleTransientFailure scheduled replay without a process-local prompt-attempt record")
	}
	if _, ok := svc.transientRetries.Load("s1"); ok {
		t.Fatal("missing prompt-attempt evidence armed a retry entry")
	}
}

func TestHandleTransientFailure_ReplayRejectsOutputOrToolEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		data watcher.AgentEventData
	}{
		{
			name: "assistant output",
			data: watcher.AgentEventData{OutputObserved: true},
		},
		{
			name: "tool activity",
			data: watcher.AgentEventData{EffectObserved: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTransientTestService(t)
			t.Cleanup(svc.cancelAllTransientRetries)
			svc.beginPromptAttempt("s1", "execution-1", 7, false)
			svc.observePromptAttempt("s1", "execution-1", 7, tc.data.OutputObserved, tc.data.EffectObserved)
			data := tc.data
			data.TaskID = "t1"
			data.SessionID = "s1"
			data.AgentExecutionID = "execution-1"
			data.PromptGeneration = 7
			data.EvidenceKnown = true
			data.ErrorMessage = cursorTransportLostDiagnostic

			if svc.handleTransientFailure(context.Background(), data) {
				t.Fatalf("handleTransientFailure scheduled replay after %s", tc.name)
			}
		})
	}
}

func TestHandleTransientFailure_ReplayUsesCurrentPromptAttempt(t *testing.T) {
	svc, mc := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	svc.beginPromptAttempt("s1", "execution-1", 7, false)

	data := watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     cursorTransportLostDiagnostic,
	}
	if !svc.handleTransientFailure(context.Background(), data) {
		t.Fatal("current output-free prompt attempt did not schedule the existing retry owner")
	}
	v, ok := svc.transientRetries.Load("s1")
	if !ok {
		t.Fatal("expected one transient retry entry")
	}
	entry, ok := v.(*transientRetryEntry)
	if !ok || entry.attempt != 1 {
		t.Fatalf("retry entry = %#v, want first attempt", v)
	}
	if len(mc.sessionMessages) != 1 || mc.sessionMessages[0].metadata["failure_code"] != "agent_transport_lost" {
		t.Fatalf("retry status messages = %+v, want one transport-loss warning", mc.sessionMessages)
	}
}

func TestPromptAttemptEvidence_ObservesThoughtAndToolActivity(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	svc.beginPromptAttempt("s1", "execution-1", 7, false)

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "execution-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:             "thinking_streaming",
			Text:             "thinking about the request",
			PromptGeneration: 7,
		},
	})
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:      "t1",
		SessionID:   "s1",
		ExecutionID: "execution-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:             agentEventToolCall,
			ToolCallID:       "read-file-1",
			ToolTitle:        "Read File",
			ToolStatus:       "running",
			PromptGeneration: 7,
		},
	})

	data := watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     cursorTransportLostDiagnostic,
	}
	if svc.handleTransientFailure(context.Background(), data) {
		t.Fatal("thought and tool activity incorrectly authorized automatic replay")
	}
	if _, ok := svc.transientRetries.Load("s1"); ok {
		t.Fatal("unsafe thought/tool attempt armed a retry entry")
	}
}

func TestPromptAttemptEvidence_RejectsReplacedAttempt(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	svc.beginPromptAttempt("s1", "execution-1", 7, false)
	svc.beginPromptAttempt("s1", "execution-2", 8, false)

	data := watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     cursorTransportLostDiagnostic,
	}
	if svc.handleTransientFailure(context.Background(), data) {
		t.Fatal("stale execution/generation authorized automatic replay")
	}
}

func TestCursorTransportLost_UsesOneSameProviderRetryOwner(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	svc.beginPromptAttempt("s1", "execution-1", 7, false)

	data := watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		AgentID:          "cursor-acp",
		PromptGeneration: 7,
		ErrorMessage:     cursorTransportLostDiagnostic,
	}
	if !svc.handleTransientFailure(context.Background(), data) {
		t.Fatal("Cursor transport-loss failure did not use the existing retry owner")
	}
	if !svc.handleTransientFailure(context.Background(), data) {
		t.Fatal("second eligible failure did not reuse the existing retry owner")
	}
	v, ok := svc.transientRetries.Load("s1")
	if !ok {
		t.Fatal("retry owner disappeared after second eligible failure")
	}
	entry, ok := v.(*transientRetryEntry)
	if !ok || entry.attempt != 2 {
		t.Fatalf("retry entry = %#v, want the same owner at attempt 2", v)
	}
}
