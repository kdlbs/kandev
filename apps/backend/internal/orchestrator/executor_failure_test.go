package orchestrator

import (
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutorFailurePromptErrorDoesNotCompleteOrRevert(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)
	err := svc.handlePromptError(t.Context(), "task1", "session1", models.TaskSessionStateWaitingForInput, lifecycle.ErrExecutorInterrupted)
	require.ErrorIs(t, err, lifecycle.ErrExecutorInterrupted)
	session, err := repo.GetTaskSession(t.Context(), "session1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State, "executor recovery owns settlement; generic error flow cannot complete the turn")
}

func TestExecutorFailureProviderOutcomeReachesTranscriptThroughEvent(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "current", TaskID: "task1", SessionID: "session1", AgentExecutionID: "current", Status: "ready"}))
	svc.handleACPSessionCreated(t.Context(), watcher.ACPSessionEventData{TaskID: "task1", SessionID: "session1", AgentExecutionID: "current", ACPSessionID: "fresh-provider", ConversationOutcome: "fresh"})
	messages, err := repo.ListMessages(t.Context(), "session1")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "fresh", messages[0].Metadata["provider_conversation"])
	svc.handleACPSessionCreated(t.Context(), watcher.ACPSessionEventData{TaskID: "task1", SessionID: "session1", AgentExecutionID: "previous", ACPSessionID: "old-provider", ConversationOutcome: "restored"})
	messages, err = repo.ListMessages(t.Context(), "session1")
	require.NoError(t, err)
	require.Len(t, messages, 1, "a delayed old event cannot add a false restoration notice")
}
