package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	"github.com/stretchr/testify/require"
)

type failedAutoStartResumeManager struct {
	*mockAgentManager
	cleanups atomic.Int32
	t        *testing.T
}

func (m *failedAutoStartResumeManager) CleanupStaleExecutionBySessionID(context.Context, string) error {
	m.cleanups.Add(1)
	m.t.Fatal("busy retry entered terminal-session recovery cleanup")
	return nil
}

func TestAutoStartAgentCallback_BusyRetryDoesNotRecoverFailedSession(t *testing.T) {
	ctx := context.Background()
	repo, svc, manager := setupBusyParentChildrenCompleted(t)
	session, err := repo.GetTaskSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	session.AgentProfileID = "profile-failed-parent"
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	probe := &failedAutoStartResumeManager{mockAgentManager: manager, t: t}
	svc.agentManager = probe
	svc.executor = executor.NewExecutor(probe, repo, testLogger(), executor.ExecutorConfig{})
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
			require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateFailed, "provider stopped"))
			manager.isAgentRunning = false
		}
		return launchErr
	}}
	_, err = cb.Execute(ctx, busyParentAutoStartInput())
	require.Error(t, err)
	require.Zero(t, probe.cleanups.Load(), "a busy retry must not enter terminal-session recovery")
	require.Equal(t, 1, launches)
	session, err = repo.GetTaskSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, session.State)
	require.Empty(t, manager.capturedPrompts)
}
