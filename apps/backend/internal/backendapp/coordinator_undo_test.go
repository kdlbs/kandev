package backendapp

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

type fakeUndoTaskAPI struct {
	archiveErr error
	task       *taskmodels.Task
	getErr     error
	moveResult *taskservice.MoveTaskResult
	moveErr    error
	moveOpts   taskservice.MoveTaskOptions
	sessions   []*taskmodels.TaskSession
	sessErr    error
}

func (f *fakeUndoTaskAPI) ArchiveTask(context.Context, string) error { return f.archiveErr }
func (f *fakeUndoTaskAPI) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return f.task, f.getErr
}
func (f *fakeUndoTaskAPI) MoveTaskWithOptions(_ context.Context, _, _, _ string, _ int, opts taskservice.MoveTaskOptions) (*taskservice.MoveTaskResult, error) {
	f.moveOpts = opts
	return f.moveResult, f.moveErr
}
func (f *fakeUndoTaskAPI) ListTaskSessions(context.Context, string) ([]*taskmodels.TaskSession, error) {
	return f.sessions, f.sessErr
}

type fakeUndoStepAPI struct {
	step  *wfmodels.WorkflowStep
	steps []*wfmodels.WorkflowStep
	err   error
}

func (f *fakeUndoStepAPI) GetStep(context.Context, string) (*wfmodels.WorkflowStep, error) {
	return f.step, f.err
}

func (f *fakeUndoStepAPI) ListStepsByWorkflow(context.Context, string) ([]*wfmodels.WorkflowStep, error) {
	return f.steps, f.err
}

func TestUndoSeam_ListStepsBuildsFeederGraph(t *testing.T) {
	steps := []*wfmodels.WorkflowStep{
		{ID: "a", PullFromStepID: "", AllowManualMove: true},
		{ID: "b", Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}}, PullFromStepID: "a"},
	}
	nodes, err := (&coordinatorUndoSeam{steps: &fakeUndoStepAPI{steps: steps}}).ListSteps(context.Background(), "wf")
	if err != nil || len(nodes) != 2 {
		t.Fatalf("nodes = %+v err = %v", nodes, err)
	}
	if !coordinator.StartsAgentOnEnter(nodes, "a") {
		t.Fatal("a feeds an auto-start step, want StartsAgentOnEnter true")
	}
}

func TestUndoSeam_ArchiveMapsErrors(t *testing.T) {
	cases := []struct {
		in   error
		want error
	}{
		{nil, nil},
		{fmt.Errorf("wrapped: %w", taskservice.ErrTaskAlreadyArchived), coordinator.ErrTaskAlreadyArchived},
		{fmt.Errorf("wrapped: %w", repoerrors.ErrTaskNotFound), coordinator.ErrTaskNotFound},
	}
	for _, tc := range cases {
		seam := &coordinatorUndoSeam{tasks: &fakeUndoTaskAPI{archiveErr: tc.in}}
		got := seam.ArchiveTask(context.Background(), "t")
		if !errors.Is(got, tc.want) {
			t.Errorf("archive(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	boom := errors.New("boom")
	if got := (&coordinatorUndoSeam{tasks: &fakeUndoTaskAPI{archiveErr: boom}}).ArchiveTask(context.Background(), "t"); !errors.Is(got, boom) {
		t.Errorf("other error = %v", got)
	}
}

func TestUndoSeam_GetTask(t *testing.T) {
	archived := time.Now()
	seam := &coordinatorUndoSeam{tasks: &fakeUndoTaskAPI{task: &taskmodels.Task{
		Identifier: "KAN-1", ArchivedAt: &archived, WorkflowID: "wf", WorkflowStepID: "s1",
	}}}
	got, err := seam.GetTask(context.Background(), "t")
	if err != nil || got.Identifier != "KAN-1" || got.ArchivedAt == nil || got.WorkflowID != "wf" || got.WorkflowStepID != "s1" {
		t.Fatalf("got = %+v err = %v", got, err)
	}
	missing := &coordinatorUndoSeam{tasks: &fakeUndoTaskAPI{getErr: fmt.Errorf("x: %w", repoerrors.ErrTaskNotFound)}}
	if _, err := missing.GetTask(context.Background(), "t"); !errors.Is(err, coordinator.ErrTaskNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestUndoSeam_MoveOptionsAndResult(t *testing.T) {
	api := &fakeUndoTaskAPI{moveResult: &taskservice.MoveTaskResult{Task: &taskmodels.Task{WIPAdmitted: true}}}
	seam := &coordinatorUndoSeam{tasks: api}
	admitted, err := seam.MoveTaskWithOptions(context.Background(), "t", "wf", "s1", 0,
		coordinator.UndoMoveOptions{ExpectedWorkflowID: "wf", SkipStepPrompt: true})
	if err != nil || !admitted {
		t.Fatalf("admitted = %v err = %v", admitted, err)
	}
	if api.moveOpts.ExpectedWorkflowID == nil || *api.moveOpts.ExpectedWorkflowID != "wf" ||
		api.moveOpts.EntryOptions == nil || !api.moveOpts.EntryOptions.SkipStepPrompt {
		t.Fatalf("opts = %+v", api.moveOpts)
	}
	api.moveResult = &taskservice.MoveTaskResult{Task: &taskmodels.Task{}}
	if _, err := seam.MoveTaskWithOptions(context.Background(), "t", "wf", "s1", 0, coordinator.UndoMoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if api.moveOpts.EntryOptions != nil || api.moveOpts.ExpectedWorkflowID != nil {
		t.Fatalf("opts = %+v, want none", api.moveOpts)
	}
}

func TestUndoSeam_MoveMapsErrors(t *testing.T) {
	for in, want := range map[error]error{
		taskservice.ErrWIPLimitExceeded:           coordinator.ErrWIPLimitExceeded,
		taskservice.ErrWorkflowResolutionConflict: coordinator.ErrMoveConflict,
		workflowmove.ErrMoveConflict:              coordinator.ErrMoveConflict,
		repoerrors.ErrTaskNotFound:                coordinator.ErrTaskNotFound,
	} {
		seam := &coordinatorUndoSeam{tasks: &fakeUndoTaskAPI{moveErr: fmt.Errorf("wrapped: %w", in)}}
		if _, err := seam.MoveTaskWithOptions(context.Background(), "t", "wf", "s1", 0, coordinator.UndoMoveOptions{}); !errors.Is(err, want) {
			t.Errorf("move(%v) = %v, want %v", in, err, want)
		}
	}
	boom := errors.New("session is running")
	seam := &coordinatorUndoSeam{tasks: &fakeUndoTaskAPI{moveErr: boom}}
	_, err := seam.MoveTaskWithOptions(context.Background(), "t", "wf", "s1", 0, coordinator.UndoMoveOptions{})
	if !errors.Is(err, boom) {
		t.Fatalf("unclassified error = %v", err)
	}
}

func TestUndoSeam_GetStep(t *testing.T) {
	step := &wfmodels.WorkflowStep{Name: "Todo", WorkflowID: "wf", CompleteTaskOnEnter: true,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}}}
	got, err := (&coordinatorUndoSeam{steps: &fakeUndoStepAPI{step: step}}).GetStep(context.Background(), "s")
	if err != nil || got.Name != "Todo" || got.WorkflowID != "wf" || !got.AutoStart || !got.CompletesOnEnter {
		t.Fatalf("got = %+v err = %v", got, err)
	}
	gone := &coordinatorUndoSeam{steps: &fakeUndoStepAPI{err: fmt.Errorf("x: %w", wfmodels.ErrWorkflowStepNotFound)}}
	if _, err := gone.GetStep(context.Background(), "s"); !errors.Is(err, coordinator.ErrStepNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestUndoSeam_HasActiveSession(t *testing.T) {
	for state, want := range map[taskmodels.TaskSessionState]bool{
		taskmodels.TaskSessionStateStarting: true,
		taskmodels.TaskSessionStateRunning:  true,
		taskmodels.TaskSessionState("IDLE"): false,
	} {
		seam := &coordinatorUndoSeam{tasks: &fakeUndoTaskAPI{sessions: []*taskmodels.TaskSession{{State: state}}}}
		got, err := seam.HasActiveSession(context.Background(), "t")
		if err != nil || got != want {
			t.Errorf("state %s: got %v err %v, want %v", state, got, err, want)
		}
	}
	none := &coordinatorUndoSeam{tasks: &fakeUndoTaskAPI{}}
	if got, err := none.HasActiveSession(context.Background(), "t"); err != nil || got {
		t.Fatalf("no sessions: got %v err %v", got, err)
	}
	boom := errors.New("boom")
	if _, err := (&coordinatorUndoSeam{tasks: &fakeUndoTaskAPI{sessErr: boom}}).HasActiveSession(context.Background(), "t"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
