package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPromptTaskWithoutExecutorLeavesSessionUnchanged(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-runtime", "session-runtime", models.TaskSessionStateWaitingForInput)
	manager := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	result, err := svc.PromptTask(ctx, "task-runtime", "session-runtime", "hello", "", false, nil, false)
	require.ErrorContains(t, err, "executor is not configured")
	require.Nil(t, result)
	session, err := repo.GetTaskSession(ctx, "session-runtime")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	require.Empty(t, manager.capturedPrompts)
}
