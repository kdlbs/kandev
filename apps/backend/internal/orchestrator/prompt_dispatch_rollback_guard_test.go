package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type rollbackStateWriteObserver struct {
	sessionExecutorStore
	writeStarted chan struct{}
}

func (o *rollbackStateWriteObserver) UpdateTaskSessionStateIfCurrent(
	ctx context.Context,
	id string,
	expected, next models.TaskSessionState,
	errorMessage string,
) (bool, time.Time, error) {
	select {
	case o.writeStarted <- struct{}{}:
	default:
	}
	return o.sessionExecutorStore.UpdateTaskSessionStateIfCurrent(ctx, id, expected, next, errorMessage)
}

func TestRollbackPromptClaimWaitsForSuccessorGuardAndKeepsItsTurn(t *testing.T) {
	ctx := context.Background()
	const taskID, sessionID = "task-rollback-guard", "session-rollback-guard"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.turnService = &repoTurnService{repo: repo}
	observer := &rollbackStateWriteObserver{
		sessionExecutorStore: svc.repo,
		writeStarted:         make(chan struct{}, 1),
	}
	svc.repo = observer

	claimedTurn, err := svc.turnService.StartTurn(ctx, sessionID)
	if err != nil {
		t.Fatalf("start claimed turn: %v", err)
	}
	dispatchGuard := svc.lockCancelInFlightGuard(sessionID)
	rollback := promptClaimRollback{
		previousSessionState: models.TaskSessionStateStarting,
		turnID:               claimedTurn.ID,
		createdTurn:          true,
		dispatchGuard:        dispatchGuard,
		dispatchGuardRelease: dispatchGuard.unlock,
	}
	// Provider acceptance releases the physical lock while promptTask retains
	// the guard reference for any later rollback.
	rollback.dispatchGuardRelease()

	successorGuard := svc.lockCancelInFlightGuard(sessionID)
	if successorGuard.mutex != dispatchGuard.mutex {
		successorGuard.release()
		dispatchGuard.release()
		t.Fatal("successor claim acquired a different session guard after provider acceptance")
	}
	if err := svc.turnService.CompleteTurn(ctx, claimedTurn.ID); err != nil {
		successorGuard.release()
		dispatchGuard.release()
		t.Fatalf("complete prior turn for successor: %v", err)
	}
	successorTurn, err := svc.turnService.StartTurn(ctx, sessionID)
	if err != nil {
		successorGuard.release()
		dispatchGuard.release()
		t.Fatalf("start successor turn: %v", err)
	}

	rollbackDone := make(chan struct{})
	go func() {
		svc.rollbackPromptClaim(ctx, taskID, sessionID, rollback)
		close(rollbackDone)
	}()
	blockedDeadline := time.NewTimer(100 * time.Millisecond)
	select {
	case <-observer.writeStarted:
		successorGuard.release()
		<-rollbackDone
		dispatchGuard.release()
		t.Fatal("rollback wrote session state while the successor held the session guard")
	case <-blockedDeadline.C:
	}
	blockedDeadline.Stop()

	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		successorGuard.release()
		<-rollbackDone
		dispatchGuard.release()
		t.Fatalf("read session while successor held guard: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		successorGuard.release()
		<-rollbackDone
		dispatchGuard.release()
		t.Fatalf("session state during successor claim = %q, want RUNNING", session.State)
	}

	successorGuard.release()
	select {
	case <-rollbackDone:
	case <-time.After(2 * time.Second):
		dispatchGuard.release()
		t.Fatal("prompt rollback did not finish after the successor released the guard")
	}
	dispatchGuard.release()

	finalSession, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("read session after rollback: %v", err)
	}
	if finalSession.State != models.TaskSessionStateRunning {
		t.Fatalf("session state after stale rollback = %q, want successor RUNNING state", finalSession.State)
	}
	activeTurn, err := svc.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil || activeTurn == nil || activeTurn.ID != successorTurn.ID {
		t.Fatalf("active turn after stale rollback = %+v, err=%v, want successor %q", activeTurn, err, successorTurn.ID)
	}
}
