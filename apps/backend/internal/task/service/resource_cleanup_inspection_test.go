package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

type inspectionFailingWorktreeCleanup struct {
	inspErr *worktree.CleanupInspectionError
}

func (f inspectionFailingWorktreeCleanup) OnTaskDeleted(context.Context, string) error {
	return f.inspErr
}

func (f inspectionFailingWorktreeCleanup) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return nil, nil
}

func (f inspectionFailingWorktreeCleanup) CleanupWorktrees(context.Context, []*worktree.Worktree) error {
	return f.inspErr
}

func (f inspectionFailingWorktreeCleanup) CleanupWorktreesWithOptions(context.Context, []*worktree.Worktree, worktree.WorktreeCleanupOptions) error {
	return f.inspErr
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6
// @covers AC-TASKS-RUNTIME-CLEANUP-001.9
// @covers AC-TASKS-RUNTIME-CLEANUP-001.32
func TestResourceCleanupInspectionFailureAttributesStageAndReason(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	log, logs := newObservedServiceLogger(t)
	svc.logger = log

	inspErr := &worktree.CleanupInspectionError{
		Stage:  worktree.CleanupInspectionStageBranch,
		Reason: worktree.CleanupInspectionReasonRepoUnavailable,
		Err:    errors.New("simulated repo unavailable"),
	}
	svc.SetWorktreeCleanup(inspectionFailingWorktreeCleanup{inspErr: inspErr})

	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Worktrees:                     []*worktree.Worktree{{ID: "wt-insp", TaskID: "task-insp"}},
		ArchiveSourceManifestCaptured: true,
	})
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}

	job := &models.TaskResourceCleanupJob{
		ID:               "job-insp-fail",
		OperationID:      "delete:job-insp-fail",
		TaskID:           "task-insp",
		Trigger:          models.TaskResourceCleanupTriggerDelete,
		State:            models.TaskResourceCleanupStatePending,
		ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("create cleanup job: %v", err)
	}

	err = svc.processTaskResourceCleanupJob(ctx, job.ID)
	if err == nil {
		t.Fatal("processTaskResourceCleanupJob succeeded, want error")
	}

	var gotInspErr *worktree.CleanupInspectionError
	if !errors.As(err, &gotInspErr) {
		t.Fatalf("error %v does not unwrap to CleanupInspectionError", err)
	}
	if gotInspErr.Stage != worktree.CleanupInspectionStageBranch {
		t.Fatalf("stage = %q, want %q", gotInspErr.Stage, worktree.CleanupInspectionStageBranch)
	}
	if gotInspErr.Reason != worktree.CleanupInspectionReasonRepoUnavailable {
		t.Fatalf("reason = %q, want %q", gotInspErr.Reason, worktree.CleanupInspectionReasonRepoUnavailable)
	}

	reloaded, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if reloaded.State != models.TaskResourceCleanupStateRetryWait {
		t.Fatalf("job state = %q, want retry_wait", reloaded.State)
	}

	entries := logs.FilterMessage("task resource cleanup job entered retry wait or failed").All()
	if len(entries) != 1 {
		t.Fatalf("logged retry entries = %d, want 1", len(entries))
	}
	ctxMap := entries[0].ContextMap()
	if ctxMap["job_id"] != job.ID || ctxMap["task_id"] != job.TaskID {
		t.Fatalf("context map = %+v, want job_id=%s task_id=%s", ctxMap, job.ID, job.TaskID)
	}
	if ctxMap["attempt"] != int64(reloaded.Attempts) && ctxMap["attempt"] != reloaded.Attempts {
		t.Fatalf("logged attempt = %v, want %d", ctxMap["attempt"], reloaded.Attempts)
	}
	if ctxMap["stage"] != worktree.CleanupInspectionStageBranch || ctxMap["reason"] != worktree.CleanupInspectionReasonRepoUnavailable {
		t.Fatalf("logged stage/reason = %v/%v, want %s/%s", ctxMap["stage"], ctxMap["reason"],
			worktree.CleanupInspectionStageBranch, worktree.CleanupInspectionReasonRepoUnavailable)
	}
}
