package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSandboxAuthorityCreationBeforeTaskInsert(t *testing.T) {
	svc, _, repo := createTestService(t)
	require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "workspace", Name: "Workspace"}))
	denied := errors.New("sandbox admission refused")
	var captured string
	svc.SetSandboxCreationAdmission(func(ctx context.Context, task *models.Task, req *CreateTaskRequest) error {
		require.Equal(t, "workspace", task.WorkspaceID)
		require.Equal(t, req.Title, task.Title)
		captured = task.ID
		return denied
	})
	_, err := svc.CreateTask(t.Context(), &CreateTaskRequest{WorkspaceID: "workspace", Title: "Restricted", IsEphemeral: true})
	require.ErrorIs(t, err, denied)
	require.NotEmpty(t, captured)
	task, err := repo.GetTask(t.Context(), captured)
	require.True(t, task == nil || err != nil, "rejected task was persisted")
}
