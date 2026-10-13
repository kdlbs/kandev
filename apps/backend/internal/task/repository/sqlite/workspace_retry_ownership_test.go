package sqlite

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4
func TestWorkspaceBindingRetryOwnership(t *testing.T) {
	for _, shared := range []bool{false, true} {
		name := "task-local"
		if shared {
			name = "shared-group"
		}
		t.Run(name, func(t *testing.T) {
			t.Run("SQLite", func(t *testing.T) { testWorkspaceRetryOwnership(t, newRepoForEntityTests(t), shared) })
			t.Run("Postgres", func(t *testing.T) {
				db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
				repo, err := NewWithDB(db, db, nil)
				require.NoError(t, err)
				testWorkspaceRetryOwnership(t, repo, shared)
			})
		})
	}
}

func testWorkspaceRetryOwnership(t *testing.T, repo *Repository, shared bool) {
	t.Helper()
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-retry")
	seed := models.SessionRuntimeConfig{Model: "initial-model"}
	for _, taskID := range []string{"owner", "follower"} {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-retry", Title: taskID,
			Metadata: map[string]interface{}{
				models.MetaKeyInitialSessionRuntimeConfig:          seed,
				models.MetaKeyInitialSessionRuntimeConfigProfileID: "profile-1",
			},
		}))
	}
	if shared {
		seedRetryWorkspaceGroup(t, repo)
	}
	bind := func(session *models.TaskSession) error {
		candidate := &models.TaskEnvironment{TaskID: session.TaskID, ExecutorType: string(models.ExecutorTypeLocal)}
		if shared {
			return repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, session, candidate, "group-retry")
		}
		return repo.CreateTaskSessionWithWorkspaceBinding(ctx, session, candidate)
	}
	owner := &models.TaskSession{ID: "materializer", TaskID: "owner", AgentProfileID: "profile-1", IsPrimary: true}
	require.NoError(t, bind(owner))
	taskID := "owner"
	if shared {
		taskID = "follower"
	}
	sessions := []*models.TaskSession{
		{ID: "retry-a", TaskID: taskID, AgentProfileID: "profile-1", IsPrimary: true},
		{ID: "retry-b", TaskID: taskID, AgentProfileID: "profile-1", IsPrimary: true},
	}
	for _, session := range sessions {
		session.Metadata = map[string]interface{}{models.SessionMetaKeyOrigin: models.SessionOriginTaskInitial}
		err := bind(session)
		require.ErrorIs(t, err, models.ErrWorkspacePreparing)
		var refusal *models.WorkspacePreparingError
		require.ErrorAs(t, err, &refusal)
		require.Equal(t, owner.TaskEnvironmentID, refusal.EnvironmentID)
	}
	env, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
	require.NoError(t, err)
	env.Status = models.TaskEnvironmentStatusReady
	env.MaterializationSessionID = ""
	require.NoError(t, repo.UpdateTaskEnvironment(ctx, env))
	start := make(chan struct{})
	errs := make([]error, len(sessions))
	var wg sync.WaitGroup
	for i, session := range sessions {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errs[i] = bind(session) }()
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	assertRetrySessionOwnership(t, repo, taskID, owner.TaskEnvironmentID, sessions, shared)
}

func seedRetryWorkspaceGroup(t *testing.T, repo *Repository) {
	t.Helper()
	_, err := repo.db.Exec(`
		CREATE TABLE task_workspace_groups (
			id TEXT PRIMARY KEY, materialized_environment_id TEXT NOT NULL DEFAULT '', updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE task_workspace_group_members (
			workspace_group_id TEXT NOT NULL, task_id TEXT NOT NULL, released_at TIMESTAMP NULL
		);
		INSERT INTO task_workspace_groups (id, updated_at) VALUES ('group-retry', CURRENT_TIMESTAMP);
		INSERT INTO task_workspace_group_members (workspace_group_id, task_id) VALUES
			('group-retry', 'owner'), ('group-retry', 'follower');
	`)
	require.NoError(t, err)
}

func assertRetrySessionOwnership(t *testing.T, repo *Repository, taskID, envID string, retries []*models.TaskSession, shared bool) {
	t.Helper()
	stored, err := repo.ListTaskSessions(context.Background(), taskID)
	require.NoError(t, err)
	primaryCount, originCount, configCount := 0, 0, 0
	for _, session := range stored {
		require.Equal(t, envID, session.TaskEnvironmentID)
		if session.IsPrimary {
			primaryCount++
		}
		if models.IsOriginalTaskSession(session.Metadata) {
			originCount++
		}
		if config, ok := models.LoadSessionRuntimeConfigOverrides(session.Metadata); ok {
			configCount++
			require.Equal(t, "initial-model", config.Model)
		}
	}
	require.Equal(t, 1, primaryCount)
	require.Equal(t, 1, originCount)
	require.Equal(t, 1, configCount)
	for _, retry := range retries {
		committed, err := repo.GetTaskSession(context.Background(), retry.ID)
		require.NoError(t, err)
		require.Equal(t, committed.IsPrimary, retry.IsPrimary, "the caller must receive committed ownership")
		if !shared {
			require.False(t, retry.IsPrimary)
		}
	}
}
