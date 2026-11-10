package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator"
)

func captureTaskCreateDispatch(t *testing.T, description string, dispatch *startAgentDispatch) *orchestrator.LaunchSessionRequest {
	t.Helper()
	called := make(chan struct{}, 1)
	orch := &captureOrchestrator{
		startCreatedCalled:   called,
		startCreatedResponse: &orchestrator.LaunchSessionResponse{Success: true, SessionID: "session-1"},
	}
	h := &TaskHandlers{orchestrator: orch, logger: newTestLogger(t)}
	h.dispatchTaskSession(
		context.Background(), "task-1", description,
		httpCreateTaskRequest{AgentProfileID: "profile-1"}, dispatch,
	)
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("async IntentStartCreated launch did not complete")
	}

	orch.mu.Lock()
	defer orch.mu.Unlock()
	require.Len(t, orch.requests, 1)
	return orch.requests[0]
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.16
func TestTaskCreateDispatch_MarksDescriptionPrompt(t *testing.T) {
	request := captureTaskCreateDispatch(t, "task description", &startAgentDispatch{sessionID: "session-1"})
	assert.True(t, request.PromptIsTaskDescription,
		"ordinary task creation transports the description, not an explicit launch brief")
	assert.False(t, request.QuickChatInitialStart)
	assert.Equal(t, "task description", request.Prompt)
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.16
func TestTaskCreateDispatch_QuickChatPromptIsNotADescription(t *testing.T) {
	request := captureTaskCreateDispatch(t, "explicit quick chat opening", &startAgentDispatch{
		sessionID: "session-1", quickChatInitialStart: true,
	})
	assert.False(t, request.PromptIsTaskDescription,
		"the Quick Chat opening prompt is an explicit user instruction")
	assert.True(t, request.QuickChatInitialStart)
	assert.Equal(t, "explicit quick chat opening", request.Prompt)
}
