package orchestrator

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	"github.com/stretchr/testify/require"
)

func TestAutoStartAgentCallback_RealAcceptedOrUncertainErrorDoesNotReplay(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		name := "accepted disconnect"
		if uncertain {
			name = "uncertain delivery"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo, svc, manager := setupBusyParentChildrenCompleted(t)
			require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
			manager.promptErr = errors.New("agent stream disconnected: EOF")
			manager.promptAcceptedOnError = !uncertain
			if uncertain {
				manager.promptErr = lifecycle.ErrUncertainPromptDelivery
			}
			_, err := (&autoStartAgentCallback{svc: svc}).Execute(ctx, busyParentAutoStartInput())
			require.Error(t, err)
			require.Len(t, manager.capturedPrompts, 1)
			entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestAutoStartAgentCallback_PostQueueDrainHonorsGuardSession(t *testing.T) {
	for _, ownGuard := range []bool{false, true} {
		name := "other session guard"
		heldSessionID := "child-session"
		if ownGuard {
			name, heldSessionID = "parent session guard", busyParentSessionID
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			repo, svc, manager := setupBusyParentChildrenCompleted(t)
			guard := svc.lockCancelInFlightGuard(heldSessionID)
			var once sync.Once
			unlock := func() { once.Do(guard.release) }
			defer unlock()
			ctx = withWorkflowProfileSwitchGuardHeld(withCancelInFlightGuardHeldForSession(ctx, heldSessionID), heldSessionID, "")
			svc.afterBusyAutoStartSessionRead = func() {
				require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
			}
			done := make(chan struct{})
			svc.onQueuedMessageExecutionComplete = func() { close(done) }
			_, err := (&autoStartAgentCallback{svc: svc}).Execute(ctx, busyParentAutoStartInput())
			require.NoError(t, err)
			if ownGuard {
				require.Empty(t, manager.capturedPrompts)
				unlock()
				require.True(t, svc.drainQueuedMessageForPromptableSession(ctx, busyParentSessionID))
			}
			waitForBusyParentWake(t, done)
			requireSingleBusyParentWake(t, manager)
		})
	}
}

func TestAutoStartAgentCallback_RealLaunchDoesNotClaimSuccessorInputs(t *testing.T) {
	ctx := context.Background()
	repo, svc, manager := setupBusyParentChildrenCompleted(t)
	task, err := repo.GetTask(ctx, busyParentID)
	require.NoError(t, err)
	task.Description = "Successor first prompt"
	require.NoError(t, repo.UpdateTask(ctx, task))
	seedHandoffCarryToken(t, repo, busyParentID, "step_coordinate", "Successor handoff", "successor-stamp")
	svc.workflowStepGetter.(*mockStepGetter).steps["step_coordinate"].Prompt = ""
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	cb := &autoStartAgentCallback{svc: svc, launch: func(callCtx context.Context, in engine.ActionInput) error {
		_, replaceErr := repo.DB().ExecContext(ctx, "UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?", "successor-incarnation", busyParentSessionID)
		require.NoError(t, replaceErr)
		_, launchErr := svc.LaunchSession(callCtx, &LaunchSessionRequest{
			TaskID: in.State.TaskID, SessionID: in.State.SessionID,
			Intent: IntentWorkflowStep, WorkflowStepID: in.Step.ID,
		})
		return launchErr
	}}
	_, err = cb.Execute(ctx, busyParentAutoStartInput())
	require.ErrorIs(t, err, messagequeue.ErrSessionIdentityMismatch)
	require.Empty(t, manager.capturedPrompts)
	claimed, err := repo.ClaimInitialPromptFallback(ctx, busyParentSessionID, "successor-incarnation")
	require.NoError(t, err)
	require.True(t, claimed, "the stale launch must not consume the successor's fallback")
	_, present := carryToken(t, repo, busyParentID)
	require.True(t, present, "the stale launch must not consume the successor's handoff")
}

func TestAutoStartAgentCallback_OtherSessionCancelGuardDoesNotStrandWake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo, svc, manager := setupBusyParentChildrenCompleted(t)
	childGuard := svc.lockCancelInFlightGuard("child-session")
	defer childGuard.release()
	ctx = withCancelInFlightGuardHeld(ctx)
	svc.afterBusyAutoStartSessionRead = func() {
		require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	}
	done := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(done) }
	_, err := (&autoStartAgentCallback{svc: svc}).Execute(ctx, busyParentAutoStartInput())
	require.NoError(t, err)
	waitForBusyParentWake(t, done)
	requireSingleBusyParentWake(t, manager)
}

func TestAutoStartAgentCallback_OwnSessionCancelGuardRemainsOwned(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo, svc, manager := setupBusyParentChildrenCompleted(t)
	guard := svc.lockCancelInFlightGuard(busyParentSessionID)
	var once sync.Once
	unlock := func() { once.Do(guard.release) }
	defer unlock()
	ctx = withCancelInFlightGuardHeldForSession(ctx, busyParentSessionID)
	svc.afterBusyAutoStartSessionRead = func() {
		require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	}
	done := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(done) }
	_, err := (&autoStartAgentCallback{svc: svc}).Execute(ctx, busyParentAutoStartInput())
	require.NoError(t, err)
	require.Empty(t, manager.capturedPrompts)
	unlock()
	require.True(t, svc.drainQueuedMessageForPromptableSession(ctx, busyParentSessionID))
	waitForBusyParentWake(t, done)
	requireSingleBusyParentWake(t, manager)
}

func TestAutoStartAgentCallback_ChildGuardCannotAuthorizeParentResumeRegistration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo, svc, _ := setupBusyParentChildrenCompleted(t)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	parentGuard := svc.lockCancelInFlightGuard(busyParentSessionID)
	var releaseOnce sync.Once
	releaseParent := func() { releaseOnce.Do(parentGuard.release) }
	defer releaseParent()
	childGuard := svc.lockCancelInFlightGuard("child-session")
	defer childGuard.release()
	ctx = withCancelInFlightGuardHeldForSession(ctx, "child-session")
	entered := make(chan struct{})
	result := make(chan error, 1)
	workerDone := make(chan struct{})
	cb := &autoStartAgentCallback{svc: svc, launch: func(callCtx context.Context, in engine.ActionInput) error {
		close(entered)
		attempt, _, err := svc.beginResumeAttempt(callCtx, in.State.TaskID, in.State.SessionID)
		if attempt != nil {
			attempt.finish(svc.resumeAttemptStore())
		}
		return err
	}}
	go func() {
		defer close(workerDone)
		_, err := cb.Execute(ctx, busyParentAutoStartInput())
		result <- err
	}()
	defer func() {
		releaseParent()
		cancel()
		select {
		case <-workerDone:
		case <-time.After(5 * time.Second):
			t.Error("resume registration worker did not settle")
		}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("callback did not reach resume registration")
	}
	for {
		select {
		case err := <-result:
			t.Fatalf("parent resume registration bypassed its held guard: %v", err)
		case <-ctx.Done():
			t.Fatal("resume registration did not contend for the parent guard")
		default:
		}
		parentGuard.mutex.stateMu.Lock()
		waiting := len(parentGuard.mutex.waiters) != 0
		parentGuard.mutex.stateMu.Unlock()
		if waiting {
			break
		}
		runtime.Gosched()
	}
	_, registered := svc.resumeAttemptStore().current(busyParentSessionID)
	require.False(t, registered, "child guard ownership must not admit a parent resume")
	releaseParent()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("resume registration did not finish after guard release")
	}
}

func TestAutoStartAgentCallback_LateSessionReloadPreservesSuccessorHandoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo, svc, manager := setupBusyParentChildrenCompleted(t)
	task, err := repo.GetTask(ctx, busyParentID)
	require.NoError(t, err)
	task.Description = "Initial workflow description"
	require.NoError(t, repo.UpdateTask(ctx, task))
	svc.workflowStepGetter.(*mockStepGetter).steps["step_coordinate"].Prompt = ""
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	var once sync.Once
	manager.isAgentReadyFn = func(context.Context, string) bool {
		once.Do(func() {
			guard, guardErr := svc.lockCancelInFlightGuardWithContext(ctx, busyParentSessionID)
			require.NoError(t, guardErr, "prompt input admission must release the guard before provider readiness")
			defer guard.release()
			_, replaceErr := repo.DB().ExecContext(ctx, "UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?", "late-successor-incarnation", busyParentSessionID)
			require.NoError(t, replaceErr)
			_, replaceErr = repo.DB().ExecContext(ctx, "DELETE FROM task_session_prompt_seq WHERE task_session_id = ?", busyParentSessionID)
			require.NoError(t, replaceErr)
			seedHandoffCarryToken(t, repo, busyParentID, "step_coordinate", "Late successor handoff", "late-successor-stamp")
		})
		return true
	}
	_, err = (&autoStartAgentCallback{svc: svc}).Execute(ctx, busyParentAutoStartInput())
	require.ErrorIs(t, err, messagequeue.ErrSessionIdentityMismatch)
	require.Empty(t, manager.capturedPrompts)
	_, present := carryToken(t, repo, busyParentID)
	require.True(t, present, "the post-readiness reload must reject the stale owner before claiming handoff")
	claimed, err := repo.ClaimInitialPromptFallback(ctx, busyParentSessionID, "late-successor-incarnation")
	require.NoError(t, err)
	require.True(t, claimed, "the successor keeps its own first-prompt slot")
}

func TestAutoStartAgentCallback_LateTaskReplacementPreservesInputs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo, svc, manager := setupBusyParentChildrenCompleted(t)
	task, err := repo.GetTask(ctx, busyParentID)
	require.NoError(t, err)
	task.Description = "Initial workflow description"
	require.NoError(t, repo.UpdateTask(ctx, task))
	svc.workflowStepGetter.(*mockStepGetter).steps["step_coordinate"].Prompt = ""
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	const successorTaskID = "replacement-task"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: successorTaskID, WorkspaceID: task.WorkspaceID, Title: "Replacement task"}))
	var once sync.Once
	manager.isAgentReadyFn = func(context.Context, string) bool {
		once.Do(func() {
			guard, guardErr := svc.lockCancelInFlightGuardWithContext(ctx, busyParentSessionID)
			require.NoError(t, guardErr)
			defer guard.release()
			original, loadErr := repo.GetTaskSession(ctx, busyParentSessionID)
			require.NoError(t, loadErr)
			require.NoError(t, repo.DeleteTaskSession(ctx, original))
			for batch := 0; ; batch++ {
				require.Less(t, batch, 20, "retirement must finish before the ID can be reused")
				result, retireErr := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{})
				require.NoError(t, retireErr)
				if !result.Pending {
					break
				}
			}
			original.TaskID, original.QueueIncarnationID = successorTaskID, ""
			require.NoError(t, repo.CreateTaskSession(ctx, original))
			seedHandoffCarryToken(t, repo, busyParentID, "step_coordinate", "Retained original task handoff", "original-task-stamp")
		})
		return true
	}
	_, err = (&autoStartAgentCallback{svc: svc}).Execute(ctx, busyParentAutoStartInput())
	require.Error(t, err)
	require.Empty(t, manager.capturedPrompts)
	_, present := carryToken(t, repo, busyParentID)
	require.True(t, present, "a replaced task binding must not disable admission before the original task handoff claim")
	successor, err := repo.GetTaskSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Equal(t, successorTaskID, successor.TaskID)
	claimed, err := repo.ClaimInitialPromptFallback(ctx, successor.ID, successor.QueueIncarnationID)
	require.NoError(t, err)
	require.True(t, claimed, "the successor keeps its own first-prompt slot")
}

type cancelledWorkflowResumeProbe struct {
	*mockAgentManager
	bindings int
}

func (m *cancelledWorkflowResumeProbe) BindResumeAttempt(context.Context, string, string) error {
	m.bindings++
	return errors.New("cancelled workflow reached runtime resume binding")
}

func TestAutoStartAgentCallback_CancellationBeforeResumeRegistration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo, svc, manager := setupBusyParentChildrenCompleted(t)
	seedMockTaskState(svc.taskRepo.(*mockTaskRepo), busyParentID, "IN_PROGRESS")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	probe := &cancelledWorkflowResumeProbe{mockAgentManager: manager}
	svc.agentManager = probe
	var cancelled sync.Once
	manager.isAgentReadyFn = func(context.Context, string) bool {
		cancelled.Do(func() {
			require.NoError(t, svc.CancelAgent(ctx, busyParentSessionID))
		})
		return false
	}
	_, err := (&autoStartAgentCallback{svc: svc}).Execute(ctx, busyParentAutoStartInput())
	require.Error(t, err)
	require.EqualValues(t, 1, manager.cancelAgentCalls.Load(), "the cancellation must finish between input admission and resume registration")
	require.Zero(t, probe.bindings, "a stale workflow must not bind a runtime resume after cancellation acknowledgement")
	require.Empty(t, manager.capturedPrompts)
	_, active := svc.resumeAttemptStore().current(busyParentSessionID)
	require.False(t, active)
}

func TestAutoStartAgentCallback_CancelledAdmissionCannotRegisterWithHeldGuard(t *testing.T) {
	ctx := context.Background()
	_, svc, _ := setupBusyParentChildrenCompleted(t)
	seedMockTaskState(svc.taskRepo.(*mockTaskRepo), busyParentID, "IN_PROGRESS")
	admissionCtx, err := svc.withWorkflowAutoStartAdmission(ctx, busyParentID, busyParentSessionID)
	require.NoError(t, err)
	require.NoError(t, svc.CancelAgent(ctx, busyParentSessionID))
	guard := svc.lockCancelInFlightGuard(busyParentSessionID)
	defer guard.release()
	admissionCtx = withCancelInFlightGuardHeldForSession(admissionCtx, busyParentSessionID)
	attempt, owner, err := svc.beginResumeAttempt(admissionCtx, busyParentID, busyParentSessionID)
	if attempt != nil {
		attempt.finish(svc.resumeAttemptStore())
	}
	require.Error(t, err)
	require.Nil(t, attempt)
	require.False(t, owner)
}
