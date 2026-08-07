package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// sessionRowChecker is the minimal repo surface fakeSessionDeleteResourceCleanup
// needs to check whether a session row still exists at call time. Satisfied
// by *sqliterepo.Repository (setupTestRepo's return type).
type sessionRowChecker interface {
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
}

// fakeSessionDeleteResourceCleanup is a test double for
// SessionDeleteResourceCleanup that records every call so tests can assert
// DeleteSession's ordering and fail-closed behavior without a real task
// service or worktree manager.
type fakeSessionDeleteResourceCleanup struct {
	mu sync.Mutex

	// repo, when set, lets StartPreparedTaskResourceCleanup check — at the
	// instant it is called — whether the session row it was prepared for is
	// still present. This makes ordering bugs (activation racing ahead of
	// the DeleteTaskSession commit) visible instead of passing regardless of
	// call order, which a fake that only appends to a slice cannot catch.
	repo                 sessionRowChecker
	sessionIDByOperation map[string]string

	prepareOperationID string
	prepareErr         error
	prepareCalls       []struct{ sessionID, taskID string }

	startErr               error
	startCalls             []string
	startSessionRowWasGone []bool

	resolveCalls []string
}

func (f *fakeSessionDeleteResourceCleanup) PrepareSessionDeleteResourceCleanup(
	_ context.Context, sessionID, taskID string,
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prepareCalls = append(f.prepareCalls, struct{ sessionID, taskID string }{sessionID, taskID})
	if f.prepareErr != nil {
		return "", f.prepareErr
	}
	if f.prepareOperationID != "" {
		if f.sessionIDByOperation == nil {
			f.sessionIDByOperation = make(map[string]string)
		}
		f.sessionIDByOperation[f.prepareOperationID] = sessionID
	}
	return f.prepareOperationID, nil
}

func (f *fakeSessionDeleteResourceCleanup) StartPreparedTaskResourceCleanup(ctx context.Context, operationID string) error {
	f.mu.Lock()
	repo := f.repo
	sessionID := f.sessionIDByOperation[operationID]
	f.mu.Unlock()

	rowWasGone := false
	if repo != nil && sessionID != "" {
		if _, err := repo.GetTaskSession(ctx, sessionID); err != nil {
			rowWasGone = true
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls = append(f.startCalls, operationID)
	f.startSessionRowWasGone = append(f.startSessionRowWasGone, rowWasGone)
	return f.startErr
}

func (f *fakeSessionDeleteResourceCleanup) ResolveSessionDeleteResourceCleanupAfterMutationError(_ context.Context, operationID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolveCalls = append(f.resolveCalls, operationID)
}

// fakeWorktreeSessionCache records ForgetSession calls.
type fakeWorktreeSessionCache struct {
	mu      sync.Mutex
	forgets []string
}

func (f *fakeWorktreeSessionCache) ForgetSession(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgets = append(f.forgets, sessionID)
}

// TestDeleteSession_ActivatesResourceCleanupAfterCommit is the regression
// guard for the spec's ordering requirement (docs/specs/
// session-delete-resource-cleanup: cleanup intent is captured before the row
// is deleted, but the job is only ACTIVATED after DeleteTaskSession commits —
// activating early would let the durable worker's reference-count query
// observe a session row that has not been deleted yet, which can make an
// exclusively-held worktree look shared and never get reclaimed). The fake's
// StartPreparedTaskResourceCleanup queries the real repo for the session row
// at the instant it is called, so an implementation that activates before
// the delete commits fails this test instead of passing regardless of order.
func TestDeleteSession_ActivatesResourceCleanupAfterCommit(t *testing.T) {
	const (
		taskID    = "task-cleanup-activate"
		sessionID = "session-cleanup-activate"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	cleanup := &fakeSessionDeleteResourceCleanup{prepareOperationID: "session_delete:op-1", repo: repo}
	svc.sessionResourceCleanup = cleanup
	cache := &fakeWorktreeSessionCache{}
	svc.worktreeSessionCache = cache

	if err := svc.DeleteSession(t.Context(), sessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if _, err := repo.GetTaskSession(t.Context(), sessionID); err == nil {
		t.Fatal("session row should be gone after DeleteSession")
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if len(cleanup.prepareCalls) != 1 || cleanup.prepareCalls[0].sessionID != sessionID || cleanup.prepareCalls[0].taskID != taskID {
		t.Fatalf("prepare calls = %+v, want one call for (%s, %s)", cleanup.prepareCalls, sessionID, taskID)
	}
	if len(cleanup.startCalls) != 1 || cleanup.startCalls[0] != "session_delete:op-1" {
		t.Fatalf("start calls = %v, want [session_delete:op-1]", cleanup.startCalls)
	}
	if len(cleanup.startSessionRowWasGone) != 1 || !cleanup.startSessionRowWasGone[0] {
		t.Fatalf("startSessionRowWasGone = %v, want [true] — activation must happen strictly after "+
			"the session row is deleted, not before or concurrently", cleanup.startSessionRowWasGone)
	}
	if len(cleanup.resolveCalls) != 0 {
		t.Fatalf("resolve calls = %v, want none on a successful delete", cleanup.resolveCalls)
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.forgets) != 1 || cache.forgets[0] != sessionID {
		t.Fatalf("worktree cache forgets = %v, want [%s]", cache.forgets, sessionID)
	}
}

// TestDeleteSession_RefusesRunningSessionWithoutTouchingCleanup covers the
// spec's refusal scenario directly (docs/specs/session-delete-resource-cleanup:
// "GIVEN a session in RUNNING state, WHEN a delete is requested, THEN the
// request is rejected with an error naming the state, the session row still
// exists, and its worktree directory still exists"). The state-check itself
// predates this feature, but the feature adds a resource-cleanup
// collaborator ahead of it in the call chain — this pins that the refusal
// still happens first and never reaches (or activates) cleanup.
func TestDeleteSession_RefusesRunningSessionWithoutTouchingCleanup(t *testing.T) {
	const (
		taskID    = "task-cleanup-running-refused"
		sessionID = "session-cleanup-running-refused"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	cleanup := &fakeSessionDeleteResourceCleanup{prepareOperationID: "session_delete:op-refused"}
	svc.sessionResourceCleanup = cleanup

	err := svc.DeleteSession(t.Context(), sessionID)
	if err == nil {
		t.Fatal("expected DeleteSession to refuse a RUNNING session")
	}
	if !strings.Contains(err.Error(), string(models.TaskSessionStateRunning)) {
		t.Fatalf("error = %q, want it to name the RUNNING state", err.Error())
	}

	if _, getErr := repo.GetTaskSession(t.Context(), sessionID); getErr != nil {
		t.Fatalf("session row should survive a refused delete: %v", getErr)
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if len(cleanup.prepareCalls) != 0 {
		t.Fatalf("prepare calls = %+v, want none — refusal must happen before cleanup is ever touched", cleanup.prepareCalls)
	}
	if len(cleanup.startCalls) != 0 {
		t.Fatalf("start calls = %v, want none", cleanup.startCalls)
	}
}

func TestDeleteSession_InventoryOrPersistFailureFailsClosed(t *testing.T) {
	const (
		taskID    = "task-cleanup-fail-closed"
		sessionID = "session-cleanup-fail-closed"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	cleanup := &fakeSessionDeleteResourceCleanup{prepareErr: errors.New("list worktrees: boom")}
	svc.sessionResourceCleanup = cleanup

	if err := svc.DeleteSession(t.Context(), sessionID); err == nil {
		t.Fatal("expected DeleteSession to fail closed when cleanup intent capture fails")
	}

	if _, err := repo.GetTaskSession(t.Context(), sessionID); err != nil {
		t.Fatalf("session row should survive a fail-closed delete: %v", err)
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if len(cleanup.startCalls) != 0 {
		t.Fatalf("start calls = %v, want none when prepare failed", cleanup.startCalls)
	}
	if len(cleanup.resolveCalls) != 0 {
		t.Fatalf("resolve calls = %v, want none — nothing was ever persisted to resolve", cleanup.resolveCalls)
	}
}

func TestDeleteSession_NoCleanupCollaboratorStillDeletes(t *testing.T) {
	const (
		taskID    = "task-cleanup-unwired"
		sessionID = "session-cleanup-unwired"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	// sessionResourceCleanup and worktreeSessionCache deliberately left nil,
	// mirroring an isolated test harness or an install without a task service.

	if err := svc.DeleteSession(t.Context(), sessionID); err != nil {
		t.Fatalf("DeleteSession without a cleanup collaborator: %v", err)
	}
	if _, err := repo.GetTaskSession(t.Context(), sessionID); err == nil {
		t.Fatal("session row should be gone after DeleteSession")
	}
}

func TestDeleteSession_NothingToReclaimSkipsActivation(t *testing.T) {
	const (
		taskID    = "task-cleanup-nothing"
		sessionID = "session-cleanup-nothing"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	// Empty operationID mirrors PrepareSessionDeleteResourceCleanup's
	// "nothing to reclaim" contract (repo-less task, no worktrees).
	cleanup := &fakeSessionDeleteResourceCleanup{prepareOperationID: ""}
	svc.sessionResourceCleanup = cleanup

	if err := svc.DeleteSession(t.Context(), sessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if len(cleanup.startCalls) != 0 {
		t.Fatalf("start calls = %v, want none when there was nothing to reclaim", cleanup.startCalls)
	}
}

// commitThenErrorSessionRepo wraps a real sessionExecutorStore and makes
// DeleteTaskSession return an error AFTER the underlying delete actually
// commits — simulating the ambiguous-outcome class review-round-1 finding 3
// is about (e.g. a commit-then-lost-ack on this repo's supported Postgres
// deployment target).
type commitThenErrorSessionRepo struct {
	sessionExecutorStore
	err error
}

func (r *commitThenErrorSessionRepo) DeleteTaskSession(ctx context.Context, id string) error {
	if err := r.sessionExecutorStore.DeleteTaskSession(ctx, id); err != nil {
		return err
	}
	return r.err
}

// TestDeleteSession_AmbiguousMutationErrorResolvesRatherThanBlindlyCancels
// covers review-round-1 finding 3 (docs/specs/session-delete-resource-cleanup):
// DeleteSession must not unconditionally cancel the prepared cleanup job when
// DeleteTaskSession returns an error — the mutation may have actually
// committed, in which case a blind cancel permanently leaks the worktree
// (the cancelled job was the only remaining pointer to it). It must resolve
// instead, which checks whether the row is really gone.
func TestDeleteSession_AmbiguousMutationErrorResolvesRatherThanBlindlyCancels(t *testing.T) {
	const (
		taskID    = "task-cleanup-ambiguous"
		sessionID = "session-cleanup-ambiguous"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	cleanup := &fakeSessionDeleteResourceCleanup{prepareOperationID: "session_delete:op-ambiguous"}
	svc.sessionResourceCleanup = cleanup
	commitErr := errors.New("transport lost after commit")
	svc.repo = &commitThenErrorSessionRepo{sessionExecutorStore: svc.repo, err: commitErr}

	err := svc.DeleteSession(t.Context(), sessionID)
	if !errors.Is(err, commitErr) {
		t.Fatalf("DeleteSession error = %v, want it to wrap %v", err, commitErr)
	}

	if _, getErr := repo.GetTaskSession(t.Context(), sessionID); getErr == nil {
		t.Fatal("the mutation actually committed — session row should be gone")
	}

	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if len(cleanup.resolveCalls) != 1 || cleanup.resolveCalls[0] != "session_delete:op-ambiguous" {
		t.Fatalf("resolve calls = %v, want exactly one call for session_delete:op-ambiguous — "+
			"an ambiguous error must resolve, not blindly cancel", cleanup.resolveCalls)
	}
}
