package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

func unavailableDeleteOptions(t *testing.T) DeleteTaskOptions {
	t.Helper()
	return DeleteTaskOptions{DiscardWorktreeChanges: true, AllowUnverifiedWorkspaceChanges: true}
}

// @covers AC-TASKS-DELETE-RECOVERY-002.1
// @covers AC-TASKS-DELETE-RECOVERY-002.7
func TestDeleteUnavailablePreflightConsent(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "damaged", "", false)
	cleaner := &taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{"damaged": {{ID: "wt", TaskID: "damaged"}}}, inspectErr: &worktree.CleanupInspectionError{Stage: worktree.CleanupInspectionStageStatus, Reason: worktree.CleanupInspectionReasonCommandFailed, Err: errors.New("broken git")}}
	svc.SetWorktreeCleanup(cleaner)
	ctx := taskDeletePreflightHumanContext()
	preview, err := svc.TaskDeletePreflightWithOptions(ctx, []string{"damaged"}, false, DeleteTaskOptions{})
	require.NoError(t, err)
	require.True(t, preview.RequiresDiscardConsent)
	encoded, err := json.Marshal(preview)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"workspace_inspection":"unknown"`)
	require.NotEmpty(t, preview.ConfirmationID)

	cleaner.inventoryErr = errors.New("database unavailable")
	_, err = svc.TaskDeletePreflightWithOptions(ctx, []string{"damaged"}, false, unavailableDeleteOptions(t))
	require.ErrorContains(t, err, "database unavailable")
	cleaner.inventoryErr = nil
	cleaner.inspectErr = context.Canceled
	_, err = svc.TaskDeletePreflightWithOptions(ctx, []string{"damaged"}, false, unavailableDeleteOptions(t))
	require.ErrorIs(t, err, context.Canceled)
}

// @covers AC-TASKS-DELETE-RECOVERY-002.2
func TestDeleteUnavailableAdmissionRequiresExplicitAcknowledgement(t *testing.T) {
	for _, acknowledged := range []bool{false, true} {
		t.Run(map[bool]string{false: "discard only", true: "acknowledged"}[acknowledged], func(t *testing.T) {
			svc, _, repo := createTestService(t)
			svc.StopTaskResourceCleanupWorker()
			seedTaskDeletePreflightTask(t, repo, "damaged", "", false)
			svc.SetWorktreeCleanup(&unavailableIdentityCleanup{taskDeletePreflightCleanup: taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{"damaged": {{ID: "wt", TaskID: "damaged"}}}, inspectErr: &worktree.CleanupInspectionError{Stage: worktree.CleanupInspectionStageStatus, Reason: worktree.CleanupInspectionReasonCommandFailed, Err: errors.New("broken git")}}})
			options := DeleteTaskOptions{DiscardWorktreeChanges: true}
			if acknowledged {
				options = unavailableDeleteOptions(t)
			}
			err := svc.DeleteTaskWithOptions(context.Background(), "damaged", options)
			if !acknowledged {
				require.Error(t, err)
				_, err = repo.GetTask(context.Background(), "damaged")
				require.NoError(t, err)
				return
			}
			require.NoError(t, err)
			_, err = repo.GetTask(context.Background(), "damaged")
			require.Error(t, err)
			jobs, err := repo.ListTaskResourceCleanupJobs(context.Background(), "damaged")
			require.NoError(t, err)
			require.Len(t, jobs, 1)
			var saved map[string]any
			require.NoError(t, json.Unmarshal([]byte(jobs[0].ResourceSnapshot), &saved))
			require.Equal(t, true, saved["allow_unverified_workspace_changes"])
			require.NotEqual(t, string(models.WorkspaceCleanupRetain), saved["workspace_cleanup"])
		})
	}
}

type unavailableIdentityCleanup struct{ taskDeletePreflightCleanup }

func (c *unavailableIdentityCleanup) CaptureCleanupHeadOIDs(_ context.Context, worktrees []*worktree.Worktree) (map[string]string, error) {
	for _, wt := range worktrees {
		if wt.ID == "broken" {
			return nil, &worktree.CleanupInspectionError{Stage: worktree.CleanupInspectionStageCommit, Reason: worktree.CleanupInspectionReasonCommandFailed, Err: errors.New("bad object HEAD")}
		}
	}
	return map[string]string{"healthy": "1111111111111111111111111111111111111111"}, nil
}

// @covers AC-TASKS-DELETE-RECOVERY-002.2
// @covers AC-TASKS-DELETE-RECOVERY-002.3
func TestDeleteUnavailablePreparation(t *testing.T) {
	for _, cascade := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "cascade"}[cascade], func(t *testing.T) {
			svc, _, repo := createTestService(t)
			svc.StopTaskResourceCleanupWorker()
			seedTaskDeletePreflightTask(t, repo, "damaged", "", false)
			cleaner := &unavailableIdentityCleanup{taskDeletePreflightCleanup: taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{"damaged": {{ID: "broken", TaskID: "damaged"}, {ID: "healthy", TaskID: "damaged"}}}}}
			svc.SetWorktreeCleanup(cleaner)
			options := unavailableDeleteOptions(t)
			if cascade {
				h := NewHandoffService(repo, nil, nil, nil, nil, nil)
				h.SetTaskResourceCleaner(svc)
				_, err := h.DeleteTaskTreeWithOptions(context.Background(), "damaged", true, options)
				require.NoError(t, err)
			} else {
				require.NoError(t, svc.DeleteTaskWithOptions(context.Background(), "damaged", options))
			}
			_, err := repo.GetTask(context.Background(), "damaged")
			require.Error(t, err)
			jobs, err := repo.ListTaskResourceCleanupJobs(context.Background(), "damaged")
			require.NoError(t, err)
			require.Len(t, jobs, 1)
			var saved map[string]any
			require.NoError(t, json.Unmarshal([]byte(jobs[0].ResourceSnapshot), &saved))
			require.Equal(t, true, saved["cleanup_inventory_captured"])
			require.Equal(t, true, saved["allow_unverified_workspace_changes"])
			require.Equal(t, map[string]any{"healthy": "1111111111111111111111111111111111111111"}, saved["worktree_head_oids"])
			require.Contains(t, saved["cleanup_preparation_errors"], "broken")
		})
	}
}

type mixedUnavailableCleanup struct {
	recordingWorktreeCleanup
	repaired bool
}

func (c *mixedUnavailableCleanup) CaptureArchiveSourceManifests(ctx context.Context, worktrees []*worktree.Worktree) (map[string]worktree.ArchiveSourceManifest, error) {
	for _, wt := range worktrees {
		if wt.ID == "broken" && !c.repaired {
			return nil, errors.New("ownership marker unreadable")
		}
	}
	return c.recordingWorktreeCleanup.CaptureArchiveSourceManifests(ctx, worktrees)
}
func (c *mixedUnavailableCleanup) CleanupWorktreesWithOptions(ctx context.Context, worktrees []*worktree.Worktree, _ worktree.WorktreeCleanupOptions) error {
	return c.CleanupWorktrees(ctx, worktrees)
}

// @covers AC-TASKS-DELETE-RECOVERY-002.5
// @covers AC-TASKS-DELETE-RECOVERY-002.6
func TestDeleteUnavailableMixedCleanupRestart(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	cleaner := &mixedUnavailableCleanup{}
	svc.SetWorktreeCleanup(cleaner)
	payload := `{"workspace_cleanup":"delete","cleanup_version":1,"cleanup_inventory_captured":true,"allow_unverified_workspace_changes":true,"discard_worktree_changes":true,"worktrees":[{"id":"broken","task_id":"deleted","path":"/tasks/deleted/broken"},{"id":"healthy","task_id":"deleted","path":"/tasks/deleted/healthy"}],"worktree_head_oids":{"broken":"1111111111111111111111111111111111111111","healthy":"1111111111111111111111111111111111111111"}}`
	job := &models.TaskResourceCleanupJob{ID: "cleanup-mixed", TaskID: "deleted", OperationID: "cleanup-mixed", Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStateRunning, Attempts: 1, ResourceSnapshot: payload}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(context.Background(), job))
	var snapshot taskResourceCleanupSnapshot
	require.NoError(t, json.Unmarshal([]byte(payload), &snapshot))
	require.Error(t, svc.executeTaskResourceCleanupJob(context.Background(), job, &snapshot))
	require.Equal(t, []string{"healthy"}, cleaner.cleanedIDs())
	var sequence int
	var name, databasePath string
	require.NoError(t, repo.DB().QueryRow("PRAGMA database_list").Scan(&sequence, &name, &databasePath))
	require.NoError(t, repo.DB().Close())
	database, err := db.OpenSQLite(databasePath)
	require.NoError(t, err)
	reopened := sqlx.NewDb(database, "sqlite3")
	t.Cleanup(func() { _ = reopened.Close() })
	repo, err = sqliterepo.NewWithDB(reopened, reopened, nil)
	require.NoError(t, err)
	svc = NewService(Repos{Tasks: repo, Sessions: repo, Executors: repo, TaskEnvironments: repo, ResourceCleanups: repo}, nil, svc.logger, RepositoryDiscoveryConfig{})
	svc.SetWorktreeCleanup(cleaner)
	protected, err := repo.HasRetainedTaskWorkspace(context.Background(), "deleted")
	require.NoError(t, err)
	require.True(t, protected)
	saved, err := repo.GetTaskResourceCleanupJob(context.Background(), job.ID)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(saved.ResourceSnapshot), &snapshot))
	cleaner.repaired = true
	require.NoError(t, svc.executeTaskResourceCleanupJob(context.Background(), job, &snapshot))
	require.ElementsMatch(t, []string{"healthy", "broken"}, cleaner.cleanedIDs())
}

func TestDeleteUnavailableMissingIdentityProvider(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "task", "", false)
	svc.SetWorktreeCleanup(&taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{"task": {{ID: "wt", TaskID: "task"}}}})
	require.Error(t, svc.DeleteTaskWithOptions(context.Background(), "task", unavailableDeleteOptions(t)))
	_, err := repo.GetTask(context.Background(), "task")
	require.NoError(t, err)
}

func TestDeleteUnavailablePreservesPriorIdentity(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	prior := `{"worktrees":[{"id":"wt","task_id":"task","path":"/tasks/task/wt"}],"worktree_head_oids":{"wt":"original"},"worktree_task_dir_names":{"wt":"original-root"},"task_environment":{"id":"old-env","task_id":"task","workspace_path":"/old"},"ssh_task_dirs":[{"task_id":"task"}]}`
	require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{ID: "prior", OperationID: "prior", TaskID: "task", State: models.TaskResourceCleanupStateFailed, ResourceSnapshot: prior}))
	snapshot := taskResourceCleanupSnapshot{AllowUnverifiedWorkspaceChanges: true, Worktrees: []*worktree.Worktree{{ID: "wt", TaskID: "task", Path: "/tasks/task/wt"}}, WorktreeHeadOIDs: map[string]string{"wt": "replacement"}}
	require.NoError(t, svc.captureUnavailableCleanupInventory(ctx, "task", &snapshot))
	require.Equal(t, "original", snapshot.WorktreeHeadOIDs["wt"])
	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "old-env")
	require.Equal(t, "original-root", snapshot.WorktreeTaskDirNames["wt"])
}

func TestDeleteUnavailableMissingInspector(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "task", "", false)
	svc.SetWorktreeCleanup(nil)
	require.Error(t, svc.DeleteTaskWithOptions(context.Background(), "task", unavailableDeleteOptions(t)))
	_, err := repo.GetTask(context.Background(), "task")
	require.NoError(t, err)
}

func TestDeleteUnavailableCascadeRetryIsBounded(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	job := &models.TaskResourceCleanupJob{ID: "bounded", OperationID: "bounded", TaskID: "deleted", Trigger: models.TaskResourceCleanupTriggerCascadeDelete, State: models.TaskResourceCleanupStateRunning, Attempts: taskResourceCleanupMaxAttempts, ResourceSnapshot: `{"allow_unverified_workspace_changes":true}`}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, job))
	require.Error(t, svc.retryTaskResourceCleanupJob(ctx, job, errors.New("ownership unavailable")))
	saved, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskResourceCleanupStateFailed, saved.State)
}

func TestDeleteUnavailableConfirmationBinding(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskDeletePreflightTask(t, repo, "task", "", false)
	svc.SetWorktreeCleanup(&taskDeletePreflightCleanup{})
	ctx := taskDeletePreflightHumanContext()
	options := unavailableDeleteOptions(t)
	preview, err := svc.TaskDeletePreflightWithOptions(ctx, []string{"task"}, false, options)
	require.NoError(t, err)
	calls := 0
	effect := func() error { calls++; return nil }
	require.ErrorIs(t, svc.WithTaskDeleteConfirmationOptions(ctx, preview.ConfirmationID, "task", false, DeleteTaskOptions{DiscardWorktreeChanges: true}, effect), ErrTaskDeleteConfirmationMismatch)
	require.Zero(t, calls)
	require.NoError(t, svc.WithTaskDeleteConfirmationOptions(ctx, preview.ConfirmationID, "task", false, options, effect))
	require.Equal(t, 1, calls)
	require.ErrorIs(t, svc.WithTaskDeleteConfirmationOptions(ctx, preview.ConfirmationID, "task", false, options, effect), ErrTaskDeleteConfirmationReplay)
	require.Equal(t, 1, calls)
}

func TestDeleteUnavailableMissingInspectorCascade(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "task", "", false)
	svc.SetWorktreeCleanup(nil)
	h := NewHandoffService(repo, nil, nil, nil, nil, nil)
	h.SetTaskResourceCleaner(svc)
	_, err := h.DeleteTaskTreeWithOptions(context.Background(), "task", true, unavailableDeleteOptions(t))
	require.Error(t, err)
	_, err = repo.GetTask(context.Background(), "task")
	require.NoError(t, err)
}

func TestDeleteUnavailableFailedStopPreservesOnlyItsWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	cleaner := &mixedUnavailableCleanup{repaired: true}
	svc.SetWorktreeCleanup(cleaner)
	snapshot := taskResourceCleanupSnapshot{CleanupVersion: 1, CleanupInventoryCaptured: true, AllowUnverifiedWorkspaceChanges: true, DiscardWorktreeChanges: true, Sessions: []*models.TaskSession{{ID: "blocked", TaskID: "deleted"}, {ID: "stopped", TaskID: "deleted"}}, Worktrees: []*worktree.Worktree{{ID: "broken", TaskID: "deleted", SessionID: "blocked"}, {ID: "healthy", TaskID: "deleted", SessionID: "stopped"}}, WorktreeHeadOIDs: map[string]string{"broken": "original", "healthy": "original"}}
	payload, err := json.Marshal(snapshot)
	require.NoError(t, err)
	job := &models.TaskResourceCleanupJob{ID: "stop-partial", OperationID: "stop-partial", TaskID: "deleted", State: models.TaskResourceCleanupStateRunning, Attempts: 1, Trigger: models.TaskResourceCleanupTriggerDelete, ResourceSnapshot: string(payload)}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(context.Background(), job))
	ctx := recoveryclaim.WithTaskCleanupJob(context.Background(), recoveryclaim.TaskCleanupJob{ID: job.ID, TaskID: job.TaskID, Attempts: 1})
	resourceErrs, err := svc.cleanupUnverifiedWorktrees(ctx, job, &snapshot, taskRuntimeStopOutcome{failed: map[string]struct{}{"blocked": {}}})
	require.NoError(t, err)
	require.Error(t, errors.Join(resourceErrs...))
	require.Equal(t, []string{"healthy"}, cleaner.cleanedIDs())
	require.False(t, snapshot.RuntimeStopCompleted)
}
