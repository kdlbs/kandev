package orchestrator

import (
	"context"
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
func TestCancelAgent_ExistingTerminalStepReconcilesReviewState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskID := "task-cancel-existing-terminal"
	sessionID := "session-cancel-existing-terminal"
	seedSession(t, repo, taskID, sessionID, "step3")

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createEngineService(t, repo, cancelCompletionStepGetter(false, false), &mockAgentManager{})
	svc.taskRepo = taskRepo

	require.NoError(t, svc.CancelAgent(ctx, sessionID))
	assert.Contains(t, taskRepo.stateHistory[taskID], v1.TaskStateReview)
}
