package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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
// Checking ObservedDetachedLaunch alone would pass identically under a
// separate-queue/goroutine implementation that loses the attestation before
// it is actually consumed, so this also dispatches the "complete" event on
// the same handleAgentStreamEvent path immediately after (no intervening
// turn_started) and asserts the attestation was consumed: the probe fired
// and the session parked.
func TestObservedDetachedLaunch_VisibleImmediatelyAfterAttestation(t *testing.T) {
	const taskID = "task-ordering"
	const sessionID = "session-ordering"
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step1")
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("load seeded session: %v", err)
	}
	// STARTING (not RUNNING): handleCompleteStreamEvent defers the
	// running->waiting transition to a later READY event when the session is
	// still RUNNING at complete time, mirroring
	// parked_projection_turn_finished_ordering_test.go's seeding choice.
	session.State = models.TaskSessionStateStarting
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("seed STARTING state: %v", err)
	}

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	probe := &spyBackgroundProbe{results: []executor.ProbeResult{executor.ProbeResultLive}}
	svc.SetBackgroundProbe(probe)

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: "exec-1",
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

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{Type: agentEventComplete},
	})

	if probe.callCount() == 0 {
		t.Fatalf("expected the background probe to have been called at turn-settle")
	}
	if parked, _ := svc.ParkedSnapshot(sessionID); !parked {
		t.Fatalf("expected the session to be parked after the attested turn settled with a live probe result")
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
