package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

func adoptedCleanupFixture(t *testing.T) (adoptionHTTPFixture, *worktree.Worktree, context.Context) {
	t.Helper()
	f := newAdoptionHTTPFixture(t)
	p := adoptionPreview(t, f)
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	wt, err := f.manager.GetByID(context.Background(), "wt-adoption")
	require.NoError(t, err)
	adoptions, err := f.manager.CaptureWorkspaceAdoptionsForCleanup(context.Background(), []*worktree.Worktree{wt})
	require.NoError(t, err)
	require.Len(t, adoptions, 1)
	encoded, err := json.Marshal(map[string]any{"worktrees": []*worktree.Worktree{wt}, "worktree_adoptions": adoptions})
	require.NoError(t, err)
	_, err = f.db.Exec(f.db.Rebind(`INSERT INTO task_resource_cleanup_jobs(id,operation_id,task_id,trigger,state,resource_snapshot,attempts,created_at,updated_at) VALUES('cleanup-adoption','cleanup-adoption','task-adoption','delete','running',?,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`), string(encoded))
	require.NoError(t, err)
	_, err = f.db.Exec(`DELETE FROM tasks WHERE id='task-adoption'`)
	require.NoError(t, err)
	ctx := recoveryclaim.WithTaskCleanupJob(context.Background(), recoveryclaim.TaskCleanupJob{ID: "cleanup-adoption", TaskID: "task-adoption", Attempts: 1})
	return f, wt, ctx
}

func TestWorkspaceAdoptionArchiveAndDeleteRetainsProjectPrimary(t *testing.T) {
	f, wt, ctx := adoptedCleanupFixture(t)
	store, err := worktree.NewSQLiteStore(f.db, f.db)
	require.NoError(t, err)
	f.manager, err = worktree.NewManager(worktree.Config{Enabled: true, TasksBasePath: filepath.Dir(f.root)}, store, newTestLogger(t))
	require.NoError(t, err)
	projectHead := adoptionGit(t, f.source, "rev-parse", "HEAD")
	projectBranch := adoptionGit(t, f.source, "rev-parse", "feature/adoption")
	manifests, err := f.manager.CaptureArchiveSourceManifests(ctx, []*worktree.Worktree{wt})
	require.NoError(t, err)
	require.True(t, manifests[wt.ID].PathPresent)
	require.NotEmpty(t, manifests[wt.ID].IndexStateSHA256)
	require.NotEmpty(t, manifests[wt.ID].Entries)
	require.NoError(t, f.manager.CleanupWorktreesWithOptions(ctx, []*worktree.Worktree{wt}, worktree.WorktreeCleanupOptions{DiscardWorktreeChanges: true}))
	require.NoError(t, f.manager.CleanupWorktreesWithOptions(ctx, []*worktree.Worktree{wt}, worktree.WorktreeCleanupOptions{DiscardWorktreeChanges: true}))
	_, err = os.Stat(f.clone)
	require.True(t, os.IsNotExist(err))
	require.Equal(t, projectHead, adoptionGit(t, f.source, "rev-parse", "HEAD"))
	require.Equal(t, projectBranch, adoptionGit(t, f.source, "rev-parse", "feature/adoption"))
	_, err = os.Stat(filepath.Join(f.old, ".git"))
	require.NoError(t, err)
}

func TestWorkspaceAdoptionArchiveCaptureDoesNotAdmitArchivedRuntime(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	p := adoptionPreview(t, f)
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	wt, err := f.manager.GetByID(context.Background(), "wt-adoption")
	require.NoError(t, err)
	_, err = f.db.Exec(`UPDATE tasks SET archived_at=CURRENT_TIMESTAMP WHERE id='task-adoption'`)
	require.NoError(t, err)
	_, err = f.manager.ValidateWorkspaceAdoptionForReuse(context.Background(), worktree.CreateRequest{TaskEnvironmentID: wt.TaskEnvironmentID, WorktreeID: wt.ID, RepositoryID: wt.RepositoryID, BranchIdentitySlug: wt.BranchSlug}, wt.Path)
	require.Error(t, err)
	proofs, err := f.manager.CaptureWorkspaceAdoptionsForCleanup(context.Background(), []*worktree.Worktree{wt})
	require.NoError(t, err)
	require.Len(t, proofs, 1)
	_, err = f.db.Exec(`UPDATE task_environment_repos SET branch_slug='changed' WHERE worktree_id=?`, wt.ID)
	require.NoError(t, err)
	_, err = f.manager.CaptureWorkspaceAdoptionsForCleanup(context.Background(), []*worktree.Worktree{wt})
	require.Error(t, err)
}

func TestWorkspaceAdoptionArchiveRejectsUnprovenCleanupPaths(t *testing.T) {
	for _, mutation := range []string{"project-primary", "foreign-task", "foreign-job", "stale-attempt", "replaced-checkout", "symlink-metadata", "missing-provenance"} {
		t.Run(mutation, func(t *testing.T) {
			f, wt, ctx := adoptedCleanupFixture(t)
			switch mutation {
			case "project-primary":
				wt.Path, wt.SourceClonePath, wt.SourceCommonDir = f.source, f.source, filepath.Join(f.source, ".git")
			case "foreign-task":
				wt.TaskID = "foreign-task"
			case "foreign-job":
				ctx = recoveryclaim.WithTaskCleanupJob(ctx, recoveryclaim.TaskCleanupJob{ID: "cleanup-adoption", TaskID: "foreign-task", Attempts: 1})
			case "stale-attempt":
				ctx = recoveryclaim.WithTaskCleanupJob(ctx, recoveryclaim.TaskCleanupJob{ID: "cleanup-adoption", TaskID: "task-adoption", Attempts: 2})
			case "replaced-checkout":
				require.NoError(t, os.Rename(f.clone, f.clone+"-retained"))
				adoptionGit(t, f.source, "clone", "--no-local", f.source, f.clone)
			case "symlink-metadata":
				require.NoError(t, os.Rename(filepath.Join(f.clone, ".git"), f.clone+"-git"))
				require.NoError(t, os.Symlink(f.clone+"-git", filepath.Join(f.clone, ".git")))
			case "missing-provenance":
				_, err := f.db.Exec(`UPDATE task_resource_cleanup_jobs SET resource_snapshot='{}' WHERE id='cleanup-adoption'`)
				require.NoError(t, err)
			}
			_, err := f.manager.CaptureArchiveSourceManifests(ctx, []*worktree.Worktree{wt})
			require.Error(t, err)
			require.Error(t, f.manager.CleanupWorktreesWithOptions(ctx, []*worktree.Worktree{wt}, worktree.WorktreeCleanupOptions{DiscardWorktreeChanges: true}))
			_, err = os.Stat(f.source)
			require.NoError(t, err)
			_, err = os.Stat(f.clone)
			require.NoError(t, err)
		})
	}
}
