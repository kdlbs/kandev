package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type contextualPassthroughTestManager struct {
	*reentrantPassthroughAgentManager
	publish func(context.Context, string) error
}

func (m *contextualPassthroughTestManager) MarkPassthroughRunningWithContext(ctx context.Context, sessionID string) error {
	return m.publish(ctx, sessionID)
}

func (m *mockAgentManager) MarkPassthroughRunningWithContext(_ context.Context, sessionID string) error {
	return m.MarkPassthroughRunning(sessionID)
}

func TestPromptTaskPassthroughSynchronousRunningPreservesAdmission(t *testing.T) {
	for _, failWrite := range []int{0, 1, 2} {
		t.Run([]string{"success", "no write", "partial write"}[failWrite], func(t *testing.T) {
			testPassthroughGuardedPublication(t, failWrite)
		})
	}
}

func testPassthroughGuardedPublication(t *testing.T, failWrite int) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	baseManager := &mockAgentManager{
		isPassthrough: true, isAgentRunning: true, repoForExecutionLookup: repo,
		passthroughConfigSet: true,
		passthroughConfig:    agents.PassthroughConfig{Supported: true, SubmitSequence: "\r", SubmitDelay: time.Nanosecond},
	}
	manager := &contextualPassthroughTestManager{reentrantPassthroughAgentManager: &reentrantPassthroughAgentManager{mockAgentManager: baseManager}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-1")
	previous, owner, err := svc.beginResumeAttempt(ctx, "task1", "session1")
	require.NoError(t, err)
	require.True(t, owner)
	previous.setExecutionID("exec-1")
	previous.finish(svc.resumeAttemptStore())

	var publications, writes, accepted int
	manager.markPassthroughRunningFunc = func(string) { t.Error("guarded dispatch used unscoped publication") }
	manager.publish = func(publishCtx context.Context, sessionID string) error {
		publications++
		require.True(t, cancelInFlightGuardHeldForSession(publishCtx, sessionID))
		assertPassthroughAdmissionHeld(t, svc, sessionID)
		require.Zero(t, writes, "RUNNING must precede all PTY input")
		svc.handleAgentRunning(publishCtx, watcher.AgentEventData{TaskID: "task1", SessionID: sessionID})
		return nil
	}
	readyDone := make(chan struct{})
	writeErr := errors.New("PTY write failed")
	baseManager.passthroughStdinFunc = func(writeCtx context.Context, sessionID, _ string) error {
		writes++
		assertPassthroughAdmissionHeld(t, svc, sessionID)
		require.False(t, cancelInFlightGuardHeld(writeCtx), "publication capability must not escape into dispatch")
		require.Equal(t, 1, publications)
		if writes == failWrite {
			return writeErr
		}
		if writes == 2 {
			attempt, ok := svc.resumeAttemptStore().current(sessionID)
			require.True(t, ok)
			data := watcher.AgentEventData{TaskID: "task1", SessionID: sessionID, AgentExecutionID: "exec-1", AttemptID: attempt.identity()}
			t.Cleanup(func() { <-readyDone })
			go func() { defer close(readyDone); svc.handleAgentReady(ctx, data) }()
			guard, release := svc.acquireCancelInFlightGuard(sessionID)
			defer release()
			waitForCancelInFlightMutexWaiters(t, guard, 1)
		}
		return nil
	}
	result, err := svc.promptTask(ctx, "task1", "session1", "follow-up", "", false, nil, false, launchOriginManual,
		promptTaskOptions{afterDispatch: func() error { accepted++; return nil }})
	if failWrite != 0 {
		require.ErrorIs(t, err, writeErr)
		require.Zero(t, accepted)
		require.Equal(t, failWrite, writes)
	} else {
		require.NoError(t, err)
		require.Equal(t, "passthrough_dispatched", result.StopReason)
		require.Equal(t, 1, accepted)
		select {
		case <-readyDone:
		case <-time.After(2 * time.Second):
			t.Fatal("immediate READY did not settle after admission released")
		}
		session, loadErr := repo.GetTaskSession(ctx, "session1")
		require.NoError(t, loadErr)
		require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	}
	guard, release := svc.acquireCancelInFlightGuard("session1")
	defer release()
	require.True(t, guard.TryLock(), "success and failure must release admission")
	guard.Unlock()
	require.Equal(t, 1, publications)
}

func assertPassthroughAdmissionHeld(t *testing.T, svc *Service, sessionID string) {
	t.Helper()
	guard, release := svc.acquireCancelInFlightGuard(sessionID)
	defer release()
	if guard.TryLock() {
		guard.Unlock()
		t.Fatal("PTY admission lost its cancellation guard")
	}
}

func TestHandleAgentRunningDoesNotInheritAnotherSessionGuard(t *testing.T) {
	for _, marker := range []string{"none", "unscoped", "other-session"} {
		t.Run(marker, func(t *testing.T) {
			ctx := context.Background()
			switch marker {
			case "unscoped":
				ctx = withCancelInFlightGuardHeld(ctx)
			case "other-session":
				ctx = withCancelInFlightGuardHeldForSession(ctx, "other-session")
			}
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isPassthrough: true})
			guard := svc.lockCancelInFlightGuard("session1")
			done := make(chan struct{})
			t.Cleanup(func() { guard.release(); <-done })
			go func() {
				defer close(done)
				svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "task1", SessionID: "session1"})
			}()
			waitForCancelInFlightMutexWaiters(t, guard.mutex, 1)
			guard.release()
			<-done
		})
	}
}

type passthroughContextCaptureRepository struct {
	*sqliterepo.Repository
	onRead func(context.Context)
}

func (r *passthroughContextCaptureRepository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	r.onRead(ctx)
	return r.Repository.GetTaskSession(ctx, id)
}

func TestHandleAgentRunningConsumesBorrowedOwnership(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{isPassthrough: true})
	guard := svc.lockCancelInFlightGuard("session1")
	defer guard.release()
	var reads int
	svc.repo = &passthroughContextCaptureRepository{Repository: repo, onRead: func(ctx context.Context) {
		reads++
		require.False(t, cancelInFlightGuardHeld(ctx))
		require.Empty(t, ctx.Value(cancelInFlightGuardSessionContextKey{}))
		assertPassthroughAdmissionHeld(t, svc, "session1")
	}}
	svc.handleAgentRunning(withCancelInFlightGuardHeldForSession(context.Background(), "session1"), watcher.AgentEventData{TaskID: "task1", SessionID: "session1"})
	require.Positive(t, reads)
}
