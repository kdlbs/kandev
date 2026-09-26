package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestPersistLaunchStateKeepsRunningSessionAndPrepareResult(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:       "session-1",
		TaskID:   "task-1",
		State:    models.TaskSessionStateRunning,
		Metadata: map[string]interface{}{"existing": "preserved"},
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	launchSession := &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateStarting,
	}
	prepareResult := &lifecycle.EnvPrepareResult{Success: true, WorkspacePath: "/workspace/task-1"}

	err := exec.persistLaunchState(
		ctx,
		"task-1",
		"session-1",
		launchSession,
		&LaunchAgentResponse{PrepareResult: prepareResult},
		true,
		time.Now(),
	)
	require.NoError(t, err)

	updated, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, updated.State)
	require.Equal(t, "preserved", updated.Metadata["existing"])
	prepareMetadata, ok := updated.Metadata["prepare_result"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "completed", prepareMetadata["status"])
}
