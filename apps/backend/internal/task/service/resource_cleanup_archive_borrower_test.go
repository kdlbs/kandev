package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

type archiveReclaimBorrower struct {
	taskID    string
	sessionID string
	state     models.TaskSessionState
	archived  bool
}

// @covers AC-TASKS-SHARED-WORKTREE-HOLDS-001.3
func TestArchiveReclaimKeepsBorrowerHistoryAfterRelease(t *testing.T) {
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	borrower := archiveReclaimBorrower{
		taskID:    "task-borrower-released",
		sessionID: "session-borrower-released",
		state:     models.TaskSessionStateCancelled,
		archived:  true,
	}
	seedArchiveReclaimBorrower(t, repo, wt.TaskID, borrower)
	seedArchiveReclaimBorrowerMessage(t, repo, borrower)

	job := seedRetainedArchiveWorktree(t, svc, repo, wt.TaskID)
	if err := svc.processTaskResourceCleanupJob(t.Context(), job.ID); err != nil {
		t.Fatalf("process archive reclaim job: %v", err)
	}

	job, err := repo.GetTaskResourceCleanupJob(t.Context(), job.ID)
	if err != nil || job.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("reclaim = %+v: %v", job, err)
	}
	if _, err := os.Stat(wt.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released borrower hold left the worktree on disk: %v", err)
	}
	session, err := repo.GetTaskSession(t.Context(), borrower.sessionID)
	if err != nil || session == nil || session.TaskID != borrower.taskID {
		t.Fatalf("borrower session after reclaim = %+v: %v", session, err)
	}
	message, err := repo.GetMessage(t.Context(), archiveReclaimBorrowerMessageID(borrower))
	if err != nil || message.TaskSessionID != borrower.sessionID {
		t.Fatalf("borrower message after reclaim = %+v: %v", message, err)
	}
}

// @covers AC-TASKS-SHARED-WORKTREE-HOLDS-001.1
func TestArchiveReclaimDefersForLiveBorrower(t *testing.T) {
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	seedArchiveReclaimBorrower(t, repo, wt.TaskID, archiveReclaimBorrower{
		taskID:    "task-borrower-live",
		sessionID: "session-borrower-live",
		state:     models.TaskSessionStateCompleted,
	})

	job := seedRetainedArchiveWorktree(t, svc, repo, wt.TaskID)
	if err := svc.processTaskResourceCleanupJob(t.Context(), job.ID); err != nil {
		t.Fatalf("process archive reclaim job: %v", err)
	}

	job, err := repo.GetTaskResourceCleanupJob(t.Context(), job.ID)
	if err != nil || job.State != models.TaskResourceCleanupStateWaitingForClean {
		t.Fatalf("reclaim = %+v: %v, want deferred for live borrower", job, err)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("live borrower must keep the shared worktree: %v", err)
	}
}

// @covers AC-TASKS-SHARED-WORKTREE-HOLDS-001.3
func TestResumeReclaimsWorktreePinnedByArchivedBorrower(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	svc.StopTaskResourceCleanupWorker()
	seedArchiveReclaimBorrower(t, repo, wt.TaskID, archiveReclaimBorrower{
		taskID:    "task-borrower-pinned",
		sessionID: "session-borrower-pinned",
		state:     models.TaskSessionStateCancelled,
		archived:  true,
	})
	// The owner was archived before this rule shipped: its active worktree row
	// has no archive cleanup job, so startup backfill must create the reclaim.
	if err := repo.ArchiveTask(ctx, wt.TaskID); err != nil {
		t.Fatalf("archive owner: %v", err)
	}

	restarted := newServiceOverRepository(t, repo)
	manager := newCleanupTestWorktreeManagerAt(t, repo, filepath.Dir(filepath.Dir(wt.Path)))
	manager.SetRepositoryProvider(worktree.NewRepositoryAdapter(repo))
	restarted.SetWorktreeCleanup(manager)
	if err := restarted.ResumeTaskResourceCleanupJobs(ctx); err != nil {
		t.Fatalf("resume task cleanup jobs: %v", err)
	}

	job := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	if job.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("backfilled reclaim state = %q, want succeeded", job.State)
	}
	if _, err := os.Stat(wt.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backfilled reclaim left a pinned worktree: %v", err)
	}
}

// @covers AC-TASKS-SHARED-WORKTREE-HOLDS-001.3
func TestStartupWakeReclaimsDeferredArchiveJobAfterBorrowerArchive(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	borrower := archiveReclaimBorrower{
		taskID:    "task-borrower-woken",
		sessionID: "session-borrower-woken",
		state:     models.TaskSessionStateCancelled,
	}
	seedArchiveReclaimBorrower(t, repo, wt.TaskID, borrower)

	// The owner is archived while the borrower is still live, so the reclaim
	// keeps the worktree and defers to a future recheck.
	job := seedRetainedArchiveWorktree(t, svc, repo, wt.TaskID)
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
		t.Fatalf("process archive reclaim job: %v", err)
	}
	deferred, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil || deferred.State != models.TaskResourceCleanupStateWaitingForClean {
		t.Fatalf("deferred reclaim = %+v: %v", deferred, err)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("live borrower must keep the shared worktree: %v", err)
	}

	// The borrower is archived after the owner. Keep the recheck in the future
	// so ordinary due-job processing skips the job, and age updated_at so the
	// next startup wake admits it instead of treating it as just deferred.
	if err := repo.ArchiveTask(ctx, borrower.taskID); err != nil {
		t.Fatalf("archive borrower: %v", err)
	}
	now := time.Now().UTC()
	if _, err := repo.DB().ExecContext(ctx, `
		UPDATE task_resource_cleanup_jobs SET updated_at = ?, next_attempt_at = ? WHERE id = ?
	`, now.Add(-time.Hour), now.Add(time.Hour), job.ID); err != nil {
		t.Fatalf("age deferred reclaim: %v", err)
	}

	restarted := newServiceOverRepository(t, repo)
	manager := newCleanupTestWorktreeManagerAt(t, repo, filepath.Dir(filepath.Dir(wt.Path)))
	manager.SetRepositoryProvider(worktree.NewRepositoryAdapter(repo))
	restarted.SetWorktreeCleanup(manager)
	if err := restarted.ResumeTaskResourceCleanupJobs(ctx); err != nil {
		t.Fatalf("resume task cleanup jobs: %v", err)
	}

	woken, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil || woken.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("woken reclaim = %+v: %v", woken, err)
	}
	if _, err := os.Stat(wt.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("woken reclaim left the worktree on disk: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, borrower.sessionID)
	if err != nil || session == nil || session.TaskID != borrower.taskID {
		t.Fatalf("borrower session after wake = %+v: %v", session, err)
	}
}

func seedArchiveReclaimBorrower(
	t *testing.T,
	repo *sqliterepo.Repository,
	ownerTaskID string,
	borrower archiveReclaimBorrower,
) {
	t.Helper()
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, borrower.taskID, borrower.sessionID)
	session, err := repo.GetTaskSession(ctx, borrower.sessionID)
	if err != nil {
		t.Fatalf("load borrower session: %v", err)
	}
	session.TaskEnvironmentID = "env-" + ownerTaskID
	session.State = borrower.state
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("link borrower session to owner environment: %v", err)
	}
	if borrower.archived {
		if err := repo.ArchiveTask(ctx, borrower.taskID); err != nil {
			t.Fatalf("archive borrower task: %v", err)
		}
	}
}

func seedArchiveReclaimBorrowerMessage(
	t *testing.T,
	repo *sqliterepo.Repository,
	borrower archiveReclaimBorrower,
) {
	t.Helper()
	ctx := context.Background()
	turn := &models.Turn{
		ID:            "turn-" + borrower.sessionID,
		TaskSessionID: borrower.sessionID,
		TaskID:        borrower.taskID,
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("create borrower turn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            archiveReclaimBorrowerMessageID(borrower),
		TurnID:        turn.ID,
		TaskID:        borrower.taskID,
		TaskSessionID: borrower.sessionID,
		AuthorType:    models.MessageAuthorAgent,
		Content:       "borrower transcript",
	}); err != nil {
		t.Fatalf("create borrower message: %v", err)
	}
}

func archiveReclaimBorrowerMessageID(borrower archiveReclaimBorrower) string {
	return "message-" + borrower.sessionID
}
