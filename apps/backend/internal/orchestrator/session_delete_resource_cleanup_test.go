package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// fakeSessionDeleteResourceCleanup is a test double for
// SessionDeleteResourceCleanup that records every call so tests can assert
// DeleteSession's ordering and fail-closed behavior without a real task
// service or worktree manager.
type fakeSessionDeleteResourceCleanup struct {
	mu sync.Mutex

	prepareOperationID string
	prepareErr         error
	prepareCalls       []struct{ sessionID, taskID string }

	startErr   error
	startCalls []string

	cancelErr   error
	cancelCalls []string
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
	return f.prepareOperationID, nil
}

func (f *fakeSessionDeleteResourceCleanup) StartPreparedTaskResourceCleanup(_ context.Context, operationID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls = append(f.startCalls, operationID)
	return f.startErr
}

func (f *fakeSessionDeleteResourceCleanup) CancelPreparedTaskResourceCleanup(_ context.Context, operationID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelCalls = append(f.cancelCalls, operationID)
	return f.cancelErr
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

func TestDeleteSession_ActivatesResourceCleanupAfterCommit(t *testing.T) {
	const (
		taskID    = "task-cleanup-activate"
		sessionID = "session-cleanup-activate"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	cleanup := &fakeSessionDeleteResourceCleanup{prepareOperationID: "session_delete:op-1"}
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
	if len(cleanup.cancelCalls) != 0 {
		t.Fatalf("cancel calls = %v, want none on a successful delete", cleanup.cancelCalls)
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
	if len(cleanup.cancelCalls) != 0 {
		t.Fatalf("cancel calls = %v, want none — nothing was ever persisted to cancel", cleanup.cancelCalls)
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
