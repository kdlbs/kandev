package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func claudeBackgroundShellPayload(command string, ended bool) *streams.NormalizedPayload {
	payload := streams.NewShellExec(command, "", "Run a detached command", 0, true)
	payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, "", true, ended)
	return payload
}

func feedClaudeBackgroundShellEvent(
	t *testing.T,
	svc *Service,
	taskID, sessionID, executionID, toolCallID, status string,
	payload *streams.NormalizedPayload,
	observation *streams.WorkloadRunObservation,
) {
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:           "tool_update",
			ToolCallID:     toolCallID,
			ToolStatus:     status,
			Normalized:     payload,
			BackgroundWork: observation,
		},
	})
}

// @covers AC-AGENTS-BACKGROUND-WORK-010.6
func TestClaudeBackgroundTerminalExitRetiresExactAccounting(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const (
		taskID      = "task-claude-terminal-accounting"
		sessionID   = "session-claude-terminal-accounting"
		executionID = "execution-claude-terminal-accounting"
		firstTool   = "toolu_e2e_claude_terminal_first"
		secondTool  = "toolu_e2e_claude_terminal_second"
	)

	firstLaunch := claudeBackgroundShellPayload("go test ./...", false)
	secondLaunch := claudeBackgroundShellPayload("gh pr checks --watch", false)
	for toolCallID, payload := range map[string]*streams.NormalizedPayload{
		firstTool: firstLaunch, secondTool: secondLaunch,
	} {
		svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
			TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
			Data: &lifecycle.AgentStreamEventData{
				Type: "tool_call", ToolCallID: toolCallID, ToolStatus: "pending", Normalized: payload,
			},
		})
	}

	// The launch tool completes while both commands keep running. This clears
	// tool ownership but must leave both accounting registrations outstanding.
	feedClaudeBackgroundShellEvent(t, svc, taskID, sessionID, executionID, firstTool, agentEventComplete, firstLaunch, nil)
	feedClaudeBackgroundShellEvent(t, svc, taskID, sessionID, executionID, secondTool, agentEventComplete, secondLaunch, nil)
	svc.markForegroundIdle(sessionID)
	require.True(t, svc.hasBackgroundTask(sessionID, firstTool))
	require.True(t, svc.hasBackgroundTask(sessionID, secondTool))

	firstEnd := &streams.WorkloadRunObservation{
		WorkID: firstTool, Kind: streams.WorkloadKindShell, Title: "Run the tests",
		State: streams.RunStateEnded, SourceCallID: firstTool,
	}
	feedClaudeBackgroundShellEvent(
		t, svc, taskID, sessionID, executionID, firstTool, agentEventComplete,
		claudeBackgroundShellPayload("go test ./...", true), firstEnd,
	)
	require.False(t, svc.hasBackgroundTask(sessionID, firstTool),
		"the exact terminal observation must retire its matching registration")
	require.True(t, svc.hasBackgroundTask(sessionID, secondTool),
		"a live sibling must stay registered")
	if got := svc.foregroundActivityValue(sessionID); got != "background" {
		t.Fatalf("activity with a live sibling = %q, want background", got)
	}

	// Duplicate delivery is harmless and cannot consume the sibling.
	feedClaudeBackgroundShellEvent(
		t, svc, taskID, sessionID, executionID, firstTool, agentEventComplete,
		claudeBackgroundShellPayload("go test ./...", true), firstEnd,
	)
	require.False(t, svc.hasBackgroundTask(sessionID, firstTool))
	require.True(t, svc.hasBackgroundTask(sessionID, secondTool))

	// A terminal observation attributed to a successor execution cannot settle
	// work registered by the predecessor.
	secondEnd := &streams.WorkloadRunObservation{
		WorkID: secondTool, Kind: streams.WorkloadKindShell, Title: "Watch checks",
		State: streams.RunStateEnded, SourceCallID: secondTool,
	}
	feedClaudeBackgroundShellEvent(
		t, svc, taskID, sessionID, "execution-successor", secondTool, agentEventComplete,
		claudeBackgroundShellPayload("gh pr checks --watch", true), secondEnd,
	)
	require.True(t, svc.hasBackgroundTask(sessionID, secondTool),
		"a successor execution must not retire predecessor work")

	feedClaudeBackgroundShellEvent(
		t, svc, taskID, sessionID, executionID, secondTool, agentEventComplete,
		claudeBackgroundShellPayload("gh pr checks --watch", true), secondEnd,
	)
	require.False(t, svc.hasBackgroundTask(sessionID, secondTool))

	activity, ok := turnActivityRecord(t, svc, sessionID)
	require.True(t, ok)
	activity.mu.Lock()
	defer activity.mu.Unlock()
	require.Empty(t, activity.background, "no accounting registration may remain")
	require.False(t, activity.yielded, "the final exact exit must clear background activity")
}
