package store

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-MCP-APPLY-001.1, AC-AGENTS-MCP-APPLY-002.1, AC-AGENTS-MCP-APPLY-002.2
func TestBuiltinApplicationTransaction(t *testing.T) {
	repo := newBuiltinStore(t)
	ctx := context.Background()
	tasks := tasksqlite.NewWithInitializedDB(repo.db, repo.db, nil)
	for _, state := range models.AllTaskSessionStates {
		require.NoError(t, tasks.CreateTaskSession(ctx, &models.TaskSession{ID: string(state), TaskID: "task", State: state}))
	}
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='task_builtin_mcp_applications'`))
	require.Equal(t, 1, count, "application intents need durable task-owned storage")
	preferences := &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "workspace", TaskID: "task", UpdatedBy: "human", UpdatedAt: time.Now().UTC(), Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}
	_, err := repo.PutBuiltinTaskPreferences(ctx, preferences, 0, 0, p.SurfaceKanbanTask)
	require.NoError(t, err)
	var sessions []string
	require.NoError(t, repo.db.Select(&sessions, `SELECT session_id FROM task_builtin_mcp_applications ORDER BY session_id`))
	require.Equal(t, []string{"RUNNING", "STARTING", "WAITING_FOR_INPUT"}, sessions)
	var actor string
	require.NoError(t, repo.db.Get(&actor, `SELECT requested_by FROM task_builtin_mcp_applications WHERE session_id='RUNNING'`))
	require.Equal(t, "human", actor)
	_, err = repo.PutBuiltinTaskPreferences(ctx, preferences, 0, 0, p.SurfaceKanbanTask)
	require.ErrorIs(t, err, mcpconfig.ErrBuiltinPolicyConflict)
	require.NoError(t, repo.db.Get(&count, `SELECT count(*) FROM task_builtin_mcp_applications WHERE target_revision=1 AND stage='pending'`))
	require.Equal(t, 3, count)
	preferences.Packages[p.PackageDiagnostics] = "on" // Same effective selection, distinct explicit preference.
	_, err = repo.PutBuiltinTaskPreferences(ctx, preferences, 1, 0, p.SurfaceKanbanTask)
	require.NoError(t, err)
	require.NoError(t, repo.db.Get(&count, `SELECT count(*) FROM task_builtin_mcp_applications WHERE target_revision=2 AND attempt_generation=0`))
	require.Equal(t, 3, count, "equivalent saves update target provenance without allocating another attempt")
	preferences.Packages[p.PackageConfiguration] = "off"
	_, err = repo.PutBuiltinTaskPreferences(ctx, preferences, 2, 0, p.SurfaceKanbanTask)
	require.NoError(t, err)
	require.NoError(t, repo.db.Get(&count, `SELECT count(*) FROM task_builtin_mcp_applications WHERE target_revision=3`))
	require.Equal(t, 3, count, "later saves replace the pending target")
}

func TestBuiltinApplicationSaveRollsBackWithIntent(t *testing.T) {
	repo := newBuiltinStore(t)
	ctx := context.Background()
	tasks := tasksqlite.NewWithInitializedDB(repo.db, repo.db, nil)
	require.NoError(t, tasks.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateWaitingForInput}))
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT count(*) FROM sqlite_master WHERE name='task_builtin_mcp_applications'`))
	require.Equal(t, 1, count)
	_, err := repo.db.Exec(`CREATE TRIGGER fail_application BEFORE INSERT ON task_builtin_mcp_applications BEGIN SELECT RAISE(ABORT, 'intent write failed'); END`)
	require.NoError(t, err)
	preferences := &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "workspace", TaskID: "task", UpdatedBy: "human", UpdatedAt: time.Now().UTC(), Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}
	_, err = repo.PutBuiltinTaskPreferences(ctx, preferences, 0, 0, p.SurfaceKanbanTask)
	require.Error(t, err)
	snapshot, err := repo.GetBuiltinPolicySnapshot(ctx, "workspace", "task")
	require.NoError(t, err)
	require.Zero(t, snapshot.Preferences.Revision, "preference save must roll back when its intent cannot persist")
}

func TestBuiltinApplicationEquivalentAndArchivedSavesDoNotRequestRestart(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(map[bool]string{false: "equivalent", true: "archived"}[archived], func(t *testing.T) {
			repo := newBuiltinStore(t)
			ctx := context.Background()
			tasks := tasksqlite.NewWithInitializedDB(repo.db, repo.db, nil)
			require.NoError(t, tasks.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateWaitingForInput}))
			choice := mcpconfig.BuiltinPackageChoices{p.PackageDiagnostics: "on"}
			if archived {
				_, err := repo.db.Exec(`UPDATE tasks SET archived_at = ? WHERE id = 'task'`, time.Now().UTC())
				require.NoError(t, err)
				choice[p.PackageConfiguration] = "on"
			}
			preferences := &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "workspace", TaskID: "task", UpdatedBy: "human", UpdatedAt: time.Now().UTC(), Packages: choice}
			_, err := repo.PutBuiltinTaskPreferences(ctx, preferences, 0, 0, p.SurfaceKanbanTask)
			require.NoError(t, err)
			var count int
			require.NoError(t, repo.db.Get(&count, `SELECT count(*) FROM task_builtin_mcp_applications`))
			require.Zero(t, count)
		})
	}
}

func TestBuiltinApplicationSavePreservesActiveAttempt(t *testing.T) {
	repo := newBuiltinStore(t)
	ctx := context.Background()
	tasks := tasksqlite.NewWithInitializedDB(repo.db, repo.db, nil)
	require.NoError(t, tasks.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateWaitingForInput}))
	preferences := &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "workspace", TaskID: "task", UpdatedBy: "human", UpdatedAt: time.Now().UTC(), Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}
	_, err := repo.PutBuiltinTaskPreferences(ctx, preferences, 0, 0, p.SurfaceKanbanTask)
	require.NoError(t, err)
	_, err = repo.db.Exec(`UPDATE task_builtin_mcp_applications SET stage='starting', attempt_generation=7, attempt_revision=1, source_execution_id='old', replacement_execution_id='new' WHERE session_id='session'`)
	require.NoError(t, err)
	preferences.Packages[p.PackageConfiguration] = "off"
	_, err = repo.PutBuiltinTaskPreferences(ctx, preferences, 1, 0, p.SurfaceKanbanTask)
	require.NoError(t, err)
	status, err := tasks.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	require.EqualValues(t, 2, status.TargetRevision)
	require.EqualValues(t, 1, status.AttemptRevision)
	require.EqualValues(t, 7, status.AttemptGeneration)
	require.Equal(t, "starting", status.Stage)
	require.Equal(t, "new", status.ReplacementExecutionID)
	require.Equal(t, "old", status.SourceExecutionID)
}

func TestBuiltinApplicationSaveReplacesPreviousWorkspaceIntent(t *testing.T) {
	repo := newBuiltinStore(t)
	ctx := context.Background()
	tasks := tasksqlite.NewWithInitializedDB(repo.db, repo.db, nil)
	require.NoError(t, tasks.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateWaitingForInput}))
	preferences := &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "workspace", TaskID: "task", UpdatedBy: "human", UpdatedAt: time.Now().UTC(), Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}
	_, err := repo.PutBuiltinTaskPreferences(ctx, preferences, 0, 0, p.SurfaceKanbanTask)
	require.NoError(t, err)
	_, err = repo.db.Exec(`UPDATE task_builtin_mcp_applications SET workspace_id='previous', stage='starting', replacement_execution_id='previous-execution' WHERE session_id='session'`)
	require.NoError(t, err)
	preferences.Packages[p.PackageConfiguration] = "off"
	_, err = repo.PutBuiltinTaskPreferences(ctx, preferences, 1, 0, p.SurfaceKanbanTask)
	require.NoError(t, err)
	status, err := tasks.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	require.NotNil(t, status, "a new human save must replace intent left by a previous workspace")
	require.Equal(t, "pending", status.Stage)
	require.Empty(t, status.ReplacementExecutionID)
}

func TestBuiltinApplicationEquivalentSaveDoesNotApplyWorkspacePolicy(t *testing.T) {
	repo := newBuiltinStore(t)
	ctx := context.Background()
	tasks := tasksqlite.NewWithInitializedDB(repo.db, repo.db, nil)
	require.NoError(t, tasks.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateWaitingForInput}))
	preference := &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: "workspace", TaskID: "task", UpdatedBy: "human", UpdatedAt: time.Now().UTC(), Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: "on"}}
	_, err := repo.PutBuiltinTaskPreferences(ctx, preference, 0, 0, p.SurfaceKanbanTask)
	require.NoError(t, err)
	_, err = repo.db.Exec(`UPDATE task_builtin_mcp_applications SET stage='applied' WHERE session_id='session'`)
	require.NoError(t, err)
	policy := mcpconfig.DefaultBuiltinWorkspacePolicy("workspace")
	policy.Packages[p.PackageDiagnostics] = mcpconfig.BuiltinPolicyAvailable
	policy.UpdatedBy = "admin"
	policy.UpdatedAt = time.Now().UTC()
	_, err = repo.PutBuiltinWorkspacePolicy(ctx, policy, 0)
	require.NoError(t, err)
	// Equivalent to the saved task preference under the new policy.
	preference.Packages[p.PackageWalkthroughs] = mcpconfig.BuiltinPreferenceOn
	_, err = repo.PutBuiltinTaskPreferences(ctx, preference, 1, 1, p.SurfaceKanbanTask)
	require.NoError(t, err)
	current, err := tasks.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, "applied", current.Stage, "a task save with no effective change cannot apply a separate workspace policy change")
}
