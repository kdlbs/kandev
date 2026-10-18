package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/engine_dispatcher"
	"github.com/kandev/kandev/internal/office/scheduler"
	"github.com/kandev/kandev/internal/office/service"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/adapters"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// The resolution workflow only queues a run. Reads use the real task/step stores;
// embedding the unused transition methods makes an unexpected move fail loudly.
type assignmentResolutionStore struct {
	engine.TransitionStore
	f       *assignmentBlockerFixture
	applied map[string]bool
}

func (s *assignmentResolutionStore) LoadState(ctx context.Context, taskID, sessionID string) (engine.MachineState, error) {
	task, err := s.f.tasks.GetTask(ctx, taskID)
	if err != nil {
		return engine.MachineState{}, err
	}
	return engine.MachineState{TaskID: taskID, SessionID: sessionID, WorkflowID: task.WorkflowID, CurrentStepID: task.WorkflowStepID}, nil
}
func (s *assignmentResolutionStore) LoadStep(ctx context.Context, _, stepID string) (engine.StepSpec, error) {
	step, err := s.f.workflows.GetStep(ctx, stepID)
	if err != nil {
		return engine.StepSpec{}, err
	}
	return engine.CompileStep(step), nil
}
func (s *assignmentResolutionStore) IsOperationApplied(_ context.Context, op string) (bool, error) {
	return s.applied[op], nil
}
func (s *assignmentResolutionStore) MarkOperationApplied(_ context.Context, op string) error {
	s.applied[op] = true
	return nil
}
func (s *assignmentResolutionStore) PersistData(context.Context, string, map[string]any) error {
	return nil
}

type assignmentResolutionQueue struct{ runs *runsservice.Service }

func (q assignmentResolutionQueue) QueueRun(ctx context.Context, req engine.QueueRunRequest) (engine.QueueOutcome, error) {
	outcome, err := q.runs.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: req.AgentProfileID, TaskID: req.TaskID, WorkflowStepID: req.WorkflowStepID,
		Reason: req.Reason, IdempotencyKey: req.IdempotencyKey, Payload: req.Payload,
	})
	return engine.QueueOutcome(outcome), err
}

func (f *assignmentBlockerFixture) wireResolutionEngine(t *testing.T) {
	t.Helper()
	step, err := f.workflows.GetStep(context.Background(), "assignment-work")
	blockerMust(t, err)
	step.Events.OnBlockerResolved = []wfmodels.GenericAction{{Type: wfmodels.GenericActionQueueRun,
		Config: map[string]any{"target": "primary", "task_id": "this", "reason": service.RunReasonTaskBlockersResolved}}}
	blockerMust(t, f.workflows.UpdateStep(context.Background(), step))
	eng := engine.New(&assignmentResolutionStore{f: f, applied: map[string]bool{}}, engine.MapRegistry{
		engine.ActionQueueRun: engine.QueueRunCallback{Adapter: assignmentResolutionQueue{runs: f.runs}, Primary: adapters.NewPrimaryAgentAdapter(f.workflows)},
	})
	f.svc.SetWorkflowEngineDispatcher(engine_dispatcher.New(eng, f.tasks, logger.Default()))
	blockerMust(t, f.tasks.CreateTaskSession(context.Background(), &taskmodels.TaskSession{
		ID: "dependent-session", TaskID: "dependent", AgentProfileID: "assignment-worker", State: taskmodels.TaskSessionStateWaitingForInput,
	}))
}

// @covers AC-OFFICE-SCHEDULER-002.7
func TestOfficeAssignmentBlockerResolution(t *testing.T) {
	for _, source := range []string{"service_event", "reactivity_without_session"} {
		t.Run(source, func(t *testing.T) {
			f := newAssignmentBlockerFixture(t)
			f.task(t, "dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
			f.blockers(t, "dependent", v1.TaskStateInProgress, v1.TaskStateInProgress)
			f.assignment(t, events.TaskCreated, "dependent", "assignment-worker")
			if got := len(f.taskRuns(t, "dependent", service.RunReasonTaskAssigned)); got != 0 {
				t.Errorf("assignment while pending=%d, want 0", got)
			}
			if source == "service_event" {
				f.wireResolutionEngine(t)
			} else {
				_, err := f.tasks.GetActiveTaskSessionByTaskID(context.Background(), "dependent")
				if !errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
					t.Fatalf("held task has session: %v", err)
				}
			}
			for i := 0; i < 2; i++ {
				id := fmt.Sprintf("dependent-prerequisite-%d", i)
				_, err := f.repo.UpdateTaskAssignee(context.Background(), id, "assignment-worker")
				blockerMust(t, err)
				blockerMust(t, f.tasks.UpdateTaskState(context.Background(), id, v1.TaskStateCompleted))
				if source == "service_event" {
					data := service.TaskMovedData{TaskID: id, WorkspaceID: "assignment-ws", AssigneeAgentProfileID: "assignment-worker", FromStepName: "Work", ToStepName: "Done"}
					blockerMust(t, f.eb.Publish(context.Background(), events.TaskMoved, bus.NewEvent(events.TaskMoved, "resolution-test", data)))
				} else {
					done := "done"
					_, err := f.scheduler.ApplyTaskMutation(context.Background(), &scheduler.TaskSnapshot{ID: id, WorkspaceID: "assignment-ws", State: "IN_PROGRESS"}, scheduler.TaskMutation{NewStatus: &done, ActorType: "user", ActorID: "operator"})
					blockerMust(t, err)
				}
				runs := f.taskRuns(t, "dependent", service.RunReasonTaskBlockersResolved)
				if len(runs) != i {
					t.Fatalf("after completion %d: resolution runs=%d, want %d", i+1, len(runs), i)
				}
				if len(runs) == 1 && (runs[0].IdempotencyKey == nil || *runs[0].IdempotencyKey == "") {
					t.Fatal("resolution lost its key")
				}
			}
		})
	}
}
