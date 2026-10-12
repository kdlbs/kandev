package orchestrator

import (
	"context"
	"errors"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"time"
)

type retryLaunchAgentManager struct {
	*mockAgentManager
	configured atomic.Bool
	durable    bool
}

func (m *retryLaunchAgentManager) DurableDeliveryCapabilityForExecution(context.Context, string) (agentruntime.DurableDeliveryCapability, bool) {
	return agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: m.durable}, m.durable
}

func TestAutomationRetryDurableInitialPromptUsesReservedCanonicalSubmission(t *testing.T) {
	assertAutomationRetryInitialStart(t, false, false, false, true)
}

func TestAutomationRetryPreparedInitialPromptUsesExactReservedPayload(t *testing.T) {
	assertAutomationRetryInitialStart(t, false, false, false, true, true)
}

func TestAutomationRetryRevocationDuringPreparationRejectsAdmission(t *testing.T) {
	assertAutomationRetryInitialStart(t, false, false, false, true, false, true)
}

func (m *retryLaunchAgentManager) IsAgentCommandConfigured(string) bool { return m.configured.Load() }

func (m *retryLaunchAgentManager) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	result, err := m.mockAgentManager.LaunchAgent(ctx, req)
	if err == nil {
		m.configured.Store(true)
	}
	return result, err
}

func (m *retryLaunchAgentManager) SetInitialDeliverySubmissionID(_ context.Context, executionID, submissionID string) error {
	if executionID != "owned-execution" || submissionID == "" {
		return errors.New("existing workspace lost reserved delivery identity")
	}
	return nil
}

func TestAutomationRetryNormalStartCarriesReservedIdentityThroughActualEntryPoint(t *testing.T) {
	assertAutomationRetryInitialStart(t, false)
}

func TestAutomationRetryCommandlessWorkspacePromotesBeforeConsumingAdmission(t *testing.T) {
	assertAutomationRetryInitialStart(t, true)
}

func TestAutomationRetryRetainedFailureRequiresCapturedTerminalIdentity(t *testing.T) {
	assertAutomationRetryInitialStart(t, false, true)
}

func TestAutomationRetryCeilingQueueReleaseAndReplayKeepReservedIdentity(t *testing.T) {
	assertAutomationRetryInitialStart(t, false, false, true)
}

func assertRetiredRetryCreationBoundaries(t *testing.T, svc *Service, base *automationRetentionFixture, task *models.Task, run *automation.AutomationRun) {
	t.Helper()
	ctx := context.Background()
	for _, afterDelete := range []bool{false, true} {
		if afterDelete {
			require.NoError(t, base.autoStore.DeleteRun(ctx, run.ID))
		}
		// A delayed old dispatcher has no typed request authority and copies task metadata.
		_, err := svc.executor.PrepareSession(ctx, &v1.Task{ID: task.ID, WorkspaceID: task.WorkspaceID, Metadata: map[string]interface{}{"explicit_manual_creation": true}}, "profile-1", "", "", "")
		require.Error(t, err)
		lateID := map[bool]string{false: "late-after-stop", true: "late-after-history-delete"}[afterDelete]
		seedAutomationTask(t, base.repo, lateID, models.TaskOriginAutomationRun, false)
		late, err := base.repo.GetTask(ctx, lateID)
		require.NoError(t, err)
		late.Metadata = map[string]interface{}{retryAutomationRunIDMetadataKey: run.ID, "explicit_manual_creation": true}
		require.NoError(t, base.repo.UpdateTask(ctx, late))
		_, err = svc.executor.PrepareSession(ctx, &v1.Task{ID: lateID, WorkspaceID: late.WorkspaceID, Metadata: late.Metadata}, "profile-1", "", "", "")
		require.Error(t, err, "late task creation cannot reopen a retired admission")
		// Session opening is automatic. Only an explicit manual prepare request can create a new conversation.
		_, err = svc.launchPrepare(ctx, &LaunchSessionRequest{TaskID: task.ID, AgentProfileID: "profile-1", NoAgentLaunch: true, ActivationSource: LaunchActivationSourceSessionOpen})
		require.Error(t, err)
		prepared, err := svc.launchPrepare(ctx, &LaunchSessionRequest{TaskID: task.ID, AgentProfileID: "profile-1", NoAgentLaunch: true})
		require.NoError(t, err)
		manualCtx := models.WithExplicitManualCreation(ctx, task.ID)
		turn, err := svc.turnService.StartTurn(manualCtx, prepared.SessionID)
		require.NoError(t, err)
		require.Equal(t, task.ID, turn.TaskID)
		var permits int
		require.NoError(t, base.db.Get(&permits, `SELECT COUNT(*) FROM task_creation_authorizations`))
		require.Zero(t, permits, "manual permission must be consumed within the transaction")
		_, err = svc.executor.PrepareSession(ctx, &v1.Task{ID: task.ID, WorkspaceID: task.WorkspaceID}, "profile-1", "", "", "")
		require.Error(t, err, "manual creation cannot authorize a subsequent older dispatcher")
	}
}

func TestLegacyAutomationRetryRejectsUnreservedOlderPersistenceEntrypoints(t *testing.T) {
	ctx := context.Background()
	base := setupAutomationRetentionFixture(t)
	taskID := "legacy-bound-task"
	seedAutomationTask(t, base.repo, taskID, models.TaskOriginAutomationRun, false)
	now := time.Now().UTC()
	session := &models.TaskSession{ID: "legacy-existing-session", TaskID: taskID, State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}
	require.NoError(t, base.repo.CreateTaskSession(ctx, session))
	a := &automation.Automation{ID: "legacy-automation", WorkspaceID: "ws-" + taskID, Name: "legacy", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
	// CreateRun mirrors pre-protocol admission. It deliberately has no launch-version evidence.
	run := &automation.AutomationRun{ID: "legacy-run", AutomationID: a.ID, TaskID: taskID, Status: automation.RunStatusTriggered, RetryGroupID: "legacy-group", RetryGroupGeneration: 1, RetryState: automation.RetryStateTriggered}
	require.NoError(t, base.autoStore.CreateRetryGroup(ctx, &automation.RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: automation.RetryGroupLive}))
	require.NoError(t, base.autoStore.CreateRun(ctx, run))
	var effects atomic.Int32
	manager := &mockAgentManager{launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		effects.Add(1)
		return nil, errors.New("unexpected provider effect")
	}}
	svc := createTestServiceWithScheduler(base.repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.SetAutomationService(base.autoSvc)
	// An older dispatcher prepares a fresh UUID session without a reserved context.
	_, err := svc.executor.PrepareSession(ctx, &v1.Task{ID: taskID, WorkspaceID: a.WorkspaceID}, "profile-1", "", "", "")
	require.Error(t, err)
	turns := taskservice.NewService(taskservice.Repos{Tasks: base.repo, TaskRepos: base.repo, Workspaces: base.repo, Workflows: base.repo, Messages: base.repo, Turns: base.repo, Sessions: base.repo}, bus.NewMemoryEventBus(testLogger()), testLogger(), taskservice.RepositoryDiscoveryConfig{})
	// Its existing-session continuation also creates a fresh unreserved turn.
	_, err = turns.StartTurn(ctx, session.ID)
	require.Error(t, err)
	require.EqualValues(t, 0, effects.Load())
	_, err = base.autoStore.ReserveRetryLaunch(ctx, run.ID, taskID, "", automation.ThreadActionCreated, "")
	require.ErrorIs(t, err, automation.ErrRetryLaunchUncertain)
	retained, err := base.autoStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, automation.RunStatusTaskCreated, retained.Status)
	require.Empty(t, retained.SessionID)
	require.Empty(t, retained.TurnID)
	require.Error(t, base.autoStore.MarkRunTerminal(ctx, run.ID, "", "", automation.RunStatusFailed, "missing local execution"))
}

func TestRetryTaskCannotStartBeforeItsRunBindingCommits(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "reserved protocol", true: "legacy protocol"}[legacy], func(t *testing.T) {
			ctx := context.Background()
			base := setupAutomationRetentionFixture(t)
			taskID := "unbound-retry-task"
			seedAutomationTask(t, base.repo, taskID, models.TaskOriginAutomationRun, false)
			task, err := base.repo.GetTask(ctx, taskID)
			require.NoError(t, err)
			if task.Metadata == nil {
				task.Metadata = map[string]interface{}{}
			}
			task.Metadata[retryAutomationRunIDMetadataKey] = "unbound-run"
			require.NoError(t, base.repo.UpdateTask(ctx, task))
			a := &automation.Automation{ID: "unbound-automation", WorkspaceID: task.WorkspaceID, Name: "unbound", Enabled: true, MaxConcurrentRuns: 1}
			require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
			run := &automation.AutomationRun{ID: "unbound-run", AutomationID: a.ID, Status: automation.RunStatusTriggered, RetryGroupID: "unbound-group", RetryGroupGeneration: 1, RetryState: automation.RetryStateTriggered}
			group := &automation.RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: automation.RetryGroupLive}
			if legacy {
				require.NoError(t, base.autoStore.CreateRetryGroup(ctx, group))
				require.NoError(t, base.autoStore.CreateRun(ctx, run))
			} else {
				require.NoError(t, base.autoStore.CreateRetryAdmission(ctx, run, group))
			}
			var effects atomic.Int32
			manager := &mockAgentManager{launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				effects.Add(1)
				return nil, errors.New("unexpected provider effect")
			}}
			svc := createTestServiceWithScheduler(base.repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.SetAutomationService(base.autoSvc)
			_, err = svc.StartTask(ctx, taskID, "profile-1", "", "", "", "retry prompt", "", false, true, nil)
			require.ErrorIs(t, err, automation.ErrRetryLaunchUncertain)
			// The legacy preparation boundary is independently fenced in persistence.
			_, err = svc.executor.PrepareSession(ctx, &v1.Task{ID: taskID, WorkspaceID: task.WorkspaceID}, "profile-1", "", "", "")
			require.Error(t, err)
			require.EqualValues(t, 0, effects.Load())
			stored, err := base.autoStore.GetRun(ctx, run.ID)
			require.NoError(t, err)
			require.Empty(t, stored.TaskID)
			require.Empty(t, stored.SessionID)
			require.Empty(t, stored.TurnID)
			ordinaryID := "ordinary-task"
			seedAutomationTask(t, base.repo, ordinaryID, models.TaskOriginManual, false)
			require.False(t, svc.automationOwnsRetrySession(ctx, ordinaryID, ""))
		})
	}
}

func TestLegacyWarmContinuationIsQuarantinedBeforeReusingAnExistingTurn(t *testing.T) {
	ctx := context.Background()
	base := setupAutomationRetentionFixture(t)
	taskID := "legacy-warm-task"
	seedAutomationTask(t, base.repo, taskID, models.TaskOriginAutomationRun, false)
	now := time.Now().UTC()
	session := &models.TaskSession{ID: "legacy-warm-session", TaskID: taskID, State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}
	require.NoError(t, base.repo.CreateTaskSession(ctx, session))
	turns := taskservice.NewService(taskservice.Repos{Tasks: base.repo, TaskRepos: base.repo, Workspaces: base.repo, Workflows: base.repo, Messages: base.repo, Turns: base.repo, Sessions: base.repo}, bus.NewMemoryEventBus(testLogger()), testLogger(), taskservice.RepositoryDiscoveryConfig{})
	turn, err := turns.StartTurn(ctx, session.ID)
	require.NoError(t, err)
	a := &automation.Automation{ID: "legacy-warm-automation", WorkspaceID: "ws-" + taskID, Name: "legacy warm", Enabled: true}
	require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
	run := &automation.AutomationRun{ID: "legacy-warm-run", AutomationID: a.ID, TaskID: taskID, SessionID: session.ID, TurnID: turn.ID, Status: automation.RunStatusTriggered, RetryGroupID: "legacy-warm-group", RetryGroupGeneration: 1}
	require.NoError(t, base.autoStore.CreateRun(ctx, run))
	base.svc.SetTurnService(turns)
	block, err := base.svc.GetOpenSessionRecoveryBlock(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, block)
	require.Equal(t, "automation_retry_acceptance_unresolved", block.Reason)
	require.Empty(t, block.DeliverySubmissionID)
	require.Empty(t, block.DeliveryTurnID)
	require.Empty(t, block.DeliveryOutcome)
	// This shared prompt admission path predates the retry launch protocol.
	// It can reuse the existing turn, so INSERT guards cannot enforce this boundary.
	_, _, _, _, _, _, err = base.svc.claimSessionRunningForPrompt(ctx, taskID, session.ID, "", false, nil, nil, "", false)
	require.True(t, isSessionRecoveryRequiredError(err), "older warm admission must stop at the recovery block: %v", err)
	active, err := base.repo.GetActiveTurnBySessionID(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, turn.ID, active.ID)
	// Schema replay/backfill retains the same canonical block without fabricating evidence.
	_, err = automation.NewStore(base.db, base.db)
	require.NoError(t, err)
	replay, err := base.svc.GetOpenSessionRecoveryBlock(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, block.ID, replay.ID)
	ordinaryID := "ordinary-warm-task"
	seedAutomationTask(t, base.repo, ordinaryID, models.TaskOriginManual, false)
	ordinary := &models.TaskSession{ID: "ordinary-warm-session", TaskID: ordinaryID, State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}
	require.NoError(t, base.repo.CreateTaskSession(ctx, ordinary))
	require.NoError(t, base.svc.checkSessionRecoveryBlock(ctx, ordinary.ID))
}

func TestRetryTwoServicesDoNotReclaimBlockedLaunchAfterLeaseExpiryOrStop(t *testing.T) {
	ctx := context.Background()
	base := setupAutomationRetentionFixture(t)
	taskID := "blocked-retry-task"
	seedAutomationTask(t, base.repo, taskID, models.TaskOriginAutomationRun, false)
	task, err := base.repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.Description = "retry prompt"
	require.NoError(t, base.repo.UpdateTask(ctx, task))
	a := &automation.Automation{ID: "blocked-automation", WorkspaceID: task.WorkspaceID, Name: "blocked", Enabled: true, AgentProfileID: "profile-1", MaxConcurrentRuns: 1}
	require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
	run := &automation.AutomationRun{ID: "blocked-run", AutomationID: a.ID, TaskID: task.ID, Status: automation.RunStatusTriggered, RetryGroupID: "blocked-group", RetryGroupGeneration: 1, RetryState: automation.RetryStateTriggered, RetryLaunchConfigVersion: automation.RetryLaunchConfigVersion}
	run.RetryLaunchConfigSnapshot = retrySnapshotForTest(run, a)
	require.NoError(t, base.autoStore.CreateRetryAdmission(ctx, run, &automation.RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: automation.RetryGroupLive}))
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var providerStarts, workspaceLaunches atomic.Int32
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "", lifecycle.ErrNoExecutionForSession },
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if workspaceLaunches.Add(1) != 1 {
				return nil, errors.New("second service reached the workspace launch")
			}
			if req.InitialDeliverySubmissionID == "" {
				return nil, errors.New("missing reserved submission")
			}
			close(entered)
			<-release
			return &executor.LaunchAgentResponse{AgentExecutionID: "blocked-execution"}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			providerStarts.Add(1)
			return errors.New("blocked launch crossed admission")
		},
	}
	taskStates := newMockTaskRepo()
	seedMockTaskState(taskStates, taskID, v1.TaskStateInProgress)
	first := createTestServiceWithScheduler(base.repo, newMockStepGetter(), taskStates, manager)
	first.executor.SetOnSessionStateTransition(first.transitionTaskSessionState)
	first.SetTurnService(taskservice.NewService(taskservice.Repos{Tasks: base.repo, TaskRepos: base.repo, Workspaces: base.repo, Workflows: base.repo, Messages: base.repo, Turns: base.repo, Sessions: base.repo, Executors: base.repo, TaskEnvironments: base.repo}, bus.NewMemoryEventBus(testLogger()), testLogger(), taskservice.RepositoryDiscoveryConfig{}))
	first.SetAutomationService(base.autoSvc)
	go func() {
		defer close(done)
		first.autoStartAutomationTaskForRun(ctx, a, task, "", run.ID, automation.ThreadActionCreated, "original", true, nil)
	}()
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("blocked dispatcher was not joined")
		}
	})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("synthetic workspace boundary not reached")
	}
	before, err := base.autoStore.GetRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, automation.RetryLaunchPreparing, before.State)
	otherStore, err := automation.NewStore(base.db, base.db)
	require.NoError(t, err)
	otherAuto := automation.NewService(otherStore, nil, testLogger())
	second := createTestServiceWithScheduler(base.repo, newMockStepGetter(), taskStates, manager)
	second.SetAutomationService(otherAuto)
	// The durable claim, rather than either service's process-local mutex, excludes this dispatcher.
	second.autoStartAutomationTaskForRun(ctx, a, task, "", run.ID, automation.ThreadActionCreated, "changed", true, nil)
	require.EqualValues(t, 1, workspaceLaunches.Load())
	require.NoError(t, otherStore.RecoverRetryLedger(ctx, time.Now().Add(24*time.Hour)))
	second.autoStartAutomationTaskForRun(ctx, a, task, "", run.ID, automation.ThreadActionCreated, "changed", true, nil)
	require.EqualValues(t, 1, workspaceLaunches.Load())
	otherAuto.SetRunStopper(first)
	_, err = otherAuto.StopRun(ctx, a.ID, run.ID)
	require.Error(t, err, "no known execution acknowledgment can settle the blocked launch")
	_, err = otherStore.DeleteAutomationWithCleanup(ctx, a.ID)
	require.ErrorIs(t, err, automation.ErrRetryLaunchUncertain)
	close(release)
	released = true
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked dispatcher did not return after release")
	}
	require.Zero(t, providerStarts.Load(), "revoked admission must fail before provider start")
	after, err := otherStore.GetRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, automation.RetryLaunchUncertain, after.State)
	require.True(t, after.CancelRequested)
	require.Equal(t, before.SessionID, after.SessionID)
	require.Equal(t, before.TurnID, after.TurnID)
	require.Equal(t, before.SubmissionID, after.SubmissionID)
	_, err = otherStore.ClaimRetryLaunch(ctx, run.ID)
	require.ErrorIs(t, err, automation.ErrRetryLaunchUncertain)
}
