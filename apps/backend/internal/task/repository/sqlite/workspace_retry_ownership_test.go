package sqlite_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
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
			t.Run("SQLite", func(t *testing.T) {
				repo, database := newRetryOwnershipRepository(t, false)
				testWorkspaceRetryOwnership(t, repo, database, shared)
			})
			t.Run("Postgres", func(t *testing.T) {
				repo, database := newRetryOwnershipRepository(t, true)
				testWorkspaceRetryOwnership(t, repo, database, shared)
			})
		})
	}
}

func newRetryOwnershipRepository(t *testing.T, postgres bool) (*tasksqlite.Repository, *sqlx.DB) {
	t.Helper()
	var database *sqlx.DB
	if postgres {
		database = testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		database.SetMaxOpenConns(4)
		database.SetMaxIdleConns(4)
	} else {
		connection, err := db.OpenSQLite(filepath.Join(t.TempDir(), "retry-ownership.db"))
		require.NoError(t, err)
		database = sqlx.NewDb(connection, "sqlite3")
		t.Cleanup(func() { _ = database.Close() })
	}
	repo, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	return repo, database
}

func testWorkspaceRetryOwnership(t *testing.T, repo *tasksqlite.Repository, database *sqlx.DB, shared bool) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-retry", Name: "Retry ownership"}))
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
		seedRetryWorkspaceGroup(t, database)
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

func seedRetryWorkspaceGroup(t *testing.T, database *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	office, err := officesqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	require.NoError(t, office.CreateWorkspaceGroup(ctx, &officemodels.WorkspaceGroup{
		ID: "group-retry", WorkspaceID: "ws-retry", OwnerTaskID: "owner", MaterializedKind: officemodels.WorkspaceGroupKindPlainFolder,
	}))
	for _, taskID := range []string{"owner", "follower"} {
		require.NoError(t, office.AddWorkspaceGroupMember(ctx, "group-retry", taskID, ""))
	}
}

func assertRetrySessionOwnership(t *testing.T, repo *tasksqlite.Repository, taskID, envID string, retries []*models.TaskSession, shared bool) {
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
