package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestExecutorMetadataSelectionDeniedBeforeTaskCreation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   string
		value string
		want  error
	}{
		{"foreign profile", models.MetaKeyExecutorProfileID, "creation-private-profile", repoerrors.ErrExecutorProfileNotFound},
		{"foreign executor", models.MetaKeyExecutorID, "creation-private", repoerrors.ErrExecutorProfileNotFound},
		{"missing profile", models.MetaKeyExecutorProfileID, "missing", repoerrors.ErrExecutorProfileNotFound},
		{"missing executor", models.MetaKeyExecutorID, "missing", models.ErrExecutorNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			f := seedRetainedExecutorTask(t, repo, "creation-denied")
			createPrivateExecutor(t, repo, "creation-private")
			ctx := authn.WithIdentity(t.Context(), retainedUser("carol"))

			result, err := svc.CreateTask(ctx, &CreateTaskRequest{
				WorkspaceID: f.workspaceID, WorkflowID: f.workflowID, Title: "New task",
				StartAgent: false, Metadata: map[string]interface{}{tc.key: tc.value},
			})

			require.ErrorIs(t, err, tc.want)
			require.Nil(t, result.Task)
			tasks, err := repo.ListTasks(t.Context(), f.workflowID)
			require.NoError(t, err)
			require.Len(t, tasks, 1)
			require.Equal(t, f.taskID, tasks[0].ID)
		})
	}
}

func TestExecutorMetadataSelectionAllowsEligibleTaskCreation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		user  string
		owner string
	}{
		{"owner", "alice", "alice"},
		{"shared", "carol", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			f := seedRetainedExecutorTask(t, repo, "creation-allowed")
			executor := &models.Executor{ID: "creation-executor", Name: "Executor", Type: models.ExecutorTypeSSH, OwnerUserID: tc.owner}
			require.NoError(t, repo.CreateExecutor(t.Context(), executor))
			profile := &models.ExecutorProfile{ID: "creation-profile", ExecutorID: executor.ID, Name: "Profile", OwnerUserID: tc.owner}
			require.NoError(t, repo.CreateExecutorProfile(t.Context(), profile))
			ctx := authn.WithIdentity(t.Context(), retainedUser(tc.user))

			result, err := svc.CreateTask(ctx, &CreateTaskRequest{
				WorkspaceID: f.workspaceID, WorkflowID: f.workflowID, Title: "New task",
				Metadata: map[string]interface{}{models.MetaKeyExecutorID: executor.ID, models.MetaKeyExecutorProfileID: profile.ID},
			})

			require.NoError(t, err)
			stored, err := repo.GetTask(t.Context(), result.Task.ID)
			require.NoError(t, err)
			require.Equal(t, executor.ID, stored.Metadata[models.MetaKeyExecutorID])
			require.Equal(t, profile.ID, stored.Metadata[models.MetaKeyExecutorProfileID])
		})
	}
}
