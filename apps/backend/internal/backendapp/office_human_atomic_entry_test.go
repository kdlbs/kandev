package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowadapters "github.com/kandev/kandev/internal/workflow/adapters"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
	"github.com/stretchr/testify/require"
	"sync"
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

func TestOfficeHumanAdmissionIntentEndsBeforePublishedCallbacks(t *testing.T) {
	for _, route := range []string{"move", "fields", "state"} {
		t.Run(route, func(t *testing.T) {
			f := newOfficeAtomicFixture(t, nil)
			ctx := context.Background()
			var mu sync.Mutex
			var leaked []string
			calls := 0
			sub, err := f.h.eventBus.Subscribe(">", func(ctx context.Context, event *bus.Event) error {
				mu.Lock()
				defer mu.Unlock()
				calls++
				if _, human := models.OfficeHumanCompletionIntentFromContext(ctx); human {
					leaked = append(leaked, event.Subject)
				}
				return nil
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sub.Unsubscribe()) })
			switch route {
			case "move":
				_, err = f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0, taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman})
			case "fields":
				title := "Renamed"
				_, err = f.svc.UpdateTaskAsHuman(ctx, "atomic-task", &taskservice.UpdateTaskRequest{Title: &title})
			case "state":
				_, err = f.svc.UpdateTaskStateAsHuman(ctx, "atomic-task", "COMPLETED")
			}
			require.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			require.Positive(t, calls)
			require.Empty(t, leaked, "postcommit consumers must not inherit a human admission fence")
		})
	}
}
