package orchestrator

import (
	"context"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5
func TestFocusRefreshReleasesAdmissionWhenLifecycleLockHeld(t *testing.T) {
	const sessionID = "session-focus-admission-lock"
	svc := &Service{repo: setupTestRepo(t), messageQueue: messagequeue.NewServiceMemory(testLogger())}

	releaseLifecycle := svc.acquireSessionLifecycleLock(sessionID)
	lifecycleReleased := false
	t.Cleanup(func() {
		if !lifecycleReleased {
			releaseLifecycle()
		}
	})

	admitted := make(chan struct{})
	focusDone := make(chan error, 1)
	go func() {
		focusDone <- svc.withSessionPromptAdmission(context.Background(), sessionID, func(ctx context.Context) error {
			close(admitted)
			svc.refreshFocusedLiveSession(ctx, "task-focus-admission-lock", sessionID)
			return nil
		})
	}()
	select {
	case <-admitted:
	case <-time.After(time.Second):
		t.Fatal("focus refresh did not acquire session admission")
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	followupErr := svc.messageQueue.WithSessionAdmission(waitCtx, sessionID, func(context.Context) error {
		return nil
	})
	cancel()
	releaseLifecycle()
	lifecycleReleased = true

	if err := <-focusDone; err != nil {
		t.Fatalf("focus admission callback: %v", err)
	}
	if followupErr != nil {
		t.Fatalf("follow-up session admission failed while lifecycle lock was held: %v", followupErr)
	}
}

type idleSuspensionCallCounter struct {
	calls int
}

func (s *idleSuspensionCallCounter) SuspendIdle(context.Context, agentruntime.IdleSuspensionIdentity) error {
	s.calls++
	return nil
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5
func TestIdleSuspensionRevalidationReleasesAdmissionWhenLifecycleLockHeld(t *testing.T) {
	const sessionID = "session-idle-admission-lock"
	svc := &Service{repo: setupTestRepo(t), messageQueue: messagequeue.NewServiceMemory(testLogger())}
	row := &models.ExecutorRunning{SessionID: sessionID}
	candidate := workspaceIdleCandidate{key: idleParkingCandidateKey{sessionID: sessionID}}
	svc.idleParkingCandidates = map[idleParkingCandidateKey]time.Time{candidate.key: time.Now().UTC()}
	suspender := &idleSuspensionCallCounter{}

	releaseLifecycle := svc.acquireSessionLifecycleLock(sessionID)
	lifecycleReleased := false
	t.Cleanup(func() {
		if !lifecycleReleased {
			releaseLifecycle()
		}
	})

	admitted := make(chan struct{})
	reaperDone := make(chan error, 1)
	go func() {
		reaperDone <- svc.withSessionPromptAdmission(context.Background(), sessionID, func(ctx context.Context) error {
			close(admitted)
			svc.suspendWorkspaceIdleCandidate(ctx, row, candidate, suspender, &idleParkingScanSummary{})
			return nil
		})
	}()
	select {
	case <-admitted:
	case <-time.After(time.Second):
		t.Fatal("idle revalidation did not acquire session admission")
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	followupErr := svc.messageQueue.WithSessionAdmission(waitCtx, sessionID, func(context.Context) error {
		return nil
	})
	cancel()
	releaseLifecycle()
	lifecycleReleased = true

	if err := <-reaperDone; err != nil {
		t.Fatalf("idle revalidation admission callback: %v", err)
	}
	if followupErr != nil {
		t.Fatalf("follow-up session admission failed while lifecycle lock was held: %v", followupErr)
	}
	if suspender.calls != 0 {
		t.Fatalf("idle suspension calls = %d, want 0 when revalidation is skipped", suspender.calls)
	}
	if _, retry := svc.idleParkingCandidates[candidate.key]; !retry {
		t.Fatal("busy lifecycle lock removed the idle candidate instead of leaving it for a later scan")
	}
}
