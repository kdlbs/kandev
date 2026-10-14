package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4
func TestKubernetesSharedGroupBindingAdmission(t *testing.T) {
	cases := []struct {
		owner, follower models.ExecutorType
		want            error
	}{
		{models.ExecutorTypeKubernetes, models.ExecutorTypeKubernetes, models.ErrWorkspaceReuseUnsupported},
		{models.ExecutorTypeLocal, models.ExecutorTypeLocal, nil},
		{models.ExecutorTypeKubernetes, models.ExecutorTypeLocal, models.ErrWorkspaceReuseUnsafe},
		{models.ExecutorTypeLocal, models.ExecutorTypeKubernetes, models.ErrWorkspaceReuseUnsafe},
	}
	for _, dialect := range []string{"SQLite", "Postgres"} {
		t.Run(dialect, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(string(tc.owner)+"-"+string(tc.follower), func(t *testing.T) {
					repo, database := newRetryOwnershipRepository(t, dialect == "Postgres")
					assertSharedGroupBindingAdmission(t, repo, database, tc.owner, tc.follower, tc.want)
				})
			}
		})
	}
}

func assertSharedGroupBindingAdmission(t *testing.T, repo *tasksqlite.Repository, database *sqlx.DB, ownerType, followerType models.ExecutorType, want error) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-retry", Name: "Group admission"}))
	for _, taskID := range []string{"owner", "follower"} {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-retry", Title: taskID}))
	}
	seedRetryWorkspaceGroup(t, database)
	bind := func(session *models.TaskSession, executorType models.ExecutorType) error {
		return repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, session,
			&models.TaskEnvironment{TaskID: session.TaskID, ExecutorType: string(executorType)}, "group-retry")
	}
	// Preflight saw an unmaterialized group. The owner becomes ready before
	// the follower enters the binding transaction.
	owner := &models.TaskSession{ID: "owner-session", TaskID: "owner"}
	require.NoError(t, bind(owner, ownerType))
	env, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
	require.NoError(t, err)
	env.Status = models.TaskEnvironmentStatusReady
	env.MaterializationSessionID = ""
	require.NoError(t, repo.UpdateTaskEnvironment(ctx, env))
	env, err = repo.GetTaskEnvironment(ctx, env.ID)
	require.NoError(t, err)
	follower := &models.TaskSession{ID: "follower-session", TaskID: "follower"}
	err = bind(follower, followerType)
	if want != nil {
		require.ErrorIs(t, err, want)
		sessions, listErr := repo.ListTaskSessions(ctx, follower.TaskID)
		require.NoError(t, listErr)
		require.Empty(t, sessions)
		require.Empty(t, follower.TaskEnvironmentID)
	} else {
		require.NoError(t, err)
		require.Equal(t, env.ID, follower.TaskEnvironmentID)
	}
	// Independent sessions of the same owner retain supported reuse.
	sibling := &models.TaskSession{ID: "owner-sibling", TaskID: "owner"}
	require.NoError(t, bind(sibling, ownerType))
	require.Equal(t, env.ID, sibling.TaskEnvironmentID)
	stored, err := repo.GetTaskEnvironment(ctx, env.ID)
	require.NoError(t, err)
	require.Equal(t, env, stored)
}
