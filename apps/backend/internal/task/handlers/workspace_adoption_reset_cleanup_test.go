package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAdoptionPreparedResetUsesCurrentBinding(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	p := adoptionPreview(t, f)
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	repo, err := tasksqlite.NewWithDB(f.db, f.db, nil)
	require.NoError(t, err)
	var generation int64
	require.NoError(t, f.db.Get(&generation, `SELECT ownership_generation FROM task_environments WHERE id='env-adoption'`))
	job, err := repo.ClaimTaskEnvironmentReset(context.Background(), "task-adoption", "env-adoption", generation, "reset-adoption")
	require.NoError(t, err)
	ctx := recoveryclaim.WithTaskCleanupJob(context.Background(), recoveryclaim.TaskCleanupJob{ID: job, TaskID: "task-adoption"})
	wt, err := f.manager.GetByID(context.Background(), "wt-adoption")
	require.NoError(t, err)
	_, err = f.manager.CaptureArchiveSourceManifests(ctx, []*worktree.Worktree{wt})
	require.NoError(t, err)
	foreign := recoveryclaim.WithTaskCleanupJob(ctx, recoveryclaim.TaskCleanupJob{ID: job, TaskID: "foreign"})
	_, err = f.manager.CaptureArchiveSourceManifests(foreign, []*worktree.Worktree{wt})
	require.Error(t, err)
	negative := recoveryclaim.WithTaskCleanupJob(ctx, recoveryclaim.TaskCleanupJob{ID: job, TaskID: "task-adoption", Attempts: -1})
	_, err = f.manager.CaptureArchiveSourceManifests(negative, []*worktree.Worktree{wt})
	require.Error(t, err)
	require.NoError(t, repo.ReleaseTaskEnvironmentReset(context.Background(), job, models.TaskResourceCleanupStateSucceeded, ""))
	_, err = f.manager.CaptureArchiveSourceManifests(ctx, []*worktree.Worktree{wt})
	require.Error(t, err)
}
