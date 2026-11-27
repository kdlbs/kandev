package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5
func TestArchiveReclaimStartupWakesLegacyWaitingJob(t *testing.T) {
	for _, unarchive := range []bool{false, true} {
		t.Run(map[bool]string{false: "archived", true: "unarchived"}[unarchive], func(t *testing.T) {
			svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
			path := filepath.Join(wt.Path, "legacy-local-work.txt")
			if err := os.WriteFile(path, []byte("uncommitted"), 0o600); err != nil {
				t.Fatal(err)
			}
			job := seedRetainedArchiveWorktree(t, svc, repo, wt.TaskID)
			future := time.Now().UTC().Add(24 * time.Hour)
			if err := repo.CompleteTaskResourceCleanupJob(t.Context(), job.ID, models.TaskResourceCleanupStateWaitingForClean, "", &future); err != nil {
				t.Fatal(err)
			}
			if unarchive {
				handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
				handoff.SetTaskResourceCleaner(svc)
				if _, err := handoff.UnarchiveTaskTree(t.Context(), wt.TaskID); err != nil {
					t.Fatal(err)
				}
			}
			restarted := newServiceOverRepository(t, repo)
			restarted.StopTaskResourceCleanupWorker()
			manager := newCleanupTestWorktreeManagerAt(t, repo, filepath.Dir(filepath.Dir(wt.Path)))
			restarted.SetWorktreeCleanup(manager)
			if err := restarted.ResumeTaskResourceCleanupJobs(t.Context()); err != nil {
				t.Fatal(err)
			}
			job, err := repo.GetTaskResourceCleanupJob(t.Context(), job.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := models.TaskResourceCleanupStateSucceeded
			if unarchive {
				want = models.TaskResourceCleanupStateCancelled
			}
			if job.State != want {
				t.Fatalf("legacy job = %s: %s, want %s", job.State, job.LastError, want)
			}
			if unarchive {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("unarchived work lost: %v", err)
				}
				return
			}
			if _, err := os.Stat(filepath.Dir(wt.Path)); !os.IsNotExist(err) {
				t.Fatalf("legacy task root retained: %v", err)
			}
		})
	}
}

func TestArchiveReclaimStartupKeepsFreshBorrowerDelay(t *testing.T) {
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	job := seedRetainedArchiveWorktree(t, svc, repo, wt.TaskID)
	future := time.Now().UTC().Add(24 * time.Hour)
	if err := repo.CompleteTaskResourceCleanupJob(t.Context(), job.ID, models.TaskResourceCleanupStateWaitingForClean, "", &future); err != nil {
		t.Fatal(err)
	}
	env, err := repo.GetTaskEnvironmentByTaskID(t.Context(), wt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(t.Context(), &models.Task{ID: "active-borrower", WorkspaceID: "ws-" + wt.TaskID, WorkflowID: "wf-" + wt.TaskID, Title: "Active borrower"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "borrower-session", TaskID: "active-borrower", TaskEnvironmentID: env.ID, State: models.TaskSessionStateRunning}); err != nil {
		t.Fatal(err)
	}
	restarted := newServiceOverRepository(t, repo)
	restarted.StopTaskResourceCleanupWorker()
	restarted.SetWorktreeCleanup(newCleanupTestWorktreeManagerAt(t, repo, filepath.Dir(filepath.Dir(wt.Path))))
	if err := restarted.ResumeTaskResourceCleanupJobs(t.Context()); err != nil {
		t.Fatal(err)
	}
	first, err := repo.GetTaskResourceCleanupJob(t.Context(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != models.TaskResourceCleanupStateWaitingForClean || first.Attempts != 1 {
		t.Fatalf("borrowed cleanup = %+v", first)
	}
	if err := restarted.ResumeTaskResourceCleanupJobs(t.Context()); err != nil {
		t.Fatal(err)
	}
	second, err := repo.GetTaskResourceCleanupJob(t.Context(), job.ID)
	if err != nil || second.Attempts != first.Attempts || !second.NextAttemptAt.Equal(*first.NextAttemptAt) {
		t.Fatalf("fresh in-use delay reset: %+v: %v", second, err)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("borrower lost worktree: %v", err)
	}
}
