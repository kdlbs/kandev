package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const wipDeferralLogMessage = "workflow on_enter deferred until destination step admits the task"

// @covers AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.11
func TestWorkflowTransition_WIPDeferralLog(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    transitionLifecycleMode
		trigger engine.Trigger
	}{
		{"turn complete", transitionLifecycleWithOnEnter, engine.TriggerOnTurnComplete},
		{"turn start", transitionLifecycleOnTurnStart, engine.TriggerOnTurnStart},
		{"guarded decision", transitionLifecycleGuardedDecision, engine.TriggerOnTurnComplete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertWIPDeferralLifecycle(t, tc.mode, tc.trigger)
		})
	}
}

func assertWIPDeferralLifecycle(t *testing.T, mode transitionLifecycleMode, trigger engine.Trigger) {
	t.Helper()
	ctx := context.Background()
	svc, logs, onEnterDone := newWIPDeferralTestService(t, true)
	session, err := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)

	if mode == transitionLifecycleGuardedDecision {
		applied, transitionErr := svc.applyGuardedTransitionLifecycle(ctx, "t1", "s1", "step1", "step2", trigger)
		require.NoError(t, transitionErr)
		require.True(t, applied)
	} else {
		require.True(t, svc.applyEngineTransitionWithMode(ctx, "t1", session, engine.HandleResult{
			Transitioned: true, FromStepID: "step1", ToStepID: "step2",
		}, trigger, "Test", mode))
	}

	task, err := svc.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step2", task.WorkflowStepID)
	require.False(t, task.WIPAdmitted)
	require.Equal(t, "step2", task.QueuedForStepID)
	require.NotNil(t, task.QueuedAt)

	entries := logs.FilterMessage(wipDeferralLogMessage).All()
	storedSession, err := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	if mode == transitionLifecycleGuardedDecision {
		require.Empty(t, entries)
		require.Equal(t, session.State, storedSession.State)
		require.Equal(t, session.Metadata, storedSession.Metadata)
	} else {
		require.Len(t, entries, 1)
		require.Equal(t, zapcore.InfoLevel, entries[0].Level)
		require.Equal(t, map[string]interface{}{
			"task_id": "t1", "session_id": "s1",
			"from_step_id": "step1", "to_step_id": "step2",
			"wip_limit": int64(1), "reason": "wip_limit_reached",
		}, entries[0].ContextMap())
		require.Equal(t, false, storedSession.Metadata["plan_mode"])
	}

	select {
	case <-onEnterDone:
		t.Fatal("destination entry ran before WIP admission")
	case <-time.After(250 * time.Millisecond):
	}
}

func TestWorkflowTransition_AdmittedDestinationDoesNotLogWIPDeferral(t *testing.T) {
	ctx := context.Background()
	svc, logs, onEnterDone := newWIPDeferralTestService(t, false)
	session, err := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)

	require.True(t, svc.applyEngineTransitionWithMode(ctx, "t1", session, engine.HandleResult{
		Transitioned: true, FromStepID: "step1", ToStepID: "step2",
	}, engine.TriggerOnTurnComplete, "Test", transitionLifecycleWithOnEnter))
	select {
	case <-onEnterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("admitted destination entry did not complete")
	}

	require.Zero(t, logs.FilterMessage(wipDeferralLogMessage).Len())
	task, err := svc.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step2", task.WorkflowStepID)
	require.True(t, task.WIPAdmitted)
	require.Empty(t, task.QueuedForStepID)
	storedSession, err := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, true, storedSession.Metadata["plan_mode"])
}

func TestWorkflowTransition_WIPDeferralLogUsesAdmissionLimit(t *testing.T) {
	ctx := context.Background()
	svc, logs, _ := newWIPDeferralTestService(t, true)
	steps := svc.workflowStepGetter.(*mockStepGetter)
	stepReads := 0
	steps.getStepFunc = func(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
		step := steps.steps[stepID]
		if step == nil {
			return nil, nil
		}
		copy := *step
		if stepID == "step2" {
			stepReads++
			if stepReads == 3 {
				copy.WIPLimit = 1
			} else {
				copy.WIPLimit = 2
			}
		}
		return &copy, nil
	}
	session, err := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)

	require.True(t, svc.applyEngineTransitionWithMode(ctx, "t1", session, engine.HandleResult{
		Transitioned: true, FromStepID: "step1", ToStepID: "step2",
	}, engine.TriggerOnTurnComplete, "Test", transitionLifecycleWithOnEnter))

	require.Equal(t, 3, stepReads)
	entries := logs.FilterMessage(wipDeferralLogMessage).All()
	require.Len(t, entries, 1)
	require.Equal(t, int64(1), entries[0].ContextMap()["wip_limit"])
	task, err := svc.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step2", task.QueuedForStepID)
	require.False(t, task.WIPAdmitted)
}

func TestWorkflowTransition_PostCommitTaskReloadFailureWarnsAndDefersEntry(t *testing.T) {
	ctx := context.Background()
	svc, logs, onEnterDone := newWIPDeferralTestService(t, true)
	repo := svc.repo.(*sqliterepo.Repository)
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)

	readErr := errors.New("task reread failed")
	wrappedRepo := &postCommitTaskReadFailureRepo{Repository: repo, readErr: readErr}
	svc.repo = wrappedRepo
	svc.workflowStore = newWorkflowStore(
		wrappedRepo, svc.workflowStepGetter, svc.agentManager, noopPublisher, svc.logger, &operationLedger{},
	)

	require.True(t, svc.applyEngineTransitionWithCommitMode(ctx, "t1", session, engine.HandleResult{
		Transitioned: true, FromStepID: "step1", ToStepID: "step2",
	}, engine.TriggerOnTurnComplete, "Test", transitionLifecycleWithOnEnter,
		func(commitCtx context.Context) (bool, error) {
			result, commitErr := svc.workflowStore.applyTransitionReportingApplied(
				commitCtx, "t1", "s1", "step1", "step2", engine.TriggerOnTurnComplete,
			)
			return result.Applied, commitErr
		}))

	warnings := logs.FilterMessage("failed to reload task after committed workflow transition").All()
	require.Len(t, warnings, 1)
	require.Equal(t, "t1", warnings[0].ContextMap()["task_id"])
	require.Equal(t, "s1", warnings[0].ContextMap()["session_id"])
	require.Equal(t, "step1", warnings[0].ContextMap()["from_step_id"])
	require.Equal(t, "step2", warnings[0].ContextMap()["to_step_id"])
	require.Equal(t, readErr.Error(), warnings[0].ContextMap()["error"])
	require.Zero(t, logs.FilterMessage(wipDeferralLogMessage).Len())

	task, err := repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "step2", task.QueuedForStepID)
	require.False(t, task.WIPAdmitted)
	storedSession, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, storedSession.State)
	select {
	case <-onEnterDone:
		t.Fatal("destination entry ran after task reload failed")
	case <-time.After(250 * time.Millisecond):
	}
}

func newWIPDeferralTestService(t *testing.T, full bool) (*Service, *observer.ObservedLogs, <-chan struct{}) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", "plan_mode", true))
	if full {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{
			ID: "occupant", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step2",
			Title: "Occupant", State: v1.TaskStateInProgress,
		}))
	}
	steps := newMockStepGetter()
	steps.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Source", Position: 0,
		Events: wfmodels.StepEvents{OnExit: []wfmodels.OnExitAction{{Type: wfmodels.OnExitDisablePlanMode}}},
	}
	steps.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Limited", Position: 1, WIPLimit: 1,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterEnablePlanMode}}},
	}
	steps.steps["step3"] = &wfmodels.WorkflowStep{
		ID: "step3", WorkflowID: "wf1", Name: "Done", Position: 2,
	}
	core, logs := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	svc := createTestService(repo, steps, newMockTaskRepo())
	svc.logger = log
	svc.workflowStore = newWorkflowStore(repo, steps, svc.agentManager, noopPublisher, log, &operationLedger{})
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() { onEnterDone <- struct{}{} }
	return svc, logs, onEnterDone
}

type postCommitTaskReadFailureRepo struct {
	*sqliterepo.Repository
	failNextGetTask atomic.Bool
	readErr         error
}

func (r *postCommitTaskReadFailureRepo) UpdateTaskWithWorkflowStepAdmissionReportingApplied(
	ctx context.Context, task *models.Task, sourceStepID, targetStepID string, limit int,
) (admitted, applied bool, err error) {
	admitted, applied, err = r.Repository.UpdateTaskWithWorkflowStepAdmissionReportingApplied(
		ctx, task, sourceStepID, targetStepID, limit,
	)
	if err == nil && applied {
		r.failNextGetTask.Store(true)
	}
	return admitted, applied, err
}

func (r *postCommitTaskReadFailureRepo) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	if r.failNextGetTask.CompareAndSwap(true, false) {
		return nil, r.readErr
	}
	return r.Repository.GetTask(ctx, taskID)
}
