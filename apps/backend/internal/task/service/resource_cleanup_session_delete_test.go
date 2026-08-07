package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// fakeSessionWorktreeCleanup is a test double for the WorktreeCleanup +
// sessionWorktreeInventoryProvider + sessionWorktreeReclaimer capability set
// PrepareSessionDeleteResourceCleanup / executeSessionDeleteResourceCleanup
// type-assert against. inventory maps sessionID -> worktrees to return from
// GetAllBySessionID; failReclaim maps worktree ID -> error to return from
// ReclaimSessionWorktree.
type fakeSessionWorktreeCleanup struct {
	mu             sync.Mutex
	inventory      map[string][]*worktree.Worktree
	inventoryErr   error
	failReclaim    map[string]error
	reclaimedIDs   []string
	reclaimedCalls int
}

func (f *fakeSessionWorktreeCleanup) OnTaskDeleted(context.Context, string) error { return nil }

func (f *fakeSessionWorktreeCleanup) GetAllBySessionID(_ context.Context, sessionID string) ([]*worktree.Worktree, error) {
	if f.inventoryErr != nil {
		return nil, f.inventoryErr
	}
	return f.inventory[sessionID], nil
}

func (f *fakeSessionWorktreeCleanup) ReclaimSessionWorktree(_ context.Context, wt *worktree.Worktree) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reclaimedCalls++
	f.reclaimedIDs = append(f.reclaimedIDs, wt.ID)
	if err, ok := f.failReclaim[wt.ID]; ok {
		return err
	}
	return nil
}

func TestPrepareSessionDeleteResourceCleanup_NothingToReclaim(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-nothing", "session-nothing")
	taskSvc.SetWorktreeCleanup(&fakeSessionWorktreeCleanup{})

	operationID, err := taskSvc.PrepareSessionDeleteResourceCleanup(ctx, "session-nothing", "task-nothing")
	if err != nil {
		t.Fatalf("PrepareSessionDeleteResourceCleanup: %v", err)
	}
	if operationID != "" {
		t.Fatalf("operationID = %q, want empty when nothing to reclaim", operationID)
	}
	var count int
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_resource_cleanup_jobs WHERE task_id = ?`, "task-nothing",
	).Scan(&count); err != nil {
		t.Fatalf("count cleanup jobs: %v", err)
	}
	if count != 0 {
		t.Fatalf("cleanup jobs created = %d, want 0", count)
	}
}

func TestPrepareSessionDeleteResourceCleanup_InventoryReadFailureFailsClosed(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-read-fail", "session-read-fail")
	taskSvc.SetWorktreeCleanup(&fakeSessionWorktreeCleanup{inventoryErr: errors.New("boom")})

	operationID, err := taskSvc.PrepareSessionDeleteResourceCleanup(ctx, "session-read-fail", "task-read-fail")
	if err == nil {
		t.Fatal("expected inventory read failure to fail closed")
	}
	if operationID != "" {
		t.Fatalf("operationID = %q, want empty on failure", operationID)
	}
	var count int
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_resource_cleanup_jobs WHERE task_id = ?`, "task-read-fail",
	).Scan(&count); err != nil {
		t.Fatalf("count cleanup jobs: %v", err)
	}
	if count != 0 {
		t.Fatalf("cleanup jobs created despite inventory read failure = %d, want 0", count)
	}
}

func TestPrepareSessionDeleteResourceCleanup_PersistsPreparedJobWithSessionSnapshot(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-persist", "session-persist")
	wt := &worktree.Worktree{ID: "wt-1", SessionID: "session-persist", TaskID: "task-persist", Path: "/tmp/wt-1"}
	taskSvc.SetWorktreeCleanup(&fakeSessionWorktreeCleanup{
		inventory: map[string][]*worktree.Worktree{"session-persist": {wt}},
	})

	operationID, err := taskSvc.PrepareSessionDeleteResourceCleanup(ctx, "session-persist", "task-persist")
	if err != nil {
		t.Fatalf("PrepareSessionDeleteResourceCleanup: %v", err)
	}
	if operationID == "" {
		t.Fatal("expected a non-empty operationID when the session holds a worktree")
	}
	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJobByOperationID: %v", err)
	}
	if job.Trigger != models.TaskResourceCleanupTriggerSessionDelete {
		t.Fatalf("trigger = %q, want session_delete", job.Trigger)
	}
	if job.State != models.TaskResourceCleanupStatePrepared {
		t.Fatalf("state = %q, want prepared", job.State)
	}
	if job.TaskID != "task-persist" {
		t.Fatalf("task_id = %q, want task-persist", job.TaskID)
	}
	snapshot, err := decodeSessionDeleteCleanupSnapshot(job.ResourceSnapshot)
	if err != nil {
		t.Fatalf("decodeSessionDeleteCleanupSnapshot: %v", err)
	}
	if snapshot.SessionID != "session-persist" {
		t.Fatalf("snapshot session_id = %q, want session-persist", snapshot.SessionID)
	}
	if len(snapshot.Worktrees) != 1 || snapshot.Worktrees[0].ID != "wt-1" {
		t.Fatalf("snapshot worktrees = %+v, want [wt-1]", snapshot.Worktrees)
	}
}

func TestSessionDeleteCleanupMutationCommitted(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-committed", "session-committed")
	job := &models.TaskResourceCleanupJob{
		ResourceSnapshot: `{"session_id":"session-committed"}`,
	}

	committed, err := taskSvc.sessionDeleteCleanupMutationCommitted(ctx, job)
	if err != nil {
		t.Fatalf("sessionDeleteCleanupMutationCommitted (session still live): %v", err)
	}
	if committed {
		t.Fatal("mutation reported committed while the session row still exists")
	}

	if err := repo.DeleteTaskSession(ctx, "session-committed"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	committed, err = taskSvc.sessionDeleteCleanupMutationCommitted(ctx, job)
	if err != nil {
		t.Fatalf("sessionDeleteCleanupMutationCommitted (session gone): %v", err)
	}
	if !committed {
		t.Fatal("mutation reported uncommitted after the session row was removed")
	}
}

func TestExecuteSessionDeleteResourceCleanup_ReclaimsEveryWorktreeIndependently(t *testing.T) {
	taskSvc, _ := setupOfficeTest(t)
	fake := &fakeSessionWorktreeCleanup{}
	taskSvc.SetWorktreeCleanup(fake)
	job := &models.TaskResourceCleanupJob{
		ID: "job-multi-repo",
		ResourceSnapshot: `{"session_id":"session-multi","worktrees":[
			{"id":"wt-a","path":"/tmp/a"},
			{"id":"wt-b","path":"/tmp/b"}
		]}`,
	}

	if err := taskSvc.executeSessionDeleteResourceCleanup(context.Background(), job); err != nil {
		t.Fatalf("executeSessionDeleteResourceCleanup: %v", err)
	}
	if fake.reclaimedCalls != 2 {
		t.Fatalf("reclaim calls = %d, want 2", fake.reclaimedCalls)
	}
}

func TestExecuteSessionDeleteResourceCleanup_FailurePathSurvivesInError(t *testing.T) {
	taskSvc, _ := setupOfficeTest(t)
	fake := &fakeSessionWorktreeCleanup{
		failReclaim: map[string]error{"wt-fails": errors.New("git worktree remove: permission denied")},
	}
	taskSvc.SetWorktreeCleanup(fake)
	job := &models.TaskResourceCleanupJob{
		ID: "job-failure-path",
		ResourceSnapshot: `{"session_id":"session-failure","worktrees":[
			{"id":"wt-ok","path":"/tmp/ok"},
			{"id":"wt-fails","path":"/tmp/unreclaimed"}
		]}`,
	}

	err := taskSvc.executeSessionDeleteResourceCleanup(context.Background(), job)
	if err == nil {
		t.Fatal("expected reclamation failure to be returned")
	}
	if !strings.Contains(err.Error(), "/tmp/unreclaimed") {
		t.Fatalf("error = %q, want it to name the unreclaimed path /tmp/unreclaimed", err.Error())
	}
	if strings.Contains(err.Error(), "/tmp/ok") {
		t.Fatalf("error = %q, should not blame the worktree that reclaimed successfully", err.Error())
	}
}

func TestExecuteSessionDeleteResourceCleanup_NoWorktreesIsNoop(t *testing.T) {
	taskSvc, _ := setupOfficeTest(t)
	taskSvc.SetWorktreeCleanup(&fakeSessionWorktreeCleanup{})
	job := &models.TaskResourceCleanupJob{ResourceSnapshot: `{"session_id":"session-empty"}`}

	if err := taskSvc.executeSessionDeleteResourceCleanup(context.Background(), job); err != nil {
		t.Fatalf("executeSessionDeleteResourceCleanup with no worktrees: %v", err)
	}
}

// TestSessionDeleteCleanupJob_FullLifecycle drives the same sequence
// DeleteSession's orchestrator wiring performs — prepare, commit the session
// delete, activate, let the durable worker run — and asserts the job reaches
// succeeded and the fake reclaimer actually ran. This is the regression guard
// from spec.md: an implementation that evaluates "other live holders" before
// the session row is gone (instead of after, as this flow guarantees) would
// never reclaim an exclusively-held worktree.
func TestSessionDeleteCleanupJob_FullLifecycle(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-lifecycle", "session-lifecycle")
	wt := &worktree.Worktree{ID: "wt-lifecycle", SessionID: "session-lifecycle", TaskID: "task-lifecycle", Path: "/tmp/lifecycle"}
	fake := &fakeSessionWorktreeCleanup{
		inventory: map[string][]*worktree.Worktree{"session-lifecycle": {wt}},
	}
	taskSvc.SetWorktreeCleanup(fake)
	taskSvc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	operationID, err := taskSvc.PrepareSessionDeleteResourceCleanup(ctx, "session-lifecycle", "task-lifecycle")
	if err != nil {
		t.Fatalf("PrepareSessionDeleteResourceCleanup: %v", err)
	}
	if operationID == "" {
		t.Fatal("expected a job to be prepared")
	}
	if err := repo.DeleteTaskSession(ctx, "session-lifecycle"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	if err := taskSvc.StartPreparedTaskResourceCleanup(ctx, operationID); err != nil {
		t.Fatalf("StartPreparedTaskResourceCleanup: %v", err)
	}
	waitForCleanupDone(t, taskSvc)

	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJobByOperationID: %v", err)
	}
	if job.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("job state = %q, want succeeded (last_error=%q)", job.State, job.LastError)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.reclaimedCalls != 1 || len(fake.reclaimedIDs) != 1 || fake.reclaimedIDs[0] != "wt-lifecycle" {
		t.Fatalf("reclaimed = %v (%d calls), want exactly [wt-lifecycle]", fake.reclaimedIDs, fake.reclaimedCalls)
	}
}

// TestSessionDeleteCleanupJob_RestartBeforeCommitIsCancelled reproduces a
// crash between PrepareSessionDeleteResourceCleanup and the session row
// actually being deleted: on restart the prepared job must be cancelled, not
// acted on, because the session it named never actually went away.
func TestSessionDeleteCleanupJob_RestartBeforeCommitIsCancelled(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-uncommitted-sd", "session-uncommitted-sd")
	wt := &worktree.Worktree{ID: "wt-uncommitted", SessionID: "session-uncommitted-sd", TaskID: "task-uncommitted-sd", Path: "/tmp/uncommitted"}
	fake := &fakeSessionWorktreeCleanup{
		inventory: map[string][]*worktree.Worktree{"session-uncommitted-sd": {wt}},
	}
	taskSvc.SetWorktreeCleanup(fake)

	operationID, err := taskSvc.PrepareSessionDeleteResourceCleanup(ctx, "session-uncommitted-sd", "task-uncommitted-sd")
	if err != nil {
		t.Fatalf("PrepareSessionDeleteResourceCleanup: %v", err)
	}
	// Session row is deliberately NOT deleted, simulating a crash before the
	// commit step.

	if err := taskSvc.StartTaskResourceCleanupWorker(ctx); err != nil {
		t.Fatalf("restart cleanup worker: %v", err)
	}
	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != models.TaskResourceCleanupStateCancelled {
		t.Fatalf("uncommitted session delete cleanup state = %q, want cancelled", job.State)
	}
	if fake.reclaimedCalls != 0 {
		t.Fatalf("reclaim was attempted for an uncommitted delete: %d calls", fake.reclaimedCalls)
	}
}

// TestSessionDeleteCleanupJob_RestartAfterCommitResumes reproduces a crash
// between the session row being deleted and StartPreparedTaskResourceCleanup
// being called: restart reconciliation must still activate and complete it.
func TestSessionDeleteCleanupJob_RestartAfterCommitResumes(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-committed-sd", "session-committed-sd")
	wt := &worktree.Worktree{ID: "wt-committed", SessionID: "session-committed-sd", TaskID: "task-committed-sd", Path: "/tmp/committed"}
	fake := &fakeSessionWorktreeCleanup{
		inventory: map[string][]*worktree.Worktree{"session-committed-sd": {wt}},
	}
	taskSvc.SetWorktreeCleanup(fake)

	operationID, err := taskSvc.PrepareSessionDeleteResourceCleanup(ctx, "session-committed-sd", "task-committed-sd")
	if err != nil {
		t.Fatalf("PrepareSessionDeleteResourceCleanup: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, "session-committed-sd"); err != nil {
		t.Fatalf("commit session delete: %v", err)
	}
	// StartPreparedTaskResourceCleanup deliberately not called, simulating a
	// crash between commit and activation.

	if err := taskSvc.StartTaskResourceCleanupWorker(ctx); err != nil {
		t.Fatalf("restart cleanup worker: %v", err)
	}
	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State == models.TaskResourceCleanupStatePrepared {
		t.Fatal("committed session delete cleanup remained prepared after restart")
	}
	if job.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("job state = %q, want succeeded (last_error=%q)", job.State, job.LastError)
	}
}
