package acp

import (
	"testing"
)

// @covers AC-AGENTS-BACKGROUND-WORK-010.8
func TestClaudeMonitorSurvivesPromptCompletion(t *testing.T) {
	const sessionID, taskID, toolCallID = "session-monitor", "task-monitor", "tool-monitor"
	a := newTestAdapterForAgent(claudeAgentID)
	t.Cleanup(func() { _ = a.Close() })

	seedMonitor(t, a, sessionID, taskID, toolCallID)
	payload := newMonitorGeneric()
	payload.SetMonitorIdentity(taskID, false)
	a.activeToolCalls[toolCallID] = payload
	drainEvents(a)

	a.cancelPromptEndToolCalls(sessionID)

	if !a.isTrackedMonitor(sessionID, toolCallID) {
		t.Fatal("live Monitor binding was removed when the foreground prompt completed")
	}
	if !payload.IsActiveMonitor() {
		t.Fatal("live Monitor projection was marked terminal at prompt completion")
	}
	if events := drainEvents(a); len(events) != 0 {
		t.Fatalf("prompt completion emitted Monitor terminal events without provider evidence: %+v", events)
	}
	if _, tracked := a.activeToolCalls[toolCallID]; !tracked {
		t.Fatal("live Monitor tool payload was removed at prompt completion")
	}
	if payload.BackgroundWork() == nil || !payload.IsActiveBackgroundWork() || payload.BackgroundWork().Ended {
		t.Fatalf("active Monitor workload = %+v, want running", payload.BackgroundWork())
	}
}
