package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// Tests of workflow outcomes can run the deferred failure work synchronously
// when they do not hold the session guard or publish from a lifecycle callback.
func (s *Service) handleRecoverableFailureLocked(ctx context.Context, data watcher.AgentEventData) {
	if dispatch := s.handleRecoverableFailureLockedState(ctx, data, lifecycle.StopReasonRecoverableAgentFailure); dispatch != nil {
		dispatch(ctx)
	}
}

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.1
func TestKubernetesRecoverableFailurePreservesResume(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-recovery", "session-recovery", "step1")
	stopped := make(chan stopAgentCall, 1)
	svc, _ := newAgentErrorTestService(t, repo, newMockStepGetter(), func(s *Service) {
		manager := s.agentManager.(*mockAgentManager)
		manager.stopAgentWithReasonFunc = func(_ context.Context, id, reason string, force bool) error {
			stopped <- stopAgentCall{ExecutionID: id, Reason: reason, Force: force}
			return nil
		}
	})
	svc.handleRecoverableFailure(context.Background(), watcher.AgentEventData{
		TaskID: "task-recovery", SessionID: "session-recovery", AgentExecutionID: "execution-recovery", ErrorMessage: "Internal error",
	})
	select {
	case call := <-stopped:
		require.Equal(t, "execution-recovery", call.ExecutionID)
		require.Equal(t, "recoverable agent failure", call.Reason)
	case <-time.After(5 * time.Second):
		t.Fatal("recoverable failure did not stop its failed execution")
	}
	session, err := repo.GetTaskSession(context.Background(), "session-recovery")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
}

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.3
func TestKubernetesRecoverableFailureCleanupBeforeWorkflowResume(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	steps := newMockStepGetter()
	steps.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionAutoStartAgent}}},
	}
	stopEntered := make(chan struct{})
	releaseStop := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(releaseStop) }) }
	defer unblock()
	svc, _ := newAgentErrorTestService(t, repo, steps, func(s *Service) {
		s.agentManager.(*mockAgentManager).stopAgentWithReasonFunc = func(context.Context, string, string, bool) error {
			close(stopEntered)
			<-releaseStop
			return nil
		}
	})
	callback := &guardReacquiringAgentErrorCallback{svc: svc, done: make(chan struct{})}
	registry := engine.MapRegistry{engine.ActionAutoStartAgent: callback}
	workflowEngine := engine.New(svc.workflowStore, registry)
	svc.workflowEngine = workflowEngine
	svc.agentErrorDeps.Store(&agentErrorDispatchDeps{engine: workflowEngine, registry: registry, store: svc.workflowStore})
	finished := make(chan struct{})
	go func() {
		svc.handleRecoverableFailure(context.Background(), watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "Internal error",
		})
		close(finished)
	}()
	awaitFailureSignal(t, stopEntered)
	awaitFailureSignal(t, finished)
	// A duplicate event must not dispatch while the first stop still owns cleanup.
	svc.handleRecoverableFailure(context.Background(), watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "Internal error",
	})
	select {
	case <-callback.done:
		t.Error("workflow resume ran before failed execution cleanup finished")
	default:
	}
	unblock()
	waitForFailureRecovery(t, svc)
	select {
	case <-callback.done:
	default:
		t.Fatal("workflow resume did not run after cleanup")
	}
}

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.3
func TestKubernetesRecoverableFailureDoesNotResumeAfterCleanupError(t *testing.T) {
	svc, resumed := recoveryWorkflowFixture(t)
	stopErr := errors.New("runtime stop failed")
	stops := 0
	svc.agentManager.(*mockAgentManager).stopAgentWithReasonFunc = func(context.Context, string, string, bool) error {
		stops++
		return stopErr
	}
	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "Internal error"}
	svc.handleRecoverableFailure(context.Background(), data)
	waitForFailureRecovery(t, svc)
	select {
	case <-resumed:
		t.Fatal("automatic Resume ran after failed cleanup")
	default:
	}
	stopErr = nil
	svc.handleRecoverableFailure(context.Background(), data)
	waitForFailureRecovery(t, svc)
	require.Equal(t, 2, stops, "failed cleanup must remain retryable")
	select {
	case <-resumed:
	default:
		t.Fatal("successful cleanup retry did not dispatch Resume")
	}
}

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.3
func TestKubernetesRecoverableFailureShutdownOwnsCleanup(t *testing.T) {
	svc, resumed := recoveryWorkflowFixture(t)
	entered, release, drained := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	stops := 0
	svc.agentManager.(*mockAgentManager).stopAgentWithReasonFunc = func(context.Context, string, string, bool) error {
		stops++
		if stops == 1 {
			close(entered)
		}
		<-release
		return nil
	}
	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "Internal error"}
	svc.handleRecoverableFailure(context.Background(), data)
	awaitFailureSignal(t, entered)
	go func() { svc.stopDynamicSuccessorWorkers(); close(drained) }()
	awaitFailureSignal(t, svc.dynamicSuccessorCtx.Done())
	select {
	case <-drained:
		t.Error("shutdown returned while recovery cleanup was still running")
	default:
	}
	unblock()
	awaitFailureSignal(t, drained)
	waitForFailureRecovery(t, svc)
	select {
	case <-resumed:
		t.Error("automatic Resume ran after shutdown started")
	default:
	}
	data.AgentExecutionID = "exec-after-shutdown"
	svc.handleRecoverableFailure(context.Background(), data)
	waitForFailureRecovery(t, svc)
	require.Equal(t, 1, stops, "stopped service must reject detached recovery work")
}

// recoveryWorkflowFixture wires a real workflow whose Resume callback signals
// completion after reacquiring the session guard.
func recoveryWorkflowFixture(t *testing.T) (*Service, <-chan struct{}) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	steps := newMockStepGetter()
	steps.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionAutoStartAgent}}},
	}
	svc, _ := newAgentErrorTestService(t, repo, steps, nil)
	callback := &guardReacquiringAgentErrorCallback{svc: svc, done: make(chan struct{})}
	registry := engine.MapRegistry{engine.ActionAutoStartAgent: callback}
	workflowEngine := engine.New(svc.workflowStore, registry)
	svc.workflowEngine = workflowEngine
	svc.agentErrorDeps.Store(&agentErrorDispatchDeps{engine: workflowEngine, registry: registry, store: svc.workflowStore})
	return svc, callback.done
}

// waitForFailureRecovery drains recovery workers after the producer has
// scheduled them, synchronizing subsequent reads of test spies and logs.
func waitForFailureRecovery(t *testing.T, svc *Service) {
	t.Helper()
	done := make(chan struct{})
	go func() { svc.dynamicSuccessorWorkers.Wait(); close(done) }()
	awaitFailureSignal(t, done)
}

// awaitFailureSignal bounds real-time synchronization without simulated time
// around SQLite-backed repository operations.
func awaitFailureSignal(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for failure recovery")
	}
}

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.4
func TestKubernetesRecoverableFailureStartupUsesBootstrapCleanup(t *testing.T) {
	for _, fromResume := range []bool{false, true} {
		for _, failure := range []struct {
			name string
			err  error
		}{
			{"auth", errors.New("authentication required: please log in")},
			{"npm", &routingerr.ManagedRuntimeStartupError{Code: routingerr.CodeManagedRuntimeNpmResolution, Details: "npm error code ETARGET"}},
		} {
			t.Run(fmt.Sprintf("%s/resume=%t", failure.name, fromResume), func(t *testing.T) {
				svc, _ := recoveryWorkflowFixture(t)
				var reason string
				svc.agentManager.(*mockAgentManager).stopAgentWithReasonFunc = func(_ context.Context, _, stopReason string, _ bool) error {
					reason = stopReason
					return nil
				}
				require.True(t, svc.handleAgentStartFailed(context.Background(), "t1", "s1", "exec-1", failure.err, fromResume))
				waitForFailureRecovery(t, svc)
				require.Equal(t, lifecycle.StopReasonAgentBootstrapFailed, reason)
			})
		}
	}
}

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.3
func TestKubernetesRecoverableFailureAlreadyStoppedAllowsRecovery(t *testing.T) {
	svc, resumed := recoveryWorkflowFixture(t)
	svc.agentManager.(*mockAgentManager).stopAgentWithReasonFunc = func(context.Context, string, string, bool) error {
		return fmt.Errorf("execution already removed: %w", lifecycle.ErrExecutionNotFound)
	}
	svc.handleRecoverableFailure(context.Background(), watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "Internal error",
	})
	waitForFailureRecovery(t, svc)
	awaitFailureSignal(t, resumed)
}
