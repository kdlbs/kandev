package handlers

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAdoptionReuseInvalidatesCachedBindingAndRetiredMarker(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	ctx := context.Background()
	original, err := f.manager.GetBySessionAndRepo(ctx, "session-adoption", "repo-adoption", "feature-adoption")
	require.NoError(t, err)
	require.Equal(t, f.old, original.Path)
	p := adoptionPreview(t, f)
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	adopted, err := f.manager.GetBySessionAndRepo(ctx, "session-adoption", "repo-adoption", "feature-adoption")
	require.NoError(t, err)
	require.Equal(t, f.clone, adopted.Path)
	require.Equal(t, original.ID, adopted.ID)
	// Existing store and manager reuse must retain the exact logical worktree.
	require.NoError(t, f.manager.AdmitTaskRecovery(ctx, "task-adoption"))
	require.NoError(t, os.WriteFile(filepath.Join(f.old, ".git"), []byte("gitdir: /foreign/missing\n"), 0600))
	require.Error(t, f.manager.AdmitTaskRecovery(ctx, "task-adoption"))
}

func TestWorkspaceAdoptionSelectedRecoveryRefusesDamagedAdoptedMetadata(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	ctx := context.Background()
	p := adoptionPreview(t, f)
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, os.Rename(filepath.Join(f.clone, ".git"), filepath.Join(f.clone, ".git-preserved")))
	pointer, err := os.ReadFile(filepath.Join(f.old, ".git"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(f.clone, ".git"), pointer, 0600))
	repo := tasksqlite.NewWithInitializedDB(f.db, f.db, nil)
	session, err := repo.GetTaskSession(ctx, "session-adoption")
	require.NoError(t, err)
	env, err := repo.GetTaskEnvironment(ctx, "env-adoption")
	require.NoError(t, err)
	entity, err := repo.GetRepository(ctx, "repo-adoption")
	require.NoError(t, err)
	snapshot := models.NewWorkspaceRecoverySelectionSnapshot(session, env, map[string]*models.Repository{entity.ID: entity})
	all, err := f.manager.GetAllByTaskID(ctx, "task-adoption")
	require.NoError(t, err)
	slots := make([]worktree.RecoverySlot, 0, len(all))
	for _, wt := range all {
		slots = append(slots, worktree.RecoverySlot{Worktree: wt, WorktreeID: wt.ID, RepositoryID: wt.RepositoryID, BranchSlug: wt.BranchSlug, RepositoryPath: f.source})
	}
	admission, err := f.manager.AdmitRecovery(ctx, worktree.RecoveryAdmissionRequest{TaskID: "task-adoption", SessionID: "session-adoption", TaskEnvironmentID: "env-adoption", OwnerTaskID: "task-adoption", OwnershipGeneration: 1, ExecutorType: "worktree", SelectionSnapshot: snapshot, Slots: slots})
	if admission != nil {
		require.NoError(t, admission.Release(ctx))
	}
	require.ErrorIs(t, err, models.ErrWorkspaceAdoptionStale)
	var path string
	require.NoError(t, f.db.Get(&path, `SELECT worktree_path FROM task_environment_repos WHERE id='slot-adoption'`))
	require.Equal(t, f.clone, path)
}
