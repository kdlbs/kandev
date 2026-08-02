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
		Metadata: map[string]interface{}{models.MetaKeyAgentTitlePending: true},
	}
	repo.sessions["session-pending"] = &models.TaskSession{TaskID: "task-pending"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	mode, err := exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-pending"])
	require.NoError(t, err)
	require.Equal(t, McpModeTaskTitlePending, mode)
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

	mode, err := exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-config"])
	require.NoError(t, err)
	require.Equal(t, McpModeConfig, mode)

	mode, err = exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-office"])
	require.NoError(t, err)
	require.Equal(t, McpModeOffice, mode)
}
