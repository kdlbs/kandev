package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCreateTaskRequiredAssignmentsAreServerOwned(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-requirements", Name: "Office"}))
	for _, count := range []int{0, 3} {
		result, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-requirements", Title: "Routine requirement", IsEphemeral: true,
			RequiredSubtaskCount: count,
			Metadata:             map[string]interface{}{models.MetaKeyRequiredSubtaskCount: 99},
		})
		require.NoError(t, err)
		got, err := svc.GetTask(ctx, result.Task.ID)
		require.NoError(t, err)
		if count == 0 {
			require.NotContains(t, got.Metadata, models.MetaKeyRequiredSubtaskCount)
		} else {
			require.EqualValues(t, count, got.Metadata[models.MetaKeyRequiredSubtaskCount])
		}
	}
}
