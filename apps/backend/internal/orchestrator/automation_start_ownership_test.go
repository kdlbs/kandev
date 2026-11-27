package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestAutomationStartOwnershipReleasePreservesSuccessor(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationTask(t, repo, "owned-task", models.TaskOriginAutomationTask, false)
	ctx := context.Background()
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "owned-task", models.MetaKeyAutomationStartOwned, "successor"))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	publisher := &recordingTaskUpdatedPublisher{}
	svc.SetTaskEventPublisher(publisher)
	require.False(t, svc.releaseAutomationStartOwnership(ctx, "owned-task", "original"))
	task, err := repo.GetTask(ctx, "owned-task")
	require.NoError(t, err)
	require.Equal(t, "successor", task.Metadata[models.MetaKeyAutomationStartOwned])
	require.Empty(t, publisher.updatedTaskIDs)
	require.True(t, svc.releaseAutomationStartOwnership(ctx, "owned-task", "successor"))
	require.Equal(t, []string{"owned-task"}, publisher.updatedTaskIDs)
}

func TestStaleAutomationDispatchCannotReconcileSuccessorOwner(t *testing.T) {
	ctx := context.Background()
	base := setupAutomationRetentionFixture(t)
	seedAutomationTask(t, base.repo, "successor-task", models.TaskOriginAutomationTask, false)
	a := &automation.Automation{ID: "successor-auto", WorkspaceID: "ws-successor-task", Name: "Successor", Enabled: true}
	require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
	original := &automation.AutomationRun{ID: "original-run", AutomationID: a.ID, TaskID: "successor-task", Status: automation.RunStatusFailed}
	successor := &automation.AutomationRun{ID: "successor-run", AutomationID: a.ID, TaskID: "successor-task", SessionID: "successor-session", TurnID: "successor-turn", Status: automation.RunStatusTaskCreated}
	require.NoError(t, base.autoStore.CreateRun(ctx, original))
	require.NoError(t, base.autoStore.CreateRun(ctx, successor))
	require.NoError(t, base.repo.SetTaskMetadataKey(ctx, "successor-task", models.MetaKeyAutomationStartOwned, successor.ID))
	svc := createTestService(base.repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(base.autoSvc)
	task, err := base.repo.GetTask(ctx, "successor-task")
	require.NoError(t, err)
	require.False(t, svc.autoStartAutomationTaskForRun(ctx, a, task, "", original.ID, automation.ThreadActionCreated, "stale", true, nil))
	current, err := base.repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, successor.ID, current.Metadata[models.MetaKeyAutomationStartOwned])
}

type ownershipQueueDispatcher struct {
	*automation.Service
}

type ownershipUnreadableSessionsRepository struct {
	*sqliterepo.Repository
}

func (r *ownershipUnreadableSessionsRepository) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return nil, errors.New("session evidence unavailable")
}

func TestDroppedLegacyInitialStartRequiresNoSessionEvidence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		state      models.TaskSessionState
		unreadable bool
		owned      bool
	}{
		{name: "never admitted"},
		{name: "unbound live session", state: models.TaskSessionStateRunning, owned: true},
		{name: "cancelled state without stop proof", state: models.TaskSessionStateCancelled, owned: true},
		{name: "session read uncertainty", unreadable: true, owned: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			base := setupAutomationRetentionFixture(t)
			seedAutomationTask(t, base.repo, "dropped-task", models.TaskOriginAutomationTask, false)
			a := &automation.Automation{ID: "dropped-auto", WorkspaceID: "ws-dropped-task", Name: "Dropped", Enabled: true}
			require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
			run := &automation.AutomationRun{ID: "dropped-run", AutomationID: a.ID, TaskID: "dropped-task", Status: automation.RunStatusFailed}
			require.NoError(t, base.autoStore.CreateRun(ctx, run))
			require.NoError(t, base.repo.SetTaskMetadataKey(ctx, run.TaskID, models.MetaKeyAutomationStartOwned, run.ID))
			if tc.state != "" {
				require.NoError(t, base.repo.CreateTaskSession(ctx, &models.TaskSession{ID: "unbound-session", TaskID: run.TaskID, State: tc.state, AgentExecutionID: "unconfirmed-execution"}))
			}
			svc := createTestService(base.repo, newMockStepGetter(), newMockTaskRepo())
			svc.SetAutomationService(base.autoSvc)
			if tc.unreadable {
				svc.repo = &ownershipUnreadableSessionsRepository{Repository: base.repo}
			}
			task, err := base.repo.GetTask(ctx, run.TaskID)
			require.NoError(t, err)
			svc.releaseDroppedAutomationStartOwnership(ctx, task, run.ID)
			current, err := base.repo.GetTask(ctx, run.TaskID)
			require.NoError(t, err)
			require.Equal(t, tc.owned, models.AutomationStartOwned(current.Metadata))
		})
	}
}

func (s *ownershipQueueDispatcher) DispatchRun(_ context.Context, _ string, _ automation.ThreadAction, _ string, dispatch func() (automation.RunDispatch, error)) error {
	_, err := dispatch()
	return err
}

func TestQueuedAutomationCompletionReconcilesOnlyItsRun(t *testing.T) {
	for _, owner := range []string{"original-run", "successor-run"} {
		t.Run(owner, func(t *testing.T) {
			ctx := context.Background()
			base := setupAutomationRetentionFixture(t)
			seedAutomationTask(t, base.repo, "queued-task", models.TaskOriginAutomationTask, false)
			a := &automation.Automation{ID: "queued-auto", WorkspaceID: "ws-queued-task", Name: "Queued", Enabled: true}
			require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
			for _, id := range []string{"original-run", "successor-run"} {
				require.NoError(t, base.autoStore.CreateRun(ctx, &automation.AutomationRun{ID: id, AutomationID: a.ID, TaskID: "queued-task", SessionID: id + "-session", TurnID: id + "-turn", Status: automation.RunStatusTaskCreated}))
			}
			require.NoError(t, base.repo.SetTaskMetadataKey(ctx, "queued-task", models.MetaKeyAutomationStartOwned, owner))
			svc := createTestService(base.repo, newMockStepGetter(), newMockTaskRepo())
			svc.SetAutomationService(&ownershipQueueDispatcher{Service: base.autoSvc})
			execution := &executor.TaskExecution{TaskID: "queued-task", SessionID: "original-run-session", TurnID: "original-run-turn"}
			actual, err := svc.startQueuedAutomationRun(ctx, "queued-task", &automationRunLaunch{RunID: "original-run"}, func() (*executor.TaskExecution, error) { return execution, nil })
			require.NoError(t, err)
			require.Same(t, execution, actual)
			task, err := base.repo.GetTask(ctx, "queued-task")
			require.NoError(t, err)
			if owner == "original-run" {
				require.False(t, models.AutomationStartOwned(task.Metadata))
			} else {
				require.Equal(t, owner, task.Metadata[models.MetaKeyAutomationStartOwned])
			}
		})
	}
}

func TestAutomationInitialStartRequiresDefinitiveExactEvidence(t *testing.T) {
	for _, state := range []string{automation.RetryLaunchPrepared, automation.RetryLaunchQueued, automation.RetryLaunchDispatching, automation.RetryLaunchAccepted, automation.RetryLaunchUncertain, automation.RetryLaunchStopped} {
		t.Run(state, func(t *testing.T) {
			task := &models.Task{ID: "task", Metadata: map[string]interface{}{models.MetaKeyAutomationStartOwned: "run"}}
			run := &automation.AutomationRun{ID: "run", AutomationID: "auto", TaskID: "task", SessionID: "session", TurnID: "turn", RetryGroupID: "group", RetryGroupGeneration: 2, Status: automation.RunStatusFailed}
			launch := &automation.RetryLaunch{RunID: "run", AutomationID: "auto", TaskID: "task", SessionID: "session", TurnID: "turn", SubmissionID: "submission", Generation: 2, State: state, CancelRequested: true}
			require.Equal(t, state == automation.RetryLaunchStopped, automationInitialStartResolved(task, run, launch))
			launch.Generation++
			require.False(t, automationInitialStartResolved(task, run, launch), "an old generation cannot release its successor")
			launch.Generation--
			launch.TurnID = "other-turn"
			require.False(t, automationInitialStartResolved(task, run, launch))
			launch.TurnID = "turn"
			run.Status = automation.RunStatusTriggered
			require.False(t, automationInitialStartResolved(task, run, launch), "open dispatch remains owned even after lease recovery")
		})
	}
}

func TestStartupReleasesOnlyBoundLegacyInitialStart(t *testing.T) {
	ctx := context.Background()
	base := setupAutomationRetentionFixture(t)
	seedAutomationTask(t, base.repo, "bound-task", models.TaskOriginAutomationTask, false)
	a := &automation.Automation{WorkspaceID: "ws-bound-task", Name: "bound", Enabled: true}
	require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
	run := &automation.AutomationRun{AutomationID: a.ID, TaskID: "bound-task", SessionID: "session", TurnID: "turn", Status: automation.RunStatusTaskCreated}
	require.NoError(t, base.autoStore.CreateRun(ctx, run))
	require.NoError(t, base.repo.SetTaskMetadataKey(ctx, "bound-task", models.MetaKeyAutomationStartOwned, run.ID))
	svc := createTestService(base.repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(base.autoSvc)
	svc.releaseAbandonedAutomationStartOwnership(ctx)
	task, err := base.repo.GetTask(ctx, "bound-task")
	require.NoError(t, err)
	require.False(t, models.AutomationStartOwned(task.Metadata))
}

func TestStartupRetainsRetryOwnerAcrossInstancesAndLeaseRecovery(t *testing.T) {
	ctx := context.Background()
	base := setupAutomationRetentionFixture(t)
	seedAutomationTask(t, base.repo, "retry-owned-task", models.TaskOriginAutomationTask, false)
	a := &automation.Automation{ID: "retry-owner-auto", WorkspaceID: "ws-retry-owned-task", Name: "owned", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
	run := &automation.AutomationRun{ID: "retry-owner-run", AutomationID: a.ID, TaskID: "retry-owned-task", Status: automation.RunStatusTriggered, RetryGroupID: "retry-owner-group", RetryGroupGeneration: 1, RetryState: automation.RetryStateTriggered, RetryLaunchConfigVersion: automation.RetryLaunchConfigVersion}
	run.RetryLaunchConfigSnapshot = retrySnapshotForTest(run, a)
	require.NoError(t, base.autoStore.CreateRetryAdmission(ctx, run, &automation.RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: automation.RetryGroupLive}))
	launch, err := base.autoStore.ReserveRetryLaunch(ctx, run.ID, run.TaskID, "", automation.ThreadActionCreated, "original")
	require.NoError(t, err)
	require.NoError(t, base.repo.SetTaskMetadataKey(ctx, run.TaskID, models.MetaKeyAutomationStartOwned, run.ID))
	for range 2 {
		svc := createTestService(base.repo, newMockStepGetter(), newMockTaskRepo())
		svc.SetAutomationService(base.autoSvc)
		require.NoError(t, base.autoStore.RecoverRetryLedger(ctx, time.Now().Add(24*time.Hour)))
		svc.releaseAbandonedAutomationStartOwnership(ctx)
		task, err := base.repo.GetTask(ctx, run.TaskID)
		require.NoError(t, err)
		require.Equal(t, run.ID, task.Metadata[models.MetaKeyAutomationStartOwned])
		current, err := base.autoStore.GetRetryLaunch(ctx, run.ID)
		require.NoError(t, err)
		require.Equal(t, launch, current, "startup must preserve immutable launch evidence")
	}
}
