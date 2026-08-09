package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// shellToolCallPayload returns a normalised payload that satisfies
// IsDetachedBackgroundLaunch() and Kind==BackgroundWorkKindShell.
func shellToolCallPayload() *streams.NormalizedPayload {
	return attestedBackgroundShellPayload("sleep 999")
}

// subagentToolCallPayload returns a normalised payload with Detached=true but
// Kind==BackgroundWorkKindSubagent. This is how stampSubagentBackgroundWork
// marks both Claude and mock-agent subagent spawns — it must NOT set
// observed_detached (AC-37 second GIVEN, AC-69a).
func subagentToolCallPayload() *streams.NormalizedPayload {
	payload := attestedSubagentPayload("async task", "do it", "general-purpose")
	payload.SubagentTask().IsAsync = true
	payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindSubagent, "sub-1", true, false)
	return payload
}

func dispatchToolCall(svc *Service, taskID, sessionID, executionID, toolCallID string, norm *streams.NormalizedPayload) {
	svc.handleAgentStreamEvent(context.TODO(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: toolCallID,
			ToolStatus: "pending",
			Normalized: norm,
		},
	})
}

func dispatchTurnStarted(svc *Service, sessionID string) {
	svc.handleAgentStreamEvent(context.TODO(), &lifecycle.AgentStreamEventPayload{
		SessionID: sessionID,
		Data:      &lifecycle.AgentStreamEventData{Type: streams.EventTypeTurnStarted},
	})
}

// TestParkedState_ObservedDetachedSetOnShellLaunch verifies that a Kind==shell
// detached background launch sets observed_detached (AC-69, AC-69a).
func TestParkedState_ObservedDetachedSetOnShellLaunch(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID, executionID = "task-shell", "session-shell", "exec-shell"

	dispatchToolCall(svc, taskID, sessionID, executionID, "tool-shell", shellToolCallPayload())

	ps := svc.parkedStateFor(sessionID)
	if ps == nil {
		t.Fatal("expected parked state entry, got nil")
	}
	if !ps.observedDetached {
		t.Fatal("expected observedDetached=true after shell launch, got false")
	}
}

// TestParkedState_KindFilterSubagentDoesNotSetObservedDetached verifies that a
// subagent spawn with Detached=true and Kind==subagent does NOT set
// observed_detached (AC-37 second GIVEN, AC-69a).
func TestParkedState_KindFilterSubagentDoesNotSetObservedDetached(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID, executionID = "task-sub", "session-sub", "exec-sub"

	dispatchToolCall(svc, taskID, sessionID, executionID, "tool-sub", subagentToolCallPayload())

	ps := svc.parkedStateFor(sessionID)
	if ps != nil && ps.observedDetached {
		t.Fatal("subagent Kind spawn must not set observedDetached (AC-37 second GIVEN)")
	}
}

// TestParkedState_TurnMarkerIncrementsOnTurnStarted verifies that each
// turn_started event increments turnMarker (AC-41b).
func TestParkedState_TurnMarkerIncrementsOnTurnStarted(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-turns"

	dispatchTurnStarted(svc, sessionID)
	dispatchTurnStarted(svc, sessionID)

	ps := svc.parkedStateFor(sessionID)
	if ps == nil {
		t.Fatal("expected parked state entry after turn_started, got nil")
	}
	if ps.turnMarker != 2 {
		t.Fatalf("turnMarker = %d after 2 turn_started events, want 2", ps.turnMarker)
	}
}

// TestParkedState_ClearedOnSessionEviction verifies that parked state is
// deleted when a session's execution activity is retired.
func TestParkedState_ClearedOnSessionEviction(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID, executionID = "task-evict", "session-evict", "exec-evict"

	// Seed some state.
	dispatchTurnStarted(svc, sessionID)
	dispatchToolCall(svc, taskID, sessionID, executionID, "tool-evict", shellToolCallPayload())
	if svc.parkedStateFor(sessionID) == nil {
		t.Fatal("expected parked state before eviction")
	}

	// Force-clear via clearTurnActivity (session removal path).
	svc.clearTurnActivity(sessionID)

	if svc.parkedStateFor(sessionID) != nil {
		t.Fatal("expected parked state deleted after clearTurnActivity, got non-nil")
	}
}

// TestParkedState_MockAgentShellLaunchSetsObservedDetached verifies that the
// orchestrator reads only the normalised Kind predicate and not an agent-name
// or agent-id field — so a mock-agent shell launch (same normalised path as
// Claude) correctly sets observed_detached (AC-69a).
func TestParkedState_MockAgentShellLaunchSetsObservedDetached(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	// Simulate a mock-agent run: the payload shape is identical to Claude's
	// because stampBackgroundShellWork is vendor-neutral (both agents go through
	// the same normaliser path).
	const taskID, sessionID, executionID = "task-mock", "session-mock", "exec-mock"

	dispatchToolCall(svc, taskID, sessionID, executionID, "tool-mock", shellToolCallPayload())

	ps := svc.parkedStateFor(sessionID)
	if ps == nil || !ps.observedDetached {
		t.Fatal("mock-agent shell launch must set observedDetached (AC-69a)")
	}
}
