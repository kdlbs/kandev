package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

// TestHandleAgentCapabilitiesEvent_RejectsStaleAttempt verifies a capability
// event from an obsolete resume attempt cannot restore a native suggestion
// source or broadcast enablement.
func TestHandleAgentCapabilitiesEvent_RejectsStaleAttempt(t *testing.T) {
	ctx := context.Background()
	svc, eb := seedPromptSuggestionSession(t, models.TaskSessionStateStarting)
	registry := svc.resumeAttemptStore()
	attemptCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	attempt := &resumeAttempt{
		id: 7, taskID: "t1", sessionID: "s1", ctx: attemptCtx, cancel: cancel, done: make(chan struct{}),
	}
	registry.mu.Lock()
	registry.attempts["s1"] = attempt
	registry.mu.Unlock()
	t.Cleanup(func() { attempt.finish(registry) })

	payload := func(executionID, attemptID string) *lifecycle.AgentStreamEventPayload {
		enabled := true
		return &lifecycle.AgentStreamEventPayload{
			TaskID:      "t1",
			SessionID:   "s1",
			ExecutionID: executionID,
			AttemptID:   attemptID,
			Data: &lifecycle.AgentStreamEventData{
				SupportsPromptSuggestions:        enabled,
				NativePromptSuggestionsSupported: &enabled,
			},
		}
	}
	requireNoSource := func() {
		t.Helper()
		session, err := svc.repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		require.NotContains(t, session.Metadata, promptSuggestionSourceMetadataKey)
	}

	svc.handleAgentCapabilitiesEvent(ctx, payload("execution-1", "resume-999"))
	requireNoSource()
	require.Empty(t, eb.events)

	// The current attempt binds the first admitted execution; a different one is
	// stale.
	svc.handleAgentCapabilitiesEvent(ctx, payload("execution-1", attempt.identity()))
	session, err := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, promptSuggestionSourceNative, session.Metadata[promptSuggestionSourceMetadataKey])
	require.Len(t, eb.events, 1)

	svc.handleAgentCapabilitiesEvent(ctx, payload("execution-2", attempt.identity()))
	require.Len(t, eb.events, 1)
}
