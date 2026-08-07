package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestResolveTaskSessionMCPMode_TitlePendingIsTaskModeVariant(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.tasks["task-pending"] = &models.Task{
		ID:       "task-pending",
		Metadata: map[string]interface{}{models.MetaKeyAgentTitlePending: true, models.MetaKeyAgentTitleOwnerSessionID: "session-pending"},
	}
	repo.sessions["session-pending"] = &models.TaskSession{ID: "session-pending", TaskID: "task-pending"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	mode, err := exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-pending"], true)
	require.NoError(t, err)
	require.Equal(t, McpModeTaskTitlePending, mode)

	repo.sessions["session-other"] = &models.TaskSession{ID: "session-other", TaskID: "task-pending"}
	mode, err = exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-other"], true)
	require.NoError(t, err)
	require.Empty(t, mode)

	mode, err = exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-pending"], false)
	require.NoError(t, err)
	require.Empty(t, mode)
}

func TestResolveTaskSessionMCPMode_TitlePendingDoesNotOverrideRestrictedModes(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.tasks["task-pending"] = &models.Task{
		ID:           "task-pending",
		IsFromOffice: true,
		Metadata:     map[string]interface{}{models.MetaKeyAgentTitlePending: true},
	}
	repo.sessions["session-config"] = &models.TaskSession{
		TaskID:   "task-pending",
		Metadata: map[string]interface{}{"config_mode": true},
	}
	repo.sessions["session-office"] = &models.TaskSession{TaskID: "task-pending"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	mode, err := exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-config"], true)
	require.NoError(t, err)
	require.Equal(t, McpModeConfig, mode)

	mode, err = exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-office"], true)
	require.NoError(t, err)
	require.Equal(t, McpModeOffice, mode)
}
