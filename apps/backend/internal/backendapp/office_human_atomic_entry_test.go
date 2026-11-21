package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowadapters "github.com/kandev/kandev/internal/workflow/adapters"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
	"github.com/stretchr/testify/require"
)

type officeEntryRunSink struct{ requests []engine.QueueRunRequest }

func (s *officeEntryRunSink) QueueRun(_ context.Context, req engine.QueueRunRequest) (engine.QueueOutcome, error) {
	s.requests = append(s.requests, req)
	return engine.QueueOutcomeQueued, nil
}

func TestOfficeHumanMoveDispatchesEngineEntryAfterCommit(t *testing.T) {
	f := newOfficeAtomicFixture(t, nil)
	ctx := context.Background()
	step, err := f.h.workflowSvc.GetStep(ctx, "atomic-done")
	require.NoError(t, err)
	step.Events = wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
		{Type: wfmodels.OnEnterClearDecisions},
		{Type: wfmodels.OnEnterQueueRunForEachParticipant, Config: map[string]any{"role": "reviewer"}},
	}}
	require.NoError(t, f.h.workflowSvc.UpdateStep(ctx, step))
	seat, err := f.workflow.UpsertTaskParticipant(ctx, "atomic-done", "atomic-task", "reviewer", "review-agent")
	require.NoError(t, err)
	require.NoError(t, f.workflow.RecordStepDecision(ctx, &wfmodels.WorkflowStepDecision{TaskID: "atomic-task", StepID: "atomic-done", ParticipantID: seat, Decision: "approved", DeciderType: "agent", DeciderID: "review-agent", Role: "reviewer"}))
	svc := orchestrator.NewService(orchestrator.DefaultServiceConfig(), f.h.eventBus, nil, &taskRepositoryAdapter{repo: f.h.repo, svc: f.svc}, f.h.repo, nil, nil, nil, f.log)
	t.Cleanup(func() { _ = svc.Stop() })
	svc.SetWorkflowStepGetter(&orchestratorWorkflowStepGetterAdapter{svc: f.h.workflowSvc})
	svc.SetEngineDecisionStore(workflowadapters.NewDecisionAdapter(f.workflow))
	svc.SetEngineParticipantStore(workflowadapters.NewParticipantAdapter(f.workflow))
	sink := &officeEntryRunSink{}
	svc.SetEngineRunQueue(sink)
	f.h.repo.SetStepEntryDispatcher(&engineStepEntryDispatcherAdapter{engineProvider: svc, log: f.log})
	pending, ok := stepentry.BuildPendingAllocation(step.ID, step.Events.OnEnter)
	require.True(t, ok)
	_, err = f.svc.MoveTaskWithOptions(stepentry.WithPendingAllocation(ctx, pending), "atomic-task", "wf-home", "atomic-done", 0, taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman})
	require.NoError(t, err)
	task, err := f.h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	require.Equal(t, "atomic-done", task.WorkflowStepID)
	decisions, err := f.workflow.ListStepDecisions(ctx, "atomic-task", "atomic-done")
	require.NoError(t, err)
	require.Empty(t, decisions, "committed human move must allow the engine's decision clear")
	var entry int64
	require.NoError(t, f.h.db.QueryRowContext(ctx, `SELECT id FROM workflow_step_entries WHERE task_id='atomic-task' AND step_id='atomic-done'`).Scan(&entry))
	for _, position := range []int{0, 1} {
		state, cause, found, err := f.h.repo.GetStepEntryMarkerState(ctx, entry, position)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, stepentry.MarkerDone, state, cause)
	}
	require.Len(t, sink.requests, 1)
	require.Equal(t, "review-agent", sink.requests[0].AgentProfileID)
}

type taskWritingStepEntryDispatcher struct {
	repo *tasksqlite.Repository
	err  error
}

func (d *taskWritingStepEntryDispatcher) DispatchStepEntry(ctx context.Context, taskID, _, _, _ string, _ int64) {
	task, err := d.repo.GetTask(ctx, taskID)
	if err != nil {
		d.err = err
		return
	}
	task.Title = "Updated by step entry"
	d.err = d.repo.UpdateTask(ctx, task)
}

func TestStepEntryWriteDoesNotInheritAdmission(t *testing.T) {
	f := newOfficeAtomicFixture(t, nil)
	ctx := context.Background()
	dispatcher := &taskWritingStepEntryDispatcher{repo: f.h.repo}
	f.h.repo.SetStepEntryDispatcher(dispatcher)
	_, err := f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0, taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman})
	require.NoError(t, err)
	require.NoError(t, dispatcher.err)
	task, err := f.h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	require.Equal(t, "Updated by step entry", task.Title)
}
