package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// specValidateFixtureSimple builds a fakeDecisionTaskService with one
// workflow ("wf-1", workspace "ws-1"), one repository ("repo-1", "ws-1"),
// one task ("task-0", "ws-1"), and a fakeStepReader for "wf-1" with two
// steps: "start" (the workflow's start step, eligible) and "auto" (auto-
// starts an agent on enter, so ineligible itself).
func specValidateFixtureSimple() (*fakeDecisionTaskService, *fakeStepReader) {
	tasks := &fakeDecisionTaskService{
		workflows: map[string]*taskmodels.Workflow{
			"wf-1": {ID: "wf-1", WorkspaceID: "ws-1"},
		},
		repos: map[string]*taskmodels.Repository{
			"repo-1": {ID: "repo-1", WorkspaceID: "ws-1"},
		},
		tasks: map[string]*taskmodels.Task{
			"task-0": {ID: "task-0", WorkspaceID: "ws-1"},
		},
	}
	steps := &fakeStepReader{steps: []*workflowmodels.WorkflowStep{
		{ID: "start", IsStartStep: true},
		{ID: "auto", Events: workflowmodels.StepEvents{
			OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}},
		}},
	}}
	return tasks, steps
}

func newSpecValidateService(t *testing.T, tasks *fakeDecisionTaskService, steps *fakeStepReader) *Service {
	t.Helper()
	store := newTestStore(t)
	svc := NewService(store, newValidatorForTest(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t))
	svc.SetDecisionDeps(tasks, steps, nil)
	return svc
}

func TestValidateProposalSpec_GoldenPath(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)

	spec := ProposalSpec{
		Title:        "  Do the thing  ",
		Description:  "  A description  ",
		Rationale:    "  Because  ",
		WorkflowID:   "wf-1",
		StepID:       "start",
		RepositoryID: "repo-1",
		SourceTaskID: "task-0",
	}
	got, err := svc.validateProposalSpec(context.Background(), "ws-1", spec)
	if err != nil {
		t.Fatalf("validateProposalSpec: %v", err)
	}
	want := ProposalSpec{
		Title:        "Do the thing",
		Description:  "A description",
		Rationale:    "Because",
		WorkflowID:   "wf-1",
		StepID:       "start",
		RepositoryID: "repo-1",
		SourceTaskID: "task-0",
	}
	if got != want {
		t.Fatalf("validateProposalSpec = %+v, want %+v", got, want)
	}
}

func TestValidateProposalSpec_EmptyOptionalFieldsSkipChecks(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)

	spec := ProposalSpec{Title: "T", WorkflowID: "wf-1", StepID: "start"}
	got, err := svc.validateProposalSpec(context.Background(), "ws-1", spec)
	if err != nil {
		t.Fatalf("validateProposalSpec: %v", err)
	}
	if got.RepositoryID != "" || got.SourceTaskID != "" {
		t.Fatalf("got = %+v, want empty repository_id/source_task_id", got)
	}
}

func TestValidateProposalSpec_TitleEmptyAfterTrim(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "   ", WorkflowID: "wf-1", StepID: "start"})
	assertFieldError(t, err, "title")
}

func TestValidateProposalSpec_TitleTooLong(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	title := strings.Repeat("a", 61)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: title, WorkflowID: "wf-1", StepID: "start"})
	assertFieldError(t, err, "title")
}

func TestValidateProposalSpec_TitleAtMaxLengthIsValid(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	title := strings.Repeat("a", 60)
	got, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: title, WorkflowID: "wf-1", StepID: "start"})
	if err != nil {
		t.Fatalf("validateProposalSpec: %v", err)
	}
	if got.Title != title {
		t.Fatalf("Title = %q, want %q", got.Title, title)
	}
}

func TestValidateProposalSpec_DescriptionTooLong(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	desc := strings.Repeat("a", 10001)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", Description: desc, WorkflowID: "wf-1", StepID: "start"})
	assertFieldError(t, err, "description")
}

func TestValidateProposalSpec_RationaleTooLong(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	rationale := strings.Repeat("a", 10001)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", Rationale: rationale, WorkflowID: "wf-1", StepID: "start"})
	assertFieldError(t, err, "rationale")
}

func TestValidateProposalSpec_WorkflowIDRequired(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T"})
	assertFieldError(t, err, "workflow_id")
}

func TestValidateProposalSpec_WorkflowNotFound(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", WorkflowID: "wf-missing", StepID: "start"})
	assertFieldError(t, err, "workflow_id")
}

func TestValidateProposalSpec_WorkflowInDifferentWorkspace(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	// wf-1 belongs to ws-1; the fake's GetWorkflow raises no error for
	// ws-2 (simulating a caller whose identity can also reach ws-1), so
	// this exercises the explicit WorkspaceID comparison.
	_, err := svc.validateProposalSpec(context.Background(), "ws-2", ProposalSpec{Title: "T", WorkflowID: "wf-1", StepID: "start"})
	assertFieldError(t, err, "workflow_id")
}

func TestValidateProposalSpec_WorkflowReadErrorPropagates(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	wantErr := errors.New("boom")
	tasks.workflowErr = map[string]error{"wf-1": wantErr}
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", WorkflowID: "wf-1", StepID: "start"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	var fieldErr *FieldError
	if errors.As(err, &fieldErr) {
		t.Fatalf("err = %v, want a plain read error, not a FieldError", err)
	}
}

func TestValidateProposalSpec_StepIDRequired(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", WorkflowID: "wf-1"})
	assertFieldError(t, err, "step_id")
}

func TestValidateProposalSpec_StepNotInWorkflow(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", WorkflowID: "wf-1", StepID: "nope"})
	assertFieldError(t, err, "step_id")
}

func TestValidateProposalSpec_StepIneligible(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", WorkflowID: "wf-1", StepID: "auto"})
	assertFieldError(t, err, "step_id")
}

func TestValidateProposalSpec_RepositoryNotFound(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", WorkflowID: "wf-1", StepID: "start", RepositoryID: "repo-missing"})
	assertFieldError(t, err, "repository_id")
}

func TestValidateProposalSpec_RepositoryInDifferentWorkspace(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	// repo-1 belongs to ws-1; point it at a different workspace than the
	// one being validated, with the workflow (also ws-1) left alone, so
	// this exercises the repository_id branch specifically.
	tasks.repos["repo-1"].WorkspaceID = "ws-2"
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", WorkflowID: "wf-1", StepID: "start", RepositoryID: "repo-1"})
	assertFieldError(t, err, "repository_id")
}

func TestValidateProposalSpec_SourceTaskNotFound(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", WorkflowID: "wf-1", StepID: "start", SourceTaskID: "task-missing"})
	assertFieldError(t, err, "source_task_id")
}

func TestValidateProposalSpec_SourceTaskInDifferentWorkspace(t *testing.T) {
	tasks, steps := specValidateFixtureSimple()
	tasks.tasks["task-0"].WorkspaceID = "ws-2"
	svc := newSpecValidateService(t, tasks, steps)
	_, err := svc.validateProposalSpec(context.Background(), "ws-1", ProposalSpec{Title: "T", WorkflowID: "wf-1", StepID: "start", SourceTaskID: "task-0"})
	assertFieldError(t, err, "source_task_id")
}
