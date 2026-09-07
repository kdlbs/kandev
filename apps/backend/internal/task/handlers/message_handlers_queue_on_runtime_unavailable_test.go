package handlers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
)

// TestForwardMessageAsPrompt_QueuesMessageDroppedDuringProfileSwitchWindow is
// the regression test for the message-drop card: a message sent while a
// workflow step move is promoting a new primary session to a different agent
// profile used to be silently discarded. PromptTask fails with exactly the
// production shape — ensureSessionRunning's "no executor record" error
// wrapped in orchestrator.ErrSessionRuntimeUnavailable, matching neither of
// handlePromptWithResume's retry sentinels — so the message must be queued
// against the session instead of reported as failed.
func TestForwardMessageAsPrompt_QueuesMessageDroppedDuringProfileSwitchWindow(t *testing.T) {
	notResumableErr := errors.New("session is not resumable: no executor record (state: WAITING_FOR_INPUT)")
	promptErr := fmt.Errorf(
		"%w: failed to ensure session is running: %w", orchestrator.ErrSessionRuntimeUnavailable, notResumableErr,
	)

	repo := &resumeRetryRepo{
		sessionStateSequencer: sessionStateSequencer{
			states: []models.TaskSessionState{models.TaskSessionStateWaitingForInput},
		},
	}
	orch := &resumeRetryOrchestrator{promptErr: promptErr}
	h := newTestMessageHandlersWithOrchestrator(t, repo, orch)

	h.forwardMessageAsPrompt(
		context.Background(), "task-1", "session-1", "profile-1", "continue",
		"", false, nil, nil, false, "",
	)

	assert.Equal(t, 1, orch.promptCalls, "PromptTask must not be retried: neither retry sentinel matches this error class")
	assert.Equal(t, 0, orch.resumeCalls)
	assert.Equal(t, 1, orch.queuePromptCalls, "the message must be queued for delivery once the runtime comes up")
	assert.Empty(t, repo.createdMessages, "a queued message must not also surface as a dropped-message error")
}

// TestForwardMessageAsPrompt_RuntimeUnavailableTerminalSessionSurfacesError
// ensures the queue fallback does not apply once the session has reached a
// terminal state: there will be no future runtime boot to drain the queue
// against, so the message must surface as an error like today.
func TestForwardMessageAsPrompt_RuntimeUnavailableTerminalSessionSurfacesError(t *testing.T) {
	notResumableErr := errors.New("session is not resumable: no executor record (state: FAILED)")
	promptErr := fmt.Errorf(
		"%w: failed to ensure session is running: %w", orchestrator.ErrSessionRuntimeUnavailable, notResumableErr,
	)

	repo := &resumeRetryRepo{
		sessionStateSequencer: sessionStateSequencer{
			states: []models.TaskSessionState{models.TaskSessionStateFailed},
		},
	}
	orch := &resumeRetryOrchestrator{promptErr: promptErr}
	h := newTestMessageHandlersWithOrchestrator(t, repo, orch)

	h.forwardMessageAsPrompt(
		context.Background(), "task-1", "session-1", "profile-1", "continue",
		"", false, nil, nil, false, "",
	)

	assert.Equal(t, 0, orch.queuePromptCalls, "a terminal session must not be queued")
	require.Len(t, repo.createdMessages, 1, "a terminal session must still surface the failure")
	assert.Contains(t, repo.createdMessages[0].Content, "Failed to send message to agent")
}

// TestForwardMessageAsPrompt_RuntimeUnavailableQueueFailureSurfacesError
// covers QueueUserPrompt itself failing (e.g. the orchestrator has no
// message queue configured): the original prompt error must still surface,
// since the message was neither delivered nor queued.
func TestForwardMessageAsPrompt_RuntimeUnavailableQueueFailureSurfacesError(t *testing.T) {
	notResumableErr := errors.New("session is not resumable: no executor record (state: WAITING_FOR_INPUT)")
	promptErr := fmt.Errorf(
		"%w: failed to ensure session is running: %w", orchestrator.ErrSessionRuntimeUnavailable, notResumableErr,
	)

	repo := &resumeRetryRepo{
		sessionStateSequencer: sessionStateSequencer{
			states: []models.TaskSessionState{models.TaskSessionStateWaitingForInput},
		},
	}
	orch := &resumeRetryOrchestrator{
		promptErr:      promptErr,
		queuePromptErr: errors.New("message queue is not configured"),
	}
	h := newTestMessageHandlersWithOrchestrator(t, repo, orch)

	h.forwardMessageAsPrompt(
		context.Background(), "task-1", "session-1", "profile-1", "continue",
		"", false, nil, nil, false, "",
	)

	assert.Equal(t, 1, orch.queuePromptCalls, "the queue attempt must still be made")
	require.Len(t, repo.createdMessages, 1, "a failed queue attempt must fall back to surfacing the error")
	assert.Contains(t, repo.createdMessages[0].Content, "Failed to send message to agent")
}
