package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// A terminal, Detached=true background-shell tool_call_update is the
// recognised condition for "a detached launch happened during this turn"
// (spec §I). It must set the session's attestation.
func TestTrackBackgroundToolUpdate_DetachedShellSetsObservedDetachedLaunch(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-detached-shell"

	if svc.ObservedDetachedLaunch(sessionID) {
		t.Fatalf("expected no attestation before any event")
	}

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: "task-1", SessionID: sessionID, ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "tool-1",
			ToolStatus: "completed",
			Normalized: attestedBackgroundShellPayload("sleep 300"),
		},
	})

	if !svc.ObservedDetachedLaunch(sessionID) {
		t.Errorf("expected ObservedDetachedLaunch to be true after a detached shell launch")
	}
}

// A detached subagent launch is a different signal (spec: "background-shell
// launch" only) and must not set the attestation.
func TestTrackBackgroundToolUpdate_DetachedSubagentDoesNotSetObservedDetachedLaunch(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-detached-subagent"

	payload := attestedSubagentPayload("background work", "do it", "general-purpose")
	payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindSubagent, "child-1", true, false)

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: "task-1", SessionID: sessionID, ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "tool-1",
			ToolStatus: "completed",
			Normalized: payload,
		},
	})

	if svc.ObservedDetachedLaunch(sessionID) {
		t.Errorf("expected a detached subagent launch to leave the shell attestation false")
	}
}

// D3: turn_started clears the attestation for the turn that is starting, so a
// stale attestation from turn N cannot mark turn N+1.
func TestHandleAgentStreamEvent_TurnStartedClearsObservedDetachedLaunch(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-turn-boundary"
	svc.setObservedDetachedLaunch(sessionID)
	if !svc.ObservedDetachedLaunch(sessionID) {
		t.Fatalf("expected attestation to be set before turn_started")
	}

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: "task-1", SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type: streams.EventTypeTurnStarted,
		},
	})

	if svc.ObservedDetachedLaunch(sessionID) {
		t.Errorf("expected turn_started to clear the attestation")
	}
}

// AC-79: the attestation and the turn-completion event arrive on the same
// ordered stream consumer. Dispatching the attestation frame immediately
// before a same-turn frame must leave the attestation visible — this pins
// the ordering an implementation on a separate queue or goroutine would lose.
func TestObservedDetachedLaunch_VisibleImmediatelyAfterAttestation(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-ordering"

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: "task-1", SessionID: sessionID, ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "tool-1",
			ToolStatus: "completed",
			Normalized: attestedBackgroundShellPayload("sleep 300"),
		},
	})

	// No intervening turn_started: the attestation must still describe the
	// turn that is settling.
	if !svc.ObservedDetachedLaunch(sessionID) {
		t.Fatalf("expected the attestation to be visible for the settling turn")
	}
}

func TestClearObservedDetachedLaunch_EmptySessionIDIsNoop(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.setObservedDetachedLaunch("") // must not panic or populate a "" entry
	if svc.ObservedDetachedLaunch("") {
		t.Errorf("expected empty session ID to never attest")
	}
	svc.clearObservedDetachedLaunch("") // must not panic
}
