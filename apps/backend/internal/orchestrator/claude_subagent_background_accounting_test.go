package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-003.6
func TestClaudePresentationUpdatesDoNotReopenCompletedBackgroundWork(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
	}{
		{name: "settlement", status: "completed"},
		{name: "resume", status: "running"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const taskID, sessionID, executionID, toolCallID, agentID = "task-claude", "session-claude", "execution-claude", "tool-claude", "agent-claude"
			svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
			launch := attestedSubagentPayload("Review changes", "Review the change", "general-purpose")
			launch.SubagentTask().IsAsync = true
			launch.SubagentTask().AgentID = agentID
			launch.SubagentTask().Status = "async_launched"
			launch.SetBackgroundWorkIdentity(streams.BackgroundWorkKindSubagent, agentID, true, false)
			svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
				TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
				Data: &lifecycle.AgentStreamEventData{
					Type: "tool_call", ToolCallID: toolCallID, ToolStatus: "pending", Normalized: launch,
				},
			})
			if got := svc.ActiveSubagentCount(sessionID); got != 1 {
				t.Fatalf("active subagent count after launch = %d, want 1", got)
			}

			svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
				TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
				Data: &lifecycle.AgentStreamEventData{
					Type: streams.EventTypeBackgroundComplete, ToolCallID: agentID,
				},
			})
			if got := svc.ActiveSubagentCount(sessionID); got != 0 {
				t.Fatalf("active subagent count after background completion = %d, want 0", got)
			}

			presentation := launch.Snapshot()
			presentation.SubagentTask().Status = tc.status
			if tc.status == "running" {
				presentation.SubagentTask().ResumeCount = 1
			}
			svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
				TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
				Data: &lifecycle.AgentStreamEventData{
					Type: "tool_update", ToolCallID: toolCallID, ToolStatus: "complete", Normalized: presentation,
				},
			})

			if got := svc.ActiveSubagentCount(sessionID); got != 0 {
				t.Fatalf("active subagent count after %s presentation update = %d, want 0", tc.name, got)
			}
			activity, ok := turnActivityRecord(t, svc, sessionID)
			if !ok {
				t.Fatal("expected the turn activity record to remain available for inspection")
			}
			activity.mu.Lock()
			backgroundCount := len(activity.background)
			activity.mu.Unlock()
			if backgroundCount != 0 {
				t.Fatalf("background entries after %s presentation update = %d, want 0", tc.name, backgroundCount)
			}
		})
	}
}
