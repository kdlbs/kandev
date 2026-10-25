package orchestrator

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestAutoStartAgentCallback_PreservesFallbackAfterRealBusyRejection(t *testing.T) {
	for _, queue := range []bool{false, true} {
		name := "retry"
		if queue {
			name = "queue"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo, svc, manager := setupBusyParentChildrenCompleted(t)
			task, err := repo.GetTask(ctx, busyParentID)
			require.NoError(t, err)
			task.Description = "Initial task description owed to the parent"
			if queue {
				svc.messageQueue = messagequeue.NewService(newWorkflowStartPromptSQLiteRepository(t, repo), messagequeue.DefaultMaxPerSession, testLogger())
				_, err = svc.messageQueue.QueueMessage(ctx, busyParentSessionID, busyParentID, "Previously queued instructions", "", messagequeue.QueuedByUser, false, nil)
				require.NoError(t, err)
			}
			require.NoError(t, repo.UpdateTask(ctx, task))
			seedHandoffCarryToken(t, repo, busyParentID, "step_coordinate", "Retained handoff notes", "busy-consumer-stamp")

			steps := svc.workflowStepGetter.(*mockStepGetter)
			steps.steps["step_coordinate"].Prompt = ""
			require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
			launches := 0
			cb := &autoStartAgentCallback{svc: svc, launch: func(callCtx context.Context, in engine.ActionInput) error {
				launches++
				if launches == 1 {
					require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateRunning, ""))
				}
				_, launchErr := svc.LaunchSession(callCtx, &LaunchSessionRequest{
					TaskID: in.State.TaskID, SessionID: in.State.SessionID,
					Intent: IntentWorkflowStep, WorkflowStepID: in.Step.ID,
				})
				if launches == 1 {
					require.ErrorIs(t, launchErr, ErrAgentPromptInProgress)
					if !queue {
						require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
					}
				}
				return launchErr
			}}
			_, err = cb.Execute(ctx, busyParentAutoStartInput())
			require.NoError(t, err)
			if queue {
				entries, _, snapshotErr := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
				require.NoError(t, snapshotErr)
				require.Len(t, entries, 1, "the claimed fallback must remain owed after rejection")
				require.Contains(t, entries[0].Content, task.Description)
				require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
				done := make(chan struct{})
				svc.onQueuedMessageExecutionComplete = func() { close(done) }
				require.True(t, svc.drainQueuedMessageForPromptableSession(ctx, busyParentSessionID))
				waitForBusyParentWake(t, done)
			}
			require.Len(t, manager.capturedPrompts, 1)
			require.Equal(t, 1, strings.Count(manager.capturedPrompts[0], task.Description))
			require.Equal(t, 1, strings.Count(manager.capturedPrompts[0], "Retained handoff notes"))
			if queue {
				require.Contains(t, manager.capturedPrompts[0], "Previously queued instructions")
			}
			require.NoError(t, svc.StartSessionForWorkflowStep(ctx, busyParentID, busyParentSessionID, "step_coordinate"))
			require.Len(t, manager.capturedPrompts, 1, "the fallback and handoff remain single-use outside this action")
		})
	}
}

func TestAutoStartAgentCallback_DoesNotRetryAfterCancellation(t *testing.T) {
	ctx := context.Background()
	repo, svc, manager := setupBusyParentChildrenCompleted(t)
	seedMockTaskState(svc.taskRepo.(*mockTaskRepo), busyParentID, v1.TaskStateInProgress)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	launches := 0
	cb := &autoStartAgentCallback{svc: svc, launch: func(callCtx context.Context, in engine.ActionInput) error {
		launches++
		if launches == 1 {
			require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateRunning, ""))
		}
		_, err := svc.LaunchSession(callCtx, &LaunchSessionRequest{
			TaskID: in.State.TaskID, SessionID: in.State.SessionID,
			Intent: IntentWorkflowStep, WorkflowStepID: in.Step.ID,
		})
		if launches == 1 {
			require.ErrorIs(t, err, ErrAgentPromptInProgress)
			require.NoError(t, svc.CancelAgent(ctx, busyParentSessionID))
		}
		return err
	}}
	_, err := cb.Execute(ctx, busyParentAutoStartInput())
	require.Error(t, err, "an acknowledged cancellation must retire this wake")
	require.Equal(t, 1, launches)
	require.Empty(t, manager.capturedPrompts)
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestAutoStartAgentCallback_BusyPreservesLaunchEligibility(t *testing.T) {
	for _, kind := range []string{"profile mismatch", "pending approval", "prompt permission"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			repo, svc, manager := setupBusyParentChildrenCompleted(t)
			session, err := repo.GetTaskSession(ctx, busyParentSessionID)
			require.NoError(t, err)
			switch kind {
			case "profile mismatch":
				session.AgentProfileID = "profile-a"
				svc.workflowStepGetter.(*mockStepGetter).steps["step_coordinate"].AgentProfileID = "profile-b"
			case "prompt permission":
				svc.SetTaskPromptChecker(func(context.Context, string) error { return errors.New("prompt permission denied") })
			default:
				session.ReviewStatus = models.ReviewStatusPending
			}
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			_, err = (&autoStartAgentCallback{svc: svc}).Execute(ctx, busyParentAutoStartInput())
			require.ErrorContains(t, err, kind)
			entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
			require.NoError(t, err)
			require.Empty(t, entries, "busy admission must retain direct-launch eligibility checks")
			require.Empty(t, manager.capturedPrompts)
		})
	}
}

func TestAutoStartAgentCallback_BusyQueueRejectsSupersededSession(t *testing.T) {
	for _, kind := range []string{"cancelled", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			repo, svc, manager := setupBusyParentChildrenCompleted(t)
			seedMockTaskState(svc.taskRepo.(*mockTaskRepo), busyParentID, v1.TaskStateInProgress)
			svc.afterBusyAutoStartSessionRead = func() {
				if kind == "cancelled" {
					require.NoError(t, svc.CancelAgent(ctx, busyParentSessionID))
				} else {
					_, err := repo.DB().ExecContext(ctx, "UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?", "replacement-incarnation", busyParentSessionID)
					require.NoError(t, err)
				}
			}
			_, err := (&autoStartAgentCallback{svc: svc}).Execute(ctx, busyParentAutoStartInput())
			require.Error(t, err, "a stale workflow action must not enqueue against the successor")
			entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
			require.NoError(t, err)
			require.Empty(t, entries)
			require.Empty(t, manager.capturedPrompts)
		})
	}
}

func TestProcessOnChildrenCompleted_ConcurrentBusyWakeUsesOneSQLiteEntry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo, svc, manager := setupBusyParentChildrenCompleted(t)
	svc.messageQueue = messagequeue.NewService(newWorkflowStartPromptSQLiteRepository(t, repo), messagequeue.DefaultMaxPerSession, testLogger())
	entered := make(chan struct{})
	release := make(chan struct{})
	var once, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	svc.afterBusyAutoStartSessionRead = func() { once.Do(func() { close(entered); <-release }) }
	done := make(chan struct{}, 2)
	run := func() { svc.processOnChildrenCompleted(ctx, busyParentID); done <- struct{}{} }
	go run()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first completion did not reach queue admission")
	}
	go run()
	unblock()
	for range 2 {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("completion operation did not settle")
		}
	}
	requireBusyParentCompletionApplied(t, repo, svc)
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	dispatched := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(dispatched) }
	require.True(t, svc.drainQueuedMessageForPromptableSession(ctx, busyParentSessionID))
	waitForBusyParentWake(t, dispatched)
	requireSingleBusyParentWake(t, manager)
	entries, _, err = svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Empty(t, entries)
}
