package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExactProfileInferenceEvidenceRequiresCurrentNonemptyStreamProgress(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)
	revision := exactProfileRecoveryAssignment(t, repo)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	binding := &models.ExactProfileLaunchAttemptBinding{
		TaskID: session.TaskID, SessionID: session.ID, ExecutionID: "exec-current", AttemptID: "attempt-current",
		SessionIncarnationID: session.QueueIncarnationID, AgentProfileID: "profile-exact", Model: "gpt-exact",
		ProfileRevision: revision, Generation: 1,
	}
	changed, err := repo.BindExactProfileLaunchAttempt(ctx, binding)
	require.NoError(t, err)
	require.True(t, changed)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}

	event := func(eventType, text string, attempt *models.ExactProfileLaunchAttemptBinding) {
		svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
			TaskID: binding.TaskID, SessionID: binding.SessionID, ExecutionID: binding.ExecutionID,
			ExactProfileAttempt: attempt,
			Data:                &lifecycle.AgentStreamEventData{Type: eventType, Text: text},
		})
	}
	assertNoExactProfileAttemptReceipt(t, ctx, repo, binding)

	// Delivery/start/terminal frames and empty stream frames are not inference evidence.
	event("session_status", "accepted", binding)
	event("message_streaming", "  \n", binding)
	event(agentEventComplete, "", binding)
	assertNoExactProfileAttemptReceipt(t, ctx, repo, binding)

	stale := *binding
	stale.Model = "gpt-foreign"
	event("message_streaming", "stale progress", &stale)
	assertNoExactProfileAttemptReceipt(t, ctx, repo, binding)

	event("message_streaming", "actual inference progress", binding)
	receipt, err := repo.GetExactProfileLaunchReceipt(ctx, binding.TaskID, binding.SessionID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, models.ExactProfileLaunchOutcomeApplied, receipt.Outcome)
	require.True(t, receipt.InferenceStarted)
	require.Equal(t, binding.Model, receipt.Model)

	// Replayed progress cannot replace the first durable outcome.
	event("thinking_streaming", "later reasoning", binding)
	replayed, err := repo.GetExactProfileLaunchReceipt(ctx, binding.TaskID, binding.SessionID)
	require.NoError(t, err)
	require.Equal(t, receipt.CreatedAt, replayed.CreatedAt)
}

func TestExactProfileInferenceEvidenceAcceptsCorrelatedToolProgress(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)
	revision := exactProfileRecoveryAssignment(t, repo)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	binding := &models.ExactProfileLaunchAttemptBinding{TaskID: session.TaskID, SessionID: session.ID, ExecutionID: "exec-tool", AttemptID: "attempt-tool", SessionIncarnationID: session.QueueIncarnationID, AgentProfileID: "profile-exact", Model: "gpt-exact", ProfileRevision: revision, Generation: 1}
	changed, err := repo.BindExactProfileLaunchAttempt(ctx, binding)
	require.NoError(t, err)
	require.True(t, changed)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}
	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{TaskID: binding.TaskID, SessionID: binding.SessionID, ExecutionID: binding.ExecutionID, ExactProfileAttempt: binding, Data: &lifecycle.AgentStreamEventData{Type: agentEventToolCall, ToolCallID: "tool-progress"}})
	receipt, err := repo.GetExactProfileLaunchReceipt(ctx, binding.TaskID, binding.SessionID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.True(t, receipt.InferenceStarted)
	require.Equal(t, binding.Model, receipt.Model)
}

func assertNoExactProfileAttemptReceipt(t *testing.T, ctx context.Context, repo interface {
	GetExactProfileLaunchReceipt(context.Context, string, string) (*models.ExactProfileLaunchReceipt, error)
}, binding *models.ExactProfileLaunchAttemptBinding) {
	t.Helper()
	receipt, err := repo.GetExactProfileLaunchReceipt(ctx, binding.TaskID, binding.SessionID)
	require.NoError(t, err)
	require.Nil(t, receipt)
}
