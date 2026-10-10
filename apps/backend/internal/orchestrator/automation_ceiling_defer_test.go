package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	deferredAutomationOccupierSession = "occupier-session"
	deferredAutomationThreadReason    = "new task created for automation run"
)

// deferredAutomationStartFixture drives a real automation service and store
// through a start refused by the session ceiling: the run is admitted and owns
// its task, another launch holds the ceiling's only slot, and the automation
// start is refused.
type deferredAutomationStartFixture struct {
	svc       *Service
	repo      *sqliterepo.Repository
	db        *sqlx.DB
	autoSvc   *automation.Service
	autoStore *automation.Store
	auto      *automation.Automation
	taskID    string
	runID     string
	launches  int
	launchErr error
}

func deferAutomationStartAtCeiling(t *testing.T, taskID string) *deferredAutomationStartFixture {
	t.Helper()
	return deferAutomationStartAtCeilingOnStep(t, taskID, nil)
}

// deferAutomationStartAtCeilingOnStep is deferAutomationStartAtCeiling for a
// task that sits on step, which the automation start names. A nil step leaves
// the task outside any workflow.
func deferAutomationStartAtCeilingOnStep(
	t *testing.T, taskID string, step *wfmodels.WorkflowStep,
) *deferredAutomationStartFixture {
	t.Helper()
	ctx := context.Background()
	base := setupAutomationRetentionFixture(t)
	seedAutomationTask(t, base.repo, taskID, models.TaskOriginAutomationRun, false)
	stepGetter := newMockStepGetter()
	workflowStepID := ""
	if step != nil {
		stepGetter.steps[step.ID] = step
		workflowStepID = step.ID
		task, err := base.repo.GetTask(ctx, taskID)
		require.NoError(t, err)
		task.WorkflowID, task.WorkflowStepID = step.WorkflowID, step.ID
		require.NoError(t, base.repo.UpdateTask(ctx, task))
	}

	f := &deferredAutomationStartFixture{
		repo: base.repo, db: base.db, autoSvc: base.autoSvc, autoStore: base.autoStore, taskID: taskID,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			if f.launches == 0 {
				return "", lifecycle.ErrNoExecutionForSession
			}
			return "exec-" + taskID, nil
		},
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			f.launches++
			if f.launchErr != nil {
				return nil, f.launchErr
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-" + taskID}, nil
		},
	}
	f.svc = createTestServiceWithScheduler(base.repo, stepGetter, taskRepo, agentMgr)
	// As in production, session state moves only through the guarded
	// transition, so a stop is not overwritten by the process-start callback.
	f.svc.executor.SetOnSessionStateTransition(f.svc.transitionTaskSessionState)
	f.svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	f.svc.turnService = &repoTurnService{repo: base.repo}
	f.svc.SetTaskLifecycleDeleter(taskLifecycleDeleterFunc(func(ctx context.Context, id string) error {
		return base.repo.DeleteTask(ctx, id)
	}))
	f.svc.SetAutomationService(base.autoSvc)

	f.auto = &automation.Automation{
		WorkspaceID: "ws-" + taskID, Name: "watch", Enabled: true,
		AgentProfileID: "profile-1", MaxConcurrentRuns: 1,
	}
	require.NoError(t, base.autoStore.CreateAutomation(ctx, f.auto))
	run := &automation.AutomationRun{
		AutomationID: f.auto.ID, TriggerType: automation.TriggerTypeScheduled,
		TaskID: taskID, Status: automation.RunStatusTriggered, TriggerData: json.RawMessage(`{}`),
	}
	require.NoError(t, base.autoStore.CreateRun(ctx, run))
	f.runID = run.ID

	require.True(t, f.svc.sessionCeiling.admit(ctx, admissionRequest{
		taskID: "occupier", sessionID: deferredAutomationOccupierSession, origin: launchOriginAutomatic, seam: "test-setup",
	}).admitted)
	f.svc.autoStartAutomationTaskForRun(ctx, f.auto, &models.Task{ID: taskID, Description: "sweep"}, workflowStepID,
		run.ID, automation.ThreadActionCreated, deferredAutomationThreadReason, false, nil)

	deferred := f.run(t)
	surviving, taskErr := base.repo.GetTask(ctx, taskID)
	require.True(t, taskErr == nil && surviving != nil && deferred.Status == automation.RunStatusTriggered,
		"a ceiling-deferred start must keep its task and leave its run open; task lookup error: %v, run status: %s (%s)",
		taskErr, deferred.Status, deferred.ErrorMessage)
	require.True(t, f.ceilingDeferred(t), "the refused automation start must stay recorded for replay")
	require.Empty(t, deferred.SessionID, "no session exists while the start is deferred")
	require.Equal(t, 1, f.activeRuns(t), "the deferred run keeps its max_concurrent_runs slot")
	require.Zero(t, f.launches)
	return f
}

func (f *deferredAutomationStartFixture) run(t *testing.T) *automation.AutomationRun {
	t.Helper()
	run, err := f.autoStore.GetRun(context.Background(), f.runID)
	require.NoError(t, err)
	require.NotNil(t, run)
	return run
}

func (f *deferredAutomationStartFixture) ceilingDeferred(t *testing.T) bool {
	t.Helper()
	return models.HasCeilingDeferredIntent(&models.Task{Metadata: map[string]interface{}{
		models.MetaKeyDeferredLaunch: deferredLaunchOf(t, f.svc, f.taskID),
	}})
}

func (f *deferredAutomationStartFixture) activeRuns(t *testing.T) int {
	t.Helper()
	active, err := f.autoStore.CountActiveRuns(context.Background(), f.auto.ID)
	require.NoError(t, err)
	return active
}

func (f *deferredAutomationStartFixture) freeCeilingAndSweep(ctx context.Context) {
	f.svc.sessionCeiling.release(deferredAutomationOccupierSession)
	f.svc.drainDeferredCeilingLaunches(ctx)
}

// A ceiling refusal is not a failed firing: the task and its deferred record
// survive, and the replayed start is bound to the run, so the turn it launched
// settles the run and frees its slot. The task sits on no auto-start workflow
// step, as automation tasks usually do.
func TestAutomationStartDeferredByCeiling_ReplayBindsAndSettlesTheRun(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "t-defer-bind")

	f.svc.drainDeferredCeilingLaunches(ctx)
	require.True(t, f.ceilingDeferred(t), "a still-refused replay keeps the record")
	require.Equal(t, automation.RunStatusTriggered, f.run(t).Status)
	require.Zero(t, f.launches)

	f.freeCeilingAndSweep(ctx)

	bound := f.run(t)
	require.False(t, f.ceilingDeferred(t), "a dispatched replay clears the record")
	require.Equal(t, 1, f.launches)
	require.Equal(t, automation.RunStatusTaskCreated, bound.Status)
	require.NotEmpty(t, bound.SessionID, "the replay binds the session it launched to the run")
	require.NotEmpty(t, bound.TurnID, "the replay binds the turn it launched to the run")
	require.Equal(t, automation.ThreadActionCreated, bound.ThreadAction)
	require.Equal(t, deferredAutomationThreadReason, bound.ThreadReason)

	session, err := f.svc.repo.GetTaskSession(ctx, bound.SessionID)
	require.NoError(t, err)
	require.Equal(t, f.taskID, session.TaskID)
	require.True(t, f.svc.handleAutomationTurnCompleteForTurn(
		ctx, f.taskID, bound.SessionID, session, bound.TurnID, "end_turn", false, ""))

	require.Equal(t, automation.RunStatusSucceeded, f.run(t).Status,
		"the bound turn's completion settles the run")
	require.Zero(t, f.activeRuns(t), "a settled run releases its max_concurrent_runs slot")
}

// Stopping a run whose start is still queued closes it; the queued start is
// then dropped instead of launching an agent for a run nothing tracks.
func TestAutomationStartDeferredByCeiling_StoppedRunIsNotLaunched(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "t-defer-stop")

	stopped, err := f.autoSvc.StopRun(ctx, f.auto.ID, f.runID)
	require.NoError(t, err)
	require.Equal(t, automation.RunStatusFailed, stopped.Status)

	f.freeCeilingAndSweep(ctx)

	require.Zero(t, f.launches, "a stopped run's queued start must not launch")
	require.False(t, f.ceilingDeferred(t), "the queued start of a closed run is dropped")
	require.Equal(t, "stopped by user", f.run(t).ErrorMessage)
}

func TestAutomationStartDeferredByCeiling_TaskDeletionFailsRunAndReleasesCapacity(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "t-defer-task-delete")
	require.NoError(t, f.repo.DeleteTask(ctx, f.taskID))

	f.svc.handleTaskDeleted(ctx, watcher.TaskEventData{TaskID: f.taskID})

	deleted := f.run(t)
	require.Equal(t, automation.RunStatusFailed, deleted.Status)
	require.Equal(t, "task deleted before deferred automation start", deleted.ErrorMessage)
	require.Zero(t, f.activeRuns(t), "deleting the queued task releases its automation slot")

	autoSvc := automation.NewService(f.autoStore, bus.NewMemoryEventBus(testLogger()), testLogger())
	result, err := autoSvc.FireTrigger(ctx, f.auto.ID, "", automation.TriggerType("manual"),
		json.RawMessage(`{}`), automation.DedupNotConfigured())
	require.NoError(t, err)
	require.False(t, result.Skipped, "a deleted deferred task must not block the next run")
	require.NotEmpty(t, result.RunID)
	require.Equal(t, 1, f.activeRuns(t))

	f.freeCeilingAndSweep(ctx)
	require.Zero(t, f.launches, "the deleted task's queued start must not launch")
}

// Deleting the automation removes its runs. A task that outlives them drops
// its queued start instead of retrying a launch no run can own.
func TestAutomationStartDeferredByCeiling_DeletedAutomationDropsTheStart(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "t-defer-delete")
	require.NoError(t, f.autoSvc.DeleteAutomation(ctx, f.auto.ID))

	f.freeCeilingAndSweep(ctx)

	require.Zero(t, f.launches, "a start whose run is gone must not launch")
	require.False(t, f.ceilingDeferred(t), "the start of a deleted run is dropped, not retried")
}

// Dropping a queued automation start settles its run: the launch that would
// have produced a completion event never happens.
func TestAutomationStartDeferredByCeiling_DroppedStartFailsTheRun(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "t-defer-drop")
	require.NoError(t, f.svc.repo.(interface {
		ArchiveTask(context.Context, string) error
	}).ArchiveTask(ctx, f.taskID))

	f.svc.drainDeferredCeilingLaunches(ctx)

	dropped := f.run(t)
	require.Equal(t, automation.RunStatusFailed, dropped.Status)
	require.Contains(t, dropped.ErrorMessage, "task is archived")
	require.False(t, f.ceilingDeferred(t))
	require.Zero(t, f.activeRuns(t))
	require.Zero(t, f.launches)
}

// A replay that fails for a non-ceiling reason fails its run, and the closed
// run's record is dropped rather than retried.
func TestAutomationStartDeferredByCeiling_FailedReplayFailsTheRunOnce(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "t-defer-fail")
	f.launchErr = errors.New("executor unavailable")

	f.freeCeilingAndSweep(ctx)

	failed := f.run(t)
	require.Equal(t, automation.RunStatusFailed, failed.Status)
	require.Contains(t, failed.ErrorMessage, "executor unavailable")
	require.Zero(t, f.activeRuns(t))
	require.Equal(t, 1, f.launches)

	f.svc.drainDeferredCeilingLaunches(ctx)
	require.Equal(t, 1, f.launches, "a failed run's start is not launched again")
	require.False(t, f.ceilingDeferred(t))
}

// A replay whose launch succeeds but whose run cannot be bound fails the run
// and stops the launched session, since no completion would settle the run.
// The task stays, and the next sweep drops the start without launching again.
func TestAutomationStartDeferredByCeiling_UnboundReplayStopsItsSession(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "t-defer-unbound")
	f.db.MustExec(`CREATE TRIGGER refuse_automation_run_binding
		BEFORE UPDATE OF status ON automation_runs WHEN NEW.status = '` + string(automation.RunStatusTaskCreated) + `'
		BEGIN SELECT RAISE(ABORT, 'run binding refused'); END`)

	f.freeCeilingAndSweep(ctx)

	failed := f.run(t)
	require.Equal(t, 1, f.launches)
	require.Equal(t, automation.RunStatusFailed, failed.Status)
	require.Contains(t, failed.ErrorMessage, "run binding refused")
	require.Zero(t, f.activeRuns(t))
	sessions, err := f.repo.ListTaskSessions(ctx, f.taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, models.TaskSessionStateCancelled, sessions[0].State,
		"a launched session whose run is not bound must be stopped")
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.NotNil(t, task, "stopping the session keeps the task")

	f.svc.drainDeferredCeilingLaunches(ctx)
	require.Equal(t, 1, f.launches, "a failed run's start is not launched again")
	require.False(t, f.ceilingDeferred(t))
	require.Contains(t, f.run(t).ErrorMessage, "run binding refused", "dropping the start keeps the run's failure")
}

// A queued start that survives its successful replay, for example because
// clearing the record failed, is dropped by the next sweep: the bound run is
// neither launched again nor failed. On a workflow step the replay also passes
// the unrouted workflow-entry check, and the step's auto-start setting does not
// gate a run's start.
func TestAutomationStartDeferredByCeiling_RecordOfABoundRunIsDroppedWithoutFailingIt(t *testing.T) {
	cases := []struct {
		name string
		step *wfmodels.WorkflowStep
	}{
		{name: "no workflow step"},
		{name: "workflow step", step: &wfmodels.WorkflowStep{ID: "step-review", WorkflowID: "wf-automation", Name: "Review"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := deferAutomationStartAtCeilingOnStep(t, "t-defer-survive", tc.step)
			if tc.step != nil {
				f.requireUnroutedWorkflowEntry(ctx, t, tc.step.ID)
			}
			bound := f.replayKeepingTheRecord(ctx, t)

			f.svc.drainDeferredCeilingLaunches(ctx)

			require.Equal(t, 1, f.launches, "a bound run's start is not launched again")
			require.False(t, f.ceilingDeferred(t), "the record of a bound run is dropped")
			after := f.run(t)
			require.Equal(t, automation.RunStatusTaskCreated, after.Status, "dropping the record must not fail a bound run")
			require.Empty(t, after.ErrorMessage)
			require.Equal(t, bound.SessionID, after.SessionID)
			require.Equal(t, bound.TurnID, after.TurnID)
		})
	}
}

// requireUnroutedWorkflowEntry asserts that the queued start is bound to its
// workflow-step entry while the task has no workflow session route, so the
// replay validates it as an unrouted workflow entry.
func (f *deferredAutomationStartFixture) requireUnroutedWorkflowEntry(ctx context.Context, t *testing.T, stepID string) {
	t.Helper()
	record, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)
	deferral, err := models.ReadCeilingDeferral(record)
	require.NoError(t, err)
	binding, present, err := models.ReadCeilingWorkflowEntryBinding(deferral.Payload)
	require.NoError(t, err)
	require.True(t, present, "a start on a workflow step records its entry binding")
	require.Equal(t, stepID, binding.DestinationStepID)
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	_, routed := models.LoadWorkflowSessionRoute(task.Metadata)
	require.False(t, routed)
}

// replayKeepingTheRecord frees the ceiling and replays the queued start, then
// writes the queued record back as if clearing it had failed.
func (f *deferredAutomationStartFixture) replayKeepingTheRecord(ctx context.Context, t *testing.T) *automation.AutomationRun {
	t.Helper()
	record, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)

	f.freeCeilingAndSweep(ctx)

	bound := f.run(t)
	require.Equal(t, 1, f.launches)
	require.Equal(t, automation.RunStatusTaskCreated, bound.Status)
	require.NotEmpty(t, bound.TurnID)
	require.False(t, f.ceilingDeferred(t))
	_, prior, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)
	stored, _, err := f.repo.SetTaskDeferredLaunchIfUnchanged(ctx, f.taskID, prior, record)
	require.NoError(t, err)
	require.True(t, stored)
	require.True(t, f.ceilingDeferred(t))
	return bound
}

// A run that closes after the sweep's drop check but before its replay
// dispatches is not launched, and its queued start is dropped at once.
func TestAutomationStartDeferredByCeiling_RunClosedBeforeReplayDropsTheStart(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "t-defer-closed")
	_, err := f.autoSvc.StopRun(ctx, f.auto.ID, f.runID)
	require.NoError(t, err)
	f.svc.sessionCeiling.release(deferredAutomationOccupierSession)
	record, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)
	deferral, err := models.ReadCeilingDeferral(record)
	require.NoError(t, err)
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)

	outcome := f.svc.replayCeilingDeferral(ctx, task, deferral)
	require.Equal(t, ceilingReplayRunClosed, outcome)
	require.Zero(t, f.launches)

	f.svc.settleCeilingReplay(ctx, task, nil, deferral, outcome)
	require.False(t, f.ceilingDeferred(t), "a start whose run closed is dropped, not retried")
	require.Equal(t, "stopped by user", f.run(t).ErrorMessage)
}
