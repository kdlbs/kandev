package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3
func TestArchiveReclaimPreservesCheckoutAfterOwnerStopFails(t *testing.T) {
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	path := filepath.Join(wt.Path, "running-agent-output.txt")
	if err := os.WriteFile(path, []byte("live work"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{
		SessionID: wt.SessionID, TaskID: wt.TaskID, Runtime: "standalone", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	svc.executionStopper = failingSessionStopper{}
	archiveDirectoryTestTask(t, svc, repo, wt.TaskID, true)
	job := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	reclaimErr := svc.processTaskResourceCleanupJob(t.Context(), job.ID)
	if content, err := os.ReadFile(path); err != nil || string(content) != "live work" {
		t.Fatalf("reclaim removed a running owner's checkout: content=%q, err=%v (reclaim=%v)", content, err, reclaimErr)
	}
	if reclaimErr != nil {
		t.Fatal(reclaimErr)
	}
	deferred, err := repo.GetTaskResourceCleanupJob(t.Context(), job.ID)
	if err != nil || deferred.State != models.TaskResourceCleanupStateWaitingForClean {
		t.Fatalf("live-owner reclaim was not deferred: %+v: %v", deferred, err)
	}
	if err := repo.DeleteExecutorRunningBySessionID(t.Context(), wt.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CompleteTaskResourceCleanupJob(t.Context(), job.ID, models.TaskResourceCleanupStatePending, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.processTaskResourceCleanupJob(t.Context(), job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(wt.Path)); !os.IsNotExist(err) {
		t.Fatalf("stopped owner's task directory was not reclaimed: %v", err)
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3
func TestArchiveReclaimPreservesCheckoutWithoutOwnerRuntimeInventory(t *testing.T) {
	for _, unavailable := range []string{"missing", "read-error"} {
		t.Run(unavailable, func(t *testing.T) {
			svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
			job := seedRetainedArchiveWorktree(t, svc, repo, wt.TaskID)
			svc.executors = nil
			if unavailable == "read-error" {
				svc.executors = &stubExecutors{runningByTaskErr: errors.New("runtime inventory unavailable")}
			}
			err := svc.processTaskResourceCleanupJob(t.Context(), job.ID)
			if _, statErr := os.Stat(wt.Path); statErr != nil {
				t.Fatalf("checkout removed without runtime inventory: %v (reclaim=%v)", statErr, err)
			}
			if err == nil {
				t.Fatal("missing runtime evidence did not keep reclaim retryable")
			}
		})
	}
}
