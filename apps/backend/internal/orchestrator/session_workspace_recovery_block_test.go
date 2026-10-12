package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.7
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.10
func TestWorkspaceRecoveryBlockPreservesPromptAdmission(t *testing.T) {
	for _, action := range []string{"", "resume"} {
		t.Run("recovery_action="+action, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
			manager := &mockAgentManager{repoForExecutionLookup: repo}
			service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			session, err := repo.GetTaskSession(ctx, "session1")
			require.NoError(t, err)
			block := &models.SessionRecoveryBlock{
				SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
				Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
			}
			require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, block))
			service.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

			response, err := service.LaunchSession(ctx, &LaunchSessionRequest{
				TaskID: session.TaskID, SessionID: session.ID,
				Intent: IntentRestoreWorkspace, RecoveryAction: action,
			})
			require.NoError(t, err, "prompt uncertainty must not block workspace inspection")
			require.True(t, response.Success)
			require.Equal(t, string(models.TaskSessionStateFailed), response.State)
			stored, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockOpen, stored.State)
			require.Empty(t, manager.startAgentProcessCalls)
			require.Empty(t, manager.capturedPromptCalls)

			_, err = service.LaunchSession(ctx, &LaunchSessionRequest{
				TaskID: session.TaskID, SessionID: session.ID, Intent: IntentResume,
			})
			require.ErrorIs(t, err, ErrSessionRecoveryRequired)
		})
	}
}

func TestWorkspaceRecoveryBlockRejectsPrompt(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	manager := &mockAgentManager{repoForExecutionLookup: repo}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	_, err := service.LaunchSession(context.Background(), &LaunchSessionRequest{
		TaskID: "task1", SessionID: "session1", Intent: IntentRestoreWorkspace, Prompt: "perform work",
	})
	require.Error(t, err, "workspace-only requests must reject a prompt")
	require.Empty(t, manager.capturedPromptCalls)
}
