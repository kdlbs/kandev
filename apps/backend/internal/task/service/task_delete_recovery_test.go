package service

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// @covers AC-TASKS-DELETE-RECOVERY-001.2
func TestRetainedCleanupNeverRemovesWorkspace(t *testing.T) {
	for _, mode := range []string{"delete", "retain"} {
		t.Run(mode, func(t *testing.T) {
			svc, _, _ := createTestService(t)
			svc.StopTaskResourceCleanupWorker()
			cleaner := &recordingWorktreeCleanup{}
			svc.SetWorktreeCleanup(cleaner)
			var snapshot taskResourceCleanupSnapshot
			payload := `{"workspace_cleanup":"` + mode + `","worktrees":[{"id":"retained-wt","task_id":"retained-task","path":"/unused/retained"}],"archive_source_manifest_captured":true}`
			if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
				t.Fatal(err)
			}
			job := &models.TaskResourceCleanupJob{ID: "retained-job", TaskID: "retained-task", Trigger: models.TaskResourceCleanupTriggerDelete}
			if err := svc.executeTaskResourceCleanupJob(context.Background(), job, &snapshot); err != nil {
				t.Fatal(err)
			}
			want := 1
			if mode == "retain" {
				want = 0
			}
			if got := len(cleaner.cleanedIDs()); got != want {
				t.Fatalf("removed %d worktrees, want %d in %s mode", got, want, mode)
			}
		})
	}
}

// @covers AC-TASKS-DELETE-RECOVERY-001.1
func TestRetainDeletionSkipsDamagedWorkspaceInspection(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "damaged", "", false)
	cleaner := &taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{
		"damaged": {{ID: "damaged-wt", TaskID: "damaged"}},
	}, inspectErr: ErrTaskDeletePreflightUnavailable}
	svc.SetWorktreeCleanup(cleaner)
	var options DeleteTaskOptions
	if err := json.Unmarshal([]byte(`{"WorkspaceCleanup":"retain"}`), &options); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteTaskWithOptions(context.Background(), "damaged", options); err != nil {
		t.Fatalf("retain deletion failed: %v", err)
	}
	if _, err := repo.GetTask(context.Background(), "damaged"); err == nil {
		t.Fatal("task still exists")
	}
	if cleaner.inspectCalls != 0 {
		t.Fatal("retain deletion inspected the workspace")
	}
}

// @covers AC-TASKS-DELETE-RECOVERY-001.1
func TestRetainCascadeSkipsPhysicalAdmission(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "retain-root", "", false)
	seedTaskDeletePreflightTask(t, repo, "retain-child", "retain-root", false)
	cleaner := &taskDeletePreflightCleanup{inspectErr: ErrTaskDeletePreflightUnavailable}
	svc.SetWorktreeCleanup(cleaner)
	handoff := NewHandoffService(repo, nil, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(svc)
	_, err := handoff.DeleteTaskTreeWithOptions(context.Background(), "retain-root", true, DeleteTaskOptions{WorkspaceCleanup: models.WorkspaceCleanupRetain})
	if err != nil {
		t.Fatal(err)
	}
	if cleaner.inspectCalls != 0 {
		t.Fatal("cascade inspected workspace")
	}
	for _, id := range []string{"retain-root", "retain-child"} {
		if _, err := repo.GetTask(context.Background(), id); err == nil {
			t.Fatalf("task %s remains", id)
		}
		jobs, err := repo.ListTaskResourceCleanupJobs(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 1 {
			t.Fatalf("cleanup snapshots = %d", len(jobs))
		}
		var snapshot taskResourceCleanupSnapshot
		if err := json.Unmarshal([]byte(jobs[0].ResourceSnapshot), &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.WorkspaceCleanup != models.WorkspaceCleanupRetain {
			t.Fatal("cascade lost retention")
		}
	}
}

func TestOlderCleanupCannotRemoveRetainedWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "retain-before-retry", "", false)
	require.NoError(t, svc.DeleteTaskWithOptions(context.Background(), "retain-before-retry", DeleteTaskOptions{WorkspaceCleanup: models.WorkspaceCleanupRetain}))
	cleaner := &recordingWorktreeCleanup{}
	svc.SetWorktreeCleanup(cleaner)
	job := &models.TaskResourceCleanupJob{ID: "old", TaskID: "retain-before-retry", Trigger: models.TaskResourceCleanupTriggerDelete}
	snapshot := &taskResourceCleanupSnapshot{Worktrees: []*worktree.Worktree{{ID: "kept", TaskID: job.TaskID, Path: "/kept"}}}
	require.Error(t, svc.executeTaskResourceCleanupJob(context.Background(), job, snapshot))
	require.Empty(t, cleaner.cleanedIDs())
	retained, err := repo.HasRetainedTaskWorkspace(context.Background(), job.TaskID)
	require.NoError(t, err)
	require.True(t, retained)
}

func TestRetainedShutdownWithoutStopperStaysPending(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	snapshot := &taskResourceCleanupSnapshot{WorkspaceCleanup: models.WorkspaceCleanupRetain, TaskEnvironment: &models.TaskEnvironment{ExecutorType: string(models.ExecutorTypeLocal)}, StopTargets: []persistedTaskStopTarget{{SessionID: "session", ExecutionID: "execution"}}}
	require.Error(t, svc.executeRetainedTaskShutdown(context.Background(), &models.TaskResourceCleanupJob{TaskID: "deleted"}, snapshot))
}

func TestRetainDeletionPreservesEarlierRuntimeStopHandles(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedTaskDeletePreflightTask(t, repo, "retained-history", "", false)
	_, err := svc.persistTaskResourceCleanup(ctx, "retained-history", models.TaskResourceCleanupTriggerArchive, "earlier-cleanup", nil, nil, []taskStopTarget{{sessionID: "earlier-session", executionID: "earlier-execution"}}, nil, taskEnvironmentCleanup{}, true, false, "")
	require.NoError(t, err)
	require.NoError(t, svc.DeleteTaskWithOptions(ctx, "retained-history", DeleteTaskOptions{WorkspaceCleanup: models.WorkspaceCleanupRetain}))
	result := svc.RetainedTaskDeletionResult(ctx, []string{"retained-history"})
	require.True(t, result.Success)
	require.True(t, result.RuntimeStopPending)
	jobs, err := repo.ListTaskResourceCleanupJobs(ctx, "retained-history")
	require.NoError(t, err)
	for _, job := range jobs {
		var snapshot taskResourceCleanupSnapshot
		require.NoError(t, json.Unmarshal([]byte(job.ResourceSnapshot), &snapshot))
		if snapshot.WorkspaceCleanup == models.WorkspaceCleanupRetain {
			require.Contains(t, snapshot.StopTargets, persistedTaskStopTarget{SessionID: "earlier-session", ExecutionID: "earlier-execution"})
			return
		}
	}
	t.Fatal("retained snapshot missing")
}
