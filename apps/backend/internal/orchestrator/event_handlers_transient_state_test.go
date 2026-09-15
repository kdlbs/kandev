package orchestrator

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func transientRetryNoticeStateCount(svc *Service) int {
	svc.transientRetryNoticeStatesMu.Lock()
	defer svc.transientRetryNoticeStatesMu.Unlock()
	return len(svc.transientRetryNoticeStates)
}

func TestTransientRetryNoticeState_CachedPromptDoesNotOwnState(t *testing.T) {
	svc := &Service{}

	svc.rememberTurnPrompt("uncertain", "prompt", "", false, nil)

	if got := transientRetryNoticeStateCount(svc); got != 0 {
		t.Fatalf("notice state entries for an unaccepted prompt = %d, want 0", got)
	}
}

func TestTransientRetryNoticeState_ReclaimsSuccessfulSessionChurn(t *testing.T) {
	svc := &Service{}

	for i := 0; i < 1000; i++ {
		sessionID := "churn-" + strconv.Itoa(i)
		svc.rememberTurnPrompt(sessionID, "prompt", "", false, nil)
		svc.resetTransientRetry(sessionID)
	}

	if got := transientRetryNoticeStateCount(svc); got != 0 {
		t.Fatalf("notice state entries after successful session churn = %d, want 0", got)
	}
}

func TestTransientRetryNoticeState_ReclaimsDeletedSessionAfterFence(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-delete", "deleted", models.TaskSessionStateCompleted)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.transientRetryNoticeFenceTTL = 10 * time.Millisecond
	svc.rememberTurnPrompt("deleted", "prompt", "", false, nil)
	svc.scheduleTransientRetry("task-delete", "deleted", "execution-1", 1, time.Hour)
	t.Cleanup(svc.cancelAllTransientRetries)

	// Session deletion ends the session incarnation even when no execution
	// remains. The fence must remain while stale events can still arrive, then
	// reclaim itself.
	if err := svc.DeleteSession(context.Background(), "deleted"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if got := transientRetryNoticeStateCount(svc); got != 1 {
		t.Fatalf("notice state entries immediately after deletion = %d, want 1 fenced entry", got)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if transientRetryNoticeStateCount(svc) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("deleted session notice state was not reclaimed after bounded fence")
}

func TestTransientRetryNoticeState_ReferenceCountsConcurrentUsers(t *testing.T) {
	svc := &Service{}
	first, releaseFirst := svc.acquireTransientRetryNoticeState("shared")
	first.mu.Lock()

	secondState := make(chan *transientRetryNoticeState, 1)
	secondAcquired := make(chan struct{})
	secondLocked := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		second, releaseSecond := svc.acquireTransientRetryNoticeState("shared")
		secondState <- second
		close(secondAcquired)
		second.mu.Lock()
		close(secondLocked)
		second.mu.Unlock()
		releaseSecond()
		close(secondDone)
	}()

	<-secondAcquired
	if got := transientRetryNoticeStateCount(svc); got != 1 {
		t.Fatalf("notice state entries with a mutex holder and waiter = %d, want 1", got)
	}
	if second := <-secondState; second != first {
		t.Fatal("concurrent users acquired different notice state mutexes")
	}

	first.mu.Unlock()
	<-secondLocked
	releaseFirst()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("concurrent notice state user did not finish")
	}
	if got := transientRetryNoticeStateCount(svc); got != 0 {
		t.Fatalf("notice state entries after concurrent users released = %d, want 0", got)
	}
}
