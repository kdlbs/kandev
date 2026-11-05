package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDeleteSessionCleansSandboxStateAfterStopBeforeRowDeletion(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleanup", true: "uncertain ownership"}[fail], func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "cleanup-task", "cleanup-session", models.TaskSessionStateWaitingForInput)
			manager := &mockAgentManager{getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "execution", nil }}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
			blocked := errors.New("ownership unavailable")
			calls := 0
			svc.SetSessionSandboxCleanup(func(ctx context.Context, taskID, sessionID string) error {
				calls++
				require.Equal(t, "cleanup-task", taskID)
				require.Equal(t, "cleanup-session", sessionID)
				manager.mu.Lock()
				stops := len(manager.stopAgentWithReasonArgs)
				manager.mu.Unlock()
				require.Equal(t, 1, stops)
				_, err := repo.GetTaskSession(ctx, sessionID)
				require.NoError(t, err, "ownership cleanup must precede row deletion")
				if fail {
					return blocked
				}
				return nil
			})
			err := svc.DeleteSession(t.Context(), "cleanup-session")
			require.Equal(t, 1, calls)
			_, readErr := repo.GetTaskSession(t.Context(), "cleanup-session")
			if fail {
				require.ErrorIs(t, err, blocked)
				require.NoError(t, readErr)
			} else {
				require.NoError(t, err)
				require.Error(t, readErr)
			}
		})
	}
}
