package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeFinalStepTasks struct {
	task    *models.Task
	moveErr error
	moves   []string
	opts    taskservice.MoveTaskOptions
}

func (f *fakeFinalStepTasks) GetTask(context.Context, string) (*models.Task, error) {
	return f.task, nil
}

func (f *fakeFinalStepTasks) MoveTaskWithOptions(
	_ context.Context, id, workflowID, stepID string, _ int, opts taskservice.MoveTaskOptions,
) (*taskservice.MoveTaskResult, error) {
	f.moves = append(f.moves, id+"->"+workflowID+"/"+stepID)
	f.opts = opts
	if f.moveErr != nil {
		return nil, f.moveErr
	}
	moved := *f.task
	moved.WorkflowStepID = stepID
	moved.State = v1.TaskStateCompleted
	return &taskservice.MoveTaskResult{Task: &moved, WorkflowStep: &wfmodels.WorkflowStep{ID: stepID, Name: "Done"}}, nil
}

type fakeWorkflowSteps struct{ steps []*wfmodels.WorkflowStep }

func (f fakeWorkflowSteps) ListStepsByWorkflow(context.Context, string) ([]*wfmodels.WorkflowStep, error) {
	return f.steps, nil
}

func newFinalStepAdapter(tasks *fakeFinalStepTasks) officeFinalStepMoverAdapter {
	return officeFinalStepMoverAdapter{tasks: tasks, steps: fakeWorkflowSteps{steps: []*wfmodels.WorkflowStep{
		{ID: "step-work", Position: 1}, {ID: "step-done", Name: "Done", Position: 4}, {ID: "step-review", Position: 2},
	}}}
}

func workTask() *models.Task {
	return &models.Task{ID: "task-1", WorkflowID: "wf-1", WorkflowStepID: "step-work", State: v1.TaskStateInProgress}
}

// @covers AC-OFFICE-HUMAN-DONE-001.1
func TestOfficeFinalStepMover_MovesToHighestPositionStepAsHuman(t *testing.T) {
	tasks := &fakeFinalStepTasks{task: workTask()}

	result, err := newFinalStepAdapter(tasks).MoveTaskToFinalStep(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("MoveTaskToFinalStep: %v", err)
	}
	if len(tasks.moves) != 1 || tasks.moves[0] != "task-1->wf-1/step-done" {
		t.Fatalf("moves = %v, want task-1->wf-1/step-done", tasks.moves)
	}
	if tasks.opts.StepHistoryActor != wfmodels.StepTransitionActorHuman || !tasks.opts.AllowActivePrimarySession {
		t.Fatalf("opts = %+v, want human actor with active primary session allowed", tasks.opts)
	}
	if result.State != string(v1.TaskStateCompleted) || result.StepID != "step-done" || result.StepName != "Done" {
		t.Fatalf("result = %+v, want COMPLETED at step-done/Done", result)
	}
}

// @covers AC-OFFICE-HUMAN-DONE-001.3
func TestOfficeFinalStepMover_TranslatesRefusals(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReason string
		wantCode   string
		wantNames  int
	}{
		{
			name: "pending approvers",
			err: &taskservice.ApprovalsPendingMoveError{
				Pending: []taskservice.PendingApprover{{AgentProfileID: "agent-ceo", Name: "CEO"}},
			},
			wantReason: dashboard.ApprovalGateReasonApprovals,
			wantNames:  1,
		},
		{
			name:       "completion gate",
			err:        taskrepository.ErrTaskCompletionGateBlocked,
			wantReason: dashboard.FinalStepMoveReasonCompletionGate,
			wantCode:   "task_completion_gate_blocked",
		},
		{
			name:       "active session",
			err:        errors.New("task has an active session"),
			wantReason: dashboard.FinalStepMoveReasonWorkflowMove,
			wantCode:   "task_move_active_session",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tasks := &fakeFinalStepTasks{task: workTask(), moveErr: tc.err}

			_, err := newFinalStepAdapter(tasks).MoveTaskToFinalStep(context.Background(), "task-1")

			var refused *dashboard.FinalStepMoveRefusedError
			if !errors.As(err, &refused) {
				t.Fatalf("err = %v, want FinalStepMoveRefusedError", err)
			}
			if refused.Reason != tc.wantReason || refused.Code != tc.wantCode || len(refused.Pending) != tc.wantNames {
				t.Fatalf("refused = %+v, want reason %q code %q pending %d", refused, tc.wantReason, tc.wantCode, tc.wantNames)
			}
		})
	}
}

func TestOfficeFinalStepMover_UnclassifiedErrorPassesThrough(t *testing.T) {
	moveErr := errors.New("database is locked")
	tasks := &fakeFinalStepTasks{task: workTask(), moveErr: moveErr}

	_, err := newFinalStepAdapter(tasks).MoveTaskToFinalStep(context.Background(), "task-1")

	var refused *dashboard.FinalStepMoveRefusedError
	if errors.As(err, &refused) || !errors.Is(err, moveErr) {
		t.Fatalf("err = %v, want the raw move error", err)
	}
}
