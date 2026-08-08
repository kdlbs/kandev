package service

import (
	"context"
	"errors"
	"reflect"
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

// TestExecuteSessionDeleteResourceCleanup_ReclaimsEveryWorktreeIndependently
// asserts reclaimedIDs set-equality, not just a call count. A call-count-only
// assertion is mutation-blind: reclaiming the same worktree twice (leaking a
// multi-repo session's other worktree) still produces 2 calls and would pass
// silently (test-supervisor round-2 finding,
// docs/specs/session-delete-resource-cleanup REVIEW-ROUND: 2).
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
	gotIDs := map[string]bool{}
	for _, id := range fake.reclaimedIDs {
		gotIDs[id] = true
	}
	wantIDs := map[string]bool{"wt-a": true, "wt-b": true}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("reclaimed IDs = %v, want exactly {wt-a, wt-b} (each reclaimed once, not one worktree twice)",
			fake.reclaimedIDs)
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

// TestProcessTaskResourceCleanupJob_SessionDeleteRetriesTransientFailureThenSucceeds
// drives a session_delete job through the actual worker dispatch
// (processTaskResourceCleanupJob -> executeClaimedTaskResourceCleanupJob ->
// executeSessionDeleteResourceCleanup), not by calling
// executeSessionDeleteResourceCleanup or retryTaskResourceCleanupJob
// directly. Round-2 test-supervisor mutation-proved this gap: making a
// failed reclamation swallow its error and report succeeded anyway left
// every package green, because nothing exercised the dispatch path for this
// trigger. Asserts the job reaches retry_wait (not failed) after one
// transient failure, then succeeds on the next attempt — and that the fake
// reclaimer actually ran twice, proving the retry re-executes real
// reclamation rather than only flipping job state.
func TestProcessTaskResourceCleanupJob_SessionDeleteRetriesTransientFailureThenSucceeds(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()

	fake := &fakeSessionWorktreeCleanup{
		failReclaim: map[string]error{"wt-retry": errors.New("git worktree remove: transient lock")},
	}
	taskSvc.SetWorktreeCleanup(fake)

	job := &models.TaskResourceCleanupJob{
		ID: "job-session-retry", OperationID: "session_delete:job-session-retry",
		TaskID: "task-session-retry", Trigger: models.TaskResourceCleanupTriggerSessionDelete,
		State:            models.TaskResourceCleanupStatePending,
		ResourceSnapshot: `{"session_id":"session-retry","worktrees":[{"id":"wt-retry","path":"/tmp/retry"}]}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}

	if err := taskSvc.processTaskResourceCleanupJob(ctx, job.ID); err == nil {
		t.Fatal("expected the first (failing) attempt to return an error")
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateRetryWait {
		t.Fatalf("state after transient failure = %q, want retry_wait", got.State)
	}
	if !strings.Contains(got.LastError, "/tmp/retry") {
		t.Fatalf("LastError = %q, want it to name /tmp/retry", got.LastError)
	}
	if len(fake.reclaimedIDs) != 1 {
		t.Fatalf("reclaim attempts so far = %v, want exactly one", fake.reclaimedIDs)
	}

	// Clear the transient failure so the retried attempt succeeds.
	// MarkTaskResourceCleanupJobRunning claims from state IN (pending,
	// retry_wait), so retry_wait alone lets the worker pick this job up
	// again without any manual state manipulation.
	fake.failReclaim = nil
	if err := taskSvc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
		t.Fatalf("second (successful) attempt: %v", err)
	}
	got, err = repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("reload after retry: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("state after retry succeeds = %q, want succeeded", got.State)
	}
	if len(fake.reclaimedIDs) != 2 || fake.reclaimedIDs[1] != "wt-retry" {
		t.Fatalf("reclaimedIDs = %v, want a second real reclaim attempt for wt-retry", fake.reclaimedIDs)
	}
}

// TestProcessTaskResourceCleanupJob_SessionDeleteExhaustsRetryBudgetAndRecordsPath
// proves a session_delete job that fails on every attempt goes terminally
// failed once the shared eight-attempt budget (taskResourceCleanupMaxAttempts)
// is exhausted, with the unreclaimed worktree path recorded on the job — the
// spec's "Reclamation failure SHALL be observable" and the Durability
// scenario's eight-attempt-exhaustion case
// (docs/specs/session-delete-resource-cleanup).
func TestProcessTaskResourceCleanupJob_SessionDeleteExhaustsRetryBudgetAndRecordsPath(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()

	fake := &fakeSessionWorktreeCleanup{
		failReclaim: map[string]error{"wt-exhausted": errors.New("git worktree remove: permission denied")},
	}
	taskSvc.SetWorktreeCleanup(fake)

	job := &models.TaskResourceCleanupJob{
		ID: "job-session-exhausted", OperationID: "session_delete:job-session-exhausted",
		TaskID: "task-session-exhausted", Trigger: models.TaskResourceCleanupTriggerSessionDelete,
		State: models.TaskResourceCleanupStatePending, Attempts: taskResourceCleanupMaxAttempts - 1,
		ResourceSnapshot: `{"session_id":"session-exhausted","worktrees":[{"id":"wt-exhausted","path":"/tmp/exhausted"}]}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}

	if err := taskSvc.processTaskResourceCleanupJob(ctx, job.ID); err == nil {
		t.Fatal("expected the budget-exhausting attempt to return an error")
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateFailed {
		t.Fatalf("state after exhausting the retry budget = %q, want failed", got.State)
	}
	if !strings.Contains(got.LastError, "/tmp/exhausted") {
		t.Fatalf("LastError = %q, want it to name the unreclaimed path /tmp/exhausted", got.LastError)
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

// TestSessionDeleteCleanupJob_FullLifecycle drives prepare, commit the
// session delete, activate, then lets the durable worker run, and asserts
// the job reaches succeeded and the fake reclaimer actually ran. The calls
// here are made directly, in a hardcoded correct order — this proves the
// reclaim mechanism itself (reference counting evaluated after the row is
// gone) works when given a correctly-ordered sequence. It does NOT guard
// against orchestrator.Service.DeleteSession itself getting that ordering
// wrong, since it never calls DeleteSession; that regression guard is
// TestDeleteSession_ActivatesResourceCleanupAfterCommit in
// internal/orchestrator, whose fake checks the row's real state at the
// instant activation is called.
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
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.reclaimedCalls != 1 || len(fake.reclaimedIDs) != 1 || fake.reclaimedIDs[0] != "wt-committed" {
		t.Fatalf("reclaimed = %v (%d calls), want exactly [wt-committed] — job state alone does not "+
			"prove reclamation actually ran", fake.reclaimedIDs, fake.reclaimedCalls)
	}
}

// TestResolveSessionDeleteResourceCleanupAfterMutationError_CommittedActivatesJob
// covers review-round-1 finding 3 (docs/specs/session-delete-resource-cleanup):
// DeleteTaskSession can return an error for a mutation that actually
// committed (a realistic ambiguous-outcome class on this repo's supported
// Postgres deployment target). Unconditionally cancelling the prepared
// cleanup job in that case permanently leaks the worktree — the cancelled
// job was the only remaining pointer to it. The resolve method must detect
// the commit and activate the job instead.
func TestResolveSessionDeleteResourceCleanupAfterMutationError_CommittedActivatesJob(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-ambiguous-committed", "session-ambiguous-committed")
	wt := &worktree.Worktree{ID: "wt-ambiguous-committed", SessionID: "session-ambiguous-committed", TaskID: "task-ambiguous-committed", Path: "/tmp/ambiguous-committed"}
	fake := &fakeSessionWorktreeCleanup{
		inventory: map[string][]*worktree.Worktree{"session-ambiguous-committed": {wt}},
	}
	taskSvc.SetWorktreeCleanup(fake)
	taskSvc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	operationID, err := taskSvc.PrepareSessionDeleteResourceCleanup(ctx, "session-ambiguous-committed", "task-ambiguous-committed")
	if err != nil {
		t.Fatalf("PrepareSessionDeleteResourceCleanup: %v", err)
	}
	// The mutation actually commits...
	if err := repo.DeleteTaskSession(ctx, "session-ambiguous-committed"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	// ...but the caller only observes an ambiguous error (e.g. a lost ack)
	// and resolves rather than unconditionally cancelling.
	taskSvc.ResolveSessionDeleteResourceCleanupAfterMutationError(ctx, operationID)
	waitForCleanupDone(t, taskSvc)

	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJobByOperationID: %v", err)
	}
	if job.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("job state = %q, want succeeded — a committed delete must not leave its cleanup job "+
			"cancelled (last_error=%q)", job.State, job.LastError)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.reclaimedCalls != 1 || len(fake.reclaimedIDs) != 1 || fake.reclaimedIDs[0] != "wt-ambiguous-committed" {
		t.Fatalf("reclaimed = %v (%d calls), want exactly [wt-ambiguous-committed]", fake.reclaimedIDs, fake.reclaimedCalls)
	}
}

// TestResolveSessionDeleteResourceCleanupAfterMutationError_NotCommittedCancelsJob
// covers the other half: when the mutation genuinely did not commit (the
// session row still exists), the prepared job must still be cancelled —
// resolving must not blindly activate every ambiguous-error job, only ones
// that actually committed.
func TestResolveSessionDeleteResourceCleanupAfterMutationError_NotCommittedCancelsJob(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-ambiguous-uncommitted", "session-ambiguous-uncommitted")
	wt := &worktree.Worktree{ID: "wt-ambiguous-uncommitted", SessionID: "session-ambiguous-uncommitted", TaskID: "task-ambiguous-uncommitted", Path: "/tmp/ambiguous-uncommitted"}
	fake := &fakeSessionWorktreeCleanup{
		inventory: map[string][]*worktree.Worktree{"session-ambiguous-uncommitted": {wt}},
	}
	taskSvc.SetWorktreeCleanup(fake)

	operationID, err := taskSvc.PrepareSessionDeleteResourceCleanup(ctx, "session-ambiguous-uncommitted", "task-ambiguous-uncommitted")
	if err != nil {
		t.Fatalf("PrepareSessionDeleteResourceCleanup: %v", err)
	}
	// The mutation genuinely fails — session row is deliberately NOT deleted.

	taskSvc.ResolveSessionDeleteResourceCleanupAfterMutationError(ctx, operationID)

	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJobByOperationID: %v", err)
	}
	if job.State != models.TaskResourceCleanupStateCancelled {
		t.Fatalf("job state = %q, want cancelled — the session row still exists, so the delete never committed", job.State)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.reclaimedCalls != 0 {
		t.Fatalf("reclaim was attempted for an uncommitted delete: %d calls", fake.reclaimedCalls)
	}
}
