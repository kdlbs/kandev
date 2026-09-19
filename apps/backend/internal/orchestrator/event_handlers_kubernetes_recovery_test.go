package orchestrator

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// Tests of workflow outcomes can run the deferred failure work synchronously
// when they do not hold the session guard or publish from a lifecycle callback.
func (s *Service) handleRecoverableFailureLocked(ctx context.Context, data watcher.AgentEventData) {
	if dispatch := s.handleRecoverableFailureLockedState(ctx, data); dispatch != nil {
		dispatch()
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
	synctest.Test(t, func(t *testing.T) {
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
		<-stopEntered
		synctest.Wait()
		select {
		case <-finished:
		default:
			t.Error("failure publisher is blocked by cleanup that may need its lifecycle lock")
		}
		select {
		case <-callback.done:
			t.Error("workflow resume ran before failed execution cleanup finished")
		default:
		}
		unblock()
		<-finished
		synctest.Wait()
		select {
		case <-callback.done:
		default:
			t.Fatal("workflow resume did not run after cleanup")
		}
	})
}
