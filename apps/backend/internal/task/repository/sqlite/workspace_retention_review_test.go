package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/stretchr/testify/require"
)

func TestRetainedMalformedInventoryIsTaskScoped(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{ID: "bad", OperationID: "bad", TaskID: "bad-task", ResourceSnapshot: `{"workspace_cleanup":"retain","retention_inventory_captured":true}`}))
	_, err := repo.db.ExecContext(ctx, `UPDATE task_resource_cleanup_jobs SET resource_snapshot = '{' WHERE id = 'bad'`)
	require.NoError(t, err)
	protected, err := repo.HasRetainedTaskWorkspace(ctx, "healthy-task")
	require.NoError(t, err)
	require.False(t, protected)
	_, err = repo.HasRetainedTaskWorkspace(ctx, "bad-task")
	require.Error(t, err)
	_, err = repo.HasRetainedWorkspaceResource(ctx, "", "unknown-container")
	require.Error(t, err)
}

func TestRetainedProjectionTracksSnapshotUpdates(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	job := &models.TaskResourceCleanupJob{ID: "projection", OperationID: "projection", TaskID: "task", State: models.TaskResourceCleanupStatePrepared, ResourceSnapshot: `{}`}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, job))
	var protection string
	require.NoError(t, repo.db.GetContext(ctx, &protection, `SELECT workspace_protection FROM task_resource_cleanup_jobs WHERE id='projection'`))
	require.Empty(t, protection)
	require.NoError(t, repo.UpdateTaskResourceCleanupSnapshot(ctx, job.OperationID, `{"workspace_cleanup":"retain","retention_inventory_captured":true}`))
	require.NoError(t, repo.db.GetContext(ctx, &protection, `SELECT workspace_protection FROM task_resource_cleanup_jobs WHERE id='projection'`))
	require.Equal(t, "retain", protection)
	var indexSQL string
	require.NoError(t, repo.db.GetContext(ctx, &indexSQL, `SELECT sql FROM sqlite_master WHERE name='idx_task_cleanup_workspace_protection'`))
	require.Contains(t, indexSQL, "workspace_protection")
}

// @covers AC-TASKS-DELETE-RECOVERY-002.4
func TestDeleteUnavailableInventoryProtection(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	job := &models.TaskResourceCleanupJob{ID: "cleanup", OperationID: "cleanup", TaskID: "task", State: models.TaskResourceCleanupStateRunning, Attempts: 1, ResourceSnapshot: `{"workspace_cleanup":"delete","cleanup_version":1,"cleanup_inventory_captured":true,"allow_unverified_workspace_changes":true,"discard_worktree_changes":true,"worktrees":[{"id":"wt","path":"/tasks/task/repo"}]}`}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, job))
	protected, err := repo.HasRetainedTaskWorkspace(ctx, "task")
	require.NoError(t, err)
	require.True(t, protected)
	wrong := recoveryclaim.WithTaskCleanupJob(ctx, recoveryclaim.TaskCleanupJob{ID: job.ID, TaskID: job.TaskID, Attempts: 2})
	protected, err = repo.HasRetainedTaskWorkspace(wrong, "task")
	require.NoError(t, err)
	require.True(t, protected)
	owner := recoveryclaim.WithTaskCleanupJob(ctx, recoveryclaim.TaskCleanupJob{ID: job.ID, TaskID: job.TaskID, Attempts: 1})
	protected, err = repo.HasRetainedTaskWorkspace(owner, "task")
	require.NoError(t, err)
	require.False(t, protected)
	require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{ID: "retain", OperationID: "retain", TaskID: "task", ResourceSnapshot: `{"workspace_cleanup":"retain","retention_inventory_captured":true}`}))
	protected, err = repo.HasRetainedTaskWorkspace(owner, "task")
	require.NoError(t, err)
	require.True(t, protected)
}

func TestRetainedProjectionBackfillsOldRows(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	for _, tc := range []struct{ id, payload, protection string }{
		{"normal", `{}`, ""}, {"retain", `{"workspace_cleanup":"retain","retention_inventory_captured":true}`, "retain"}, {"broken", `{`, "unknown"},
	} {
		require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{ID: tc.id, OperationID: tc.id, TaskID: tc.id, ResourceSnapshot: `{}`}))
		_, err := repo.db.ExecContext(ctx, "UPDATE task_resource_cleanup_jobs SET workspace_protection='unclassified',resource_snapshot=? WHERE id=?", tc.payload, tc.id)
		require.NoError(t, err)
	}
	require.NoError(t, repo.migrateWorkspaceProtection(ctx))
	require.NoError(t, repo.migrateWorkspaceProtection(ctx))
	var values []string
	require.NoError(t, repo.db.SelectContext(ctx, &values, "SELECT workspace_protection FROM task_resource_cleanup_jobs ORDER BY id"))
	require.Equal(t, []string{"unknown", "", "retain"}, values)
}
