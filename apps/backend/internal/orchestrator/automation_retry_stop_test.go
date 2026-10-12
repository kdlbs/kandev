package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestRetryStopRunUsesUnprojectedLegacyOperationAndRequiresActualAcknowledgment(t *testing.T) {
	ctx := context.Background()
	base := setupAutomationRetentionFixture(t)
	seedSession(t, base.repo, "legacy-stop-task", "legacy-stop-session", "step1")
	task, err := base.repo.GetTask(ctx, "legacy-stop-task")
	require.NoError(t, err)
	task.Origin = models.TaskOriginAutomationRun
	require.NoError(t, base.repo.UpdateTask(ctx, task))
	session, err := base.repo.GetTaskSession(ctx, "legacy-stop-session")
	require.NoError(t, err)
	session.AgentExecutionID = "legacy-execution"
	require.NoError(t, base.repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, base.repo, session.ID, task.ID, session.AgentExecutionID)
	turns := taskservice.NewService(taskservice.Repos{Tasks: base.repo, TaskRepos: base.repo, Workspaces: base.repo, Workflows: base.repo, Messages: base.repo, Turns: base.repo, Sessions: base.repo, Executors: base.repo, TaskEnvironments: base.repo}, bus.NewMemoryEventBus(testLogger()), testLogger(), taskservice.RepositoryDiscoveryConfig{})
	turn, err := turns.StartTurn(ctx, session.ID)
	require.NoError(t, err)
	a := &automation.Automation{WorkspaceID: task.WorkspaceID, Name: "legacy stop", Enabled: true}
	require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
	group := &automation.RetryGroup{ID: "legacy-stop-group", AutomationID: a.ID, Generation: 1, State: automation.RetryGroupLive}
	require.NoError(t, base.autoStore.CreateRetryGroup(ctx, group))
	run := &automation.AutomationRun{ID: "legacy-stop-run", AutomationID: a.ID, Status: automation.RunStatusTriggered, RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: automation.RetryStateTriggered}
	require.NoError(t, base.autoStore.CreateRun(ctx, run))
	intent := &automation.RetryTaskIntent{ID: "legacy-stop-intent", RunID: run.ID, GroupGeneration: 1, State: "created"}
	require.NoError(t, base.autoStore.CreateRetryIntent(ctx, intent))
	require.NoError(t, base.autoStore.CreateRetryOperation(ctx, &automation.RetryOperation{ID: "legacy-stop-operation", IntentID: intent.ID, RunID: run.ID, GroupGeneration: 1, Kind: "create_task", State: "ambiguous", ExternalTaskID: task.ID, ExternalSessionID: session.ID, ExternalTurnID: turn.ID}))
	var blockState string
	require.NoError(t, base.db.Get(&blockState, `SELECT state FROM session_recovery_blocks WHERE session_id=? AND reason='automation_retry_acceptance_unresolved'`, session.ID))
	require.Equal(t, "open", blockState, "unprojected operation ownership must also fence the legacy warm prompt path")
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, task.ID, v1.TaskStateInProgress)
	manager := &mockAgentManager{repoForExecutionLookup: base.repo}
	var calls atomic.Int32
	stopErr := errors.New("provider termination not acknowledged")
	manager.stopAgentWithReasonFunc = func(_ context.Context, executionID, reason string, force bool) error {
		if executionID != session.AgentExecutionID || reason != coordinatorMCPStopReason || force {
			return errors.New("stop targeted another execution")
		}
		if calls.Add(1) == 1 {
			return stopErr
		}
		return nil
	}
	svc := newCoordinatorStopTestService(base.repo, taskRepo, manager)
	svc.SetTurnService(turns)
	svc.SetAutomationService(base.autoSvc)
	base.autoSvc.SetRunStopper(svc)
	_, err = svc.executor.PrepareSession(ctx, &v1.Task{ID: task.ID, WorkspaceID: task.WorkspaceID, Title: "delayed", Metadata: map[string]interface{}{}}, "profile-1", "", "", "")
	require.Error(t, err, "an old dispatcher without task metadata cannot create an unrelated session")
	_, err = base.autoSvc.StopRun(ctx, a.ID, run.ID)
	require.ErrorIs(t, err, stopErr)
	retained, err := base.autoStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, automation.RunStatusTriggered, retained.Status)
	require.Empty(t, retained.TaskID, "observed operation ownership must not invent acceptance projection")
	require.Empty(t, retained.SessionID)
	require.Empty(t, retained.TurnID)
	require.NotEmpty(t, retained.ErrorMessage)
	require.ErrorIs(t, base.autoStore.DeleteRun(ctx, run.ID), automation.ErrRetryLaunchUncertain)
	_, err = base.autoSvc.StopRun(ctx, a.ID, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
	var receipt struct {
		TaskID  string `db:"task_id"`
		Version int    `db:"version"`
	}
	require.NoError(t, base.db.Get(&receipt, `SELECT task_id,version FROM automation_retry_launch_protocols WHERE run_id=?`, run.ID))
	require.Equal(t, task.ID, receipt.TaskID)
	require.Equal(t, -1, receipt.Version)
	claim, ok := svc.executionTeardownClaimFor(session.ID, session.AgentExecutionID)
	require.True(t, ok)
	require.True(t, claim.cleanupCompleted, "synchronous acknowledged teardown must complete its exact claim")
	require.NoError(t, base.autoStore.DeleteRun(ctx, run.ID))
}
