package lifecycle

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-RUNTIME-CLEANUP-001.1, AC-TASKS-RUNTIME-CLEANUP-001.42
func TestStopAgentCheckpointAdmissionHonorsDeadline(t *testing.T) {
	for _, boundary := range []string{"remote lifecycle", "prompt checkpoint", "in-flight capture"} {
		t.Run(boundary, func(t *testing.T) {
			manager, backend := newDetachTestManager(t, false)
			execution := &AgentExecution{
				ID: "stop-deadline", TaskID: "task-stop-deadline", SessionID: "session-stop-deadline",
				TaskEnvironmentID: "environment-stop-deadline", RuntimeName: executor.NameStandalone,
				Status: v1.AgentStatusRunning,
			}
			manager.SetTurnChangeCaptureHandler(&shutdownCheckpointHandler{})
			require.NoError(t, manager.executionStore.Add(execution))
			release, frame := holdStopAdmissionBoundary(execution, boundary)
			var releaseOnce sync.Once
			releaseOwner := func() { releaseOnce.Do(release) }
			t.Cleanup(releaseOwner)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() { done <- manager.StopAgentWithReason(ctx, execution.ID, "task tree deleted", false) }()
			require.Eventually(t, func() bool { return stopAdmissionWaitsIn(frame) },
				lockOrderDeadlockGuard, time.Millisecond, "stop did not reach the held admission boundary")
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				releaseOwner()
				<-done
				t.Fatal("runtime stop ignored cancellation while its admission boundary remained held")
			}
			current, exists := manager.executionStore.Get(execution.ID)
			require.True(t, exists, "an unconfirmed stop must retain its runtime registration")
			require.Same(t, execution, current)
			manager.streamManager.reconciliationMu.Lock()
			_, reconciliationStopped := manager.streamManager.stoppedReconciliations[execution.ID]
			manager.streamManager.reconciliationMu.Unlock()
			require.False(t, reconciliationStopped,
				"an abandoned stop must restore delivery reconciliation for the still-attached execution")
			require.Empty(t, backend.stopInstanceCalls, "timed-out admission cannot destroy the runtime")
			releaseOwner()
			require.NoError(t, manager.StopAgentWithReason(context.Background(), execution.ID, "task tree deleted", false))
			require.Len(t, backend.stopInstanceCalls, 1, "a fresh retry must stop the retained execution")
		})
	}
}

func holdStopAdmissionBoundary(execution *AgentExecution, boundary string) (func(), string) {
	switch boundary {
	case "remote lifecycle":
		execution.remoteInstanceLifecycleMu.Lock()
		return execution.remoteInstanceLifecycleMu.Unlock, "(*Manager).StopAgentWithReason"
	case "prompt checkpoint":
		execution.promptLifecycleMu.Lock()
		return execution.promptLifecycleMu.Unlock, "(*Manager).preserveTurnChangesBeforeStop"
	default:
		execution.promptGeneration = 1
		execution.promptTurnIDs = map[uint64]string{1: "turn-stop-deadline"}
		beginTurnChangeCaptureLocked(execution, 1)
		return func() {
			execution.promptLifecycleMu.Lock()
			execution.promptCompletionGeneration = 1
			finishTurnChangeCaptureLocked(execution, 1)
			execution.promptLifecycleMu.Unlock()
		}, "(*Manager).waitForTurnChangeCaptureBeforeStop"
	}
}

func stopAdmissionWaitsIn(frame string) bool {
	buffer := make([]byte, 1<<20)
	for _, stack := range strings.Split(string(buffer[:runtime.Stack(buffer, true)]), "\n\n") {
		header, _, _ := strings.Cut(stack, "\n")
		if strings.Contains(stack, frame) &&
			(strings.Contains(header, "[sync.") || strings.Contains(header, "[select")) {
			return true
		}
	}
	return false
}

type deadlineStopCaptureHandler struct {
	phase   string
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (*deadlineStopCaptureHandler) AdmitTurnChanges(context.Context, TurnChangeAdmission, TurnChangeCheckpointClient) error {
	return nil
}

func (h *deadlineStopCaptureHandler) FinishTurnChanges(ctx context.Context, _ TurnChangeTerminal, _ TurnChangeCheckpointClient) error {
	if h.phase == "capture" {
		return h.wait(ctx)
	}
	return nil
}

func (h *deadlineStopCaptureHandler) ProcessTurnChanges(ctx context.Context, _ TurnChangeTerminal, _ TurnChangeCheckpointClient) error {
	if h.phase == "summary" {
		return h.wait(ctx)
	}
	return nil
}

func (h *deadlineStopCaptureHandler) wait(ctx context.Context) error {
	h.once.Do(func() { close(h.entered) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.release:
		return nil
	}
}

type failedStopSummaryHandler struct {
	summaryCalls atomic.Int32
}

func (*failedStopSummaryHandler) AdmitTurnChanges(context.Context, TurnChangeAdmission, TurnChangeCheckpointClient) error {
	return nil
}

func (*failedStopSummaryHandler) FinishTurnChanges(context.Context, TurnChangeTerminal, TurnChangeCheckpointClient) error {
	return nil
}

func (h *failedStopSummaryHandler) ProcessTurnChanges(context.Context, TurnChangeTerminal, TurnChangeCheckpointClient) error {
	if h.summaryCalls.Add(1) == 1 {
		return errors.New("summary unavailable")
	}
	return nil
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.1
func TestStopAgentSummaryFailureDoesNotBlockRuntimeStop(t *testing.T) {
	manager, backend := newDetachTestManager(t, false)
	execution := &AgentExecution{
		ID: "stop-summary-failure", TaskID: "task-stop-summary", SessionID: "session-stop-summary",
		TaskEnvironmentID: "environment-stop-summary", RuntimeName: executor.NameStandalone,
		Status: v1.AgentStatusRunning, promptGeneration: 1,
		promptTurnIDs: map[uint64]string{1: "turn-stop-summary"},
	}
	handler := &failedStopSummaryHandler{}
	manager.SetTurnChangeCaptureHandler(handler)
	require.NoError(t, manager.executionStore.Add(execution))

	require.NoError(t, manager.StopAgentWithReason(context.Background(), execution.ID, "task tree deleted", false))
	require.Len(t, backend.stopInstanceCalls, 1, "a derived summary failure must not prevent runtime teardown")
	require.Eventually(t, func() bool { return handler.summaryCalls.Load() >= 2 }, time.Second, time.Millisecond,
		"the failed summary should be retried after runtime stop is admitted")
}

func TestStopAgentSummaryProcessingDoesNotBlockRuntimeStop(t *testing.T) {
	manager, backend := newDetachTestManager(t, false)
	execution := &AgentExecution{
		ID: "stop-summary-delay", TaskID: "task-stop-summary-delay", SessionID: "session-stop-summary-delay",
		TaskEnvironmentID: "environment-stop-summary-delay", RuntimeName: executor.NameStandalone,
		Status: v1.AgentStatusRunning, promptGeneration: 1,
		promptTurnIDs: map[uint64]string{1: "turn-stop-summary-delay"},
	}
	handler := &deadlineStopCaptureHandler{phase: "summary", entered: make(chan struct{}), release: make(chan struct{})}
	manager.SetTurnChangeCaptureHandler(handler)
	require.NoError(t, manager.executionStore.Add(execution))
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(handler.release) }) }
	t.Cleanup(release)

	require.NoError(t, manager.StopAgentWithReason(context.Background(), execution.ID, "task tree deleted", false))
	require.Len(t, backend.stopInstanceCalls, 1, "runtime stop must not wait for derived summary processing")
	select {
	case <-handler.entered:
	case <-time.After(time.Second):
		t.Fatal("summary retry worker did not start")
	}
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.1, AC-TASKS-RUNTIME-CLEANUP-001.42
func TestStopAgentCaptureWorkHonorsDeadline(t *testing.T) {
	for _, phase := range []string{"capture"} {
		t.Run(phase, func(t *testing.T) {
			manager, backend := newDetachTestManager(t, false)
			execution := &AgentExecution{
				ID: "stop-capture-deadline", TaskID: "task-stop-capture", SessionID: "session-stop-capture",
				TaskEnvironmentID: "environment-stop-capture", RuntimeName: executor.NameStandalone,
				Status: v1.AgentStatusRunning, promptGeneration: 1,
				promptTurnIDs: map[uint64]string{1: "turn-stop-capture"},
			}
			require.NoError(t, manager.executionStore.Add(execution))
			handler := &deadlineStopCaptureHandler{phase: phase, entered: make(chan struct{}), release: make(chan struct{})}
			manager.SetTurnChangeCaptureHandler(handler)
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(handler.release) }) }
			t.Cleanup(release)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() { done <- manager.StopAgentWithReason(ctx, execution.ID, "task tree deleted", false) }()
			select {
			case <-handler.entered:
			case err := <-done:
				t.Fatalf("stop returned before the %s phase ran: %v", phase, err)
			case <-time.After(lockOrderDeadlockGuard):
				t.Fatalf("%s phase was not entered within %v", phase, lockOrderDeadlockGuard)
			}
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				release()
				<-done
				t.Fatal("checkpoint work ignored its caller's cancellation")
			}
			_, exists := manager.executionStore.Get(execution.ID)
			require.True(t, exists)
			require.Empty(t, backend.stopInstanceCalls)
			release()
			require.NoError(t, manager.StopAgentWithReason(context.Background(), execution.ID, "task tree deleted", false))
			require.Len(t, backend.stopInstanceCalls, 1)
		})
	}
}

// @covers AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
func TestCancellationPromptIdentityReadHonorsDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager := &Manager{executionStore: NewExecutionStore()}
		execution := &AgentExecution{ID: "identity-deadline", TaskID: "task-identity", SessionID: "session-identity", promptGeneration: 7}
		require.NoError(t, manager.executionStore.Add(execution))
		execution.promptLifecycleMu.Lock()
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(execution.promptLifecycleMu.Unlock) }
		t.Cleanup(release)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		type result struct {
			generation uint64
			err        error
		}
		done := make(chan result, 1)
		go func() {
			generation, err := manager.GetPromptGenerationForSession(ctx, execution.SessionID)
			done <- result{generation: generation, err: err}
		}()
		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		select {
		case observed := <-done:
			require.ErrorIs(t, observed.err, context.DeadlineExceeded)
			require.Zero(t, observed.generation)
		default:
			release()
			<-done
			t.Fatal("cancellation identity capture ignored the operation deadline while the prompt mutex was held")
		}
		release()
		generation, err := manager.GetPromptGenerationForSession(context.Background(), execution.SessionID)
		require.NoError(t, err)
		require.Equal(t, uint64(7), generation)
	})
}
