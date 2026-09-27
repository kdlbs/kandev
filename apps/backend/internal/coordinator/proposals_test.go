package coordinator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakeWorkflowReader, fakeRepositoryReader and fakeSourceTaskReader are
// keyed-map test doubles for the propose-time readers: a missing key returns
// (nil, nil), matching the real services' "not found" shape (no error, nil
// row), which buildProposalSpec must turn into a *FieldError.
type fakeWorkflowReader struct {
	workflows map[string]*taskmodels.Workflow
}

func (f fakeWorkflowReader) GetWorkflow(_ context.Context, id string) (*taskmodels.Workflow, error) {
	return f.workflows[id], nil
}

type fakeRepositoryReader struct {
	repositories map[string]*taskmodels.Repository
}

func (f fakeRepositoryReader) GetRepository(_ context.Context, id string) (*taskmodels.Repository, error) {
	return f.repositories[id], nil
}

type fakeSourceTaskReader struct{ tasks map[string]*taskmodels.Task }

func (f fakeSourceTaskReader) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	return f.tasks[id], nil
}

type fakeWorkflowStepReader struct {
	stepsByWorkflow map[string][]*wfmodels.WorkflowStep
}

func (f fakeWorkflowStepReader) ListStepsByWorkflow(_ context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error) {
	return f.stepsByWorkflow[workflowID], nil
}

// proposalTestFixture is the standard workspace/workflow/step graph shared by
// most propose_task tests: one workspace, one workflow with a start step
// ("start"), a manual-move step ("manual") and an auto-start step ("auto")
// fed directly by "manual".
type proposalTestFixture struct {
	svc         *Service
	coordinator *Coordinator
	workspaceID string
	workflowID  string
	repository  *taskmodels.Repository
	sourceTask  *taskmodels.Task
}

func newProposalTestFixture(t *testing.T) proposalTestFixture {
	t.Helper()
	const workspaceID = "ws-proposals"
	const workflowID = "wf-1"

	store := newTestStore(t)
	coordinator := newTestCoordinator(t, store, workspaceID)
	validator := newValidatorForTest(nil, nil)
	svc := NewService(store, validator, &fakeWorkspaceAuthorizer{}, newTestLogger(t))

	repository := &taskmodels.Repository{ID: "repo-1", WorkspaceID: workspaceID}
	sourceTask := &taskmodels.Task{ID: "task-source", WorkspaceID: workspaceID}
	svc.SetProposalDeps(
		fakeWorkflowReader{workflows: map[string]*taskmodels.Workflow{
			workflowID: {ID: workflowID, WorkspaceID: workspaceID},
		}},
		fakeRepositoryReader{repositories: map[string]*taskmodels.Repository{repository.ID: repository}},
		fakeSourceTaskReader{tasks: map[string]*taskmodels.Task{sourceTask.ID: sourceTask}},
		fakeWorkflowStepReader{stepsByWorkflow: map[string][]*wfmodels.WorkflowStep{
			workflowID: {
				{ID: "start", IsStartStep: true},
				{ID: "manual", AllowManualMove: true},
				{ID: "auto", AllowManualMove: true, PullFromStepID: "manual", Events: wfmodels.StepEvents{
					OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
				}},
			},
		}},
	)

	return proposalTestFixture{
		svc: svc, coordinator: coordinator, workspaceID: workspaceID, workflowID: workflowID,
		repository: repository, sourceTask: sourceTask,
	}
}

func (f proposalTestFixture) baseRequest() ProposeTaskRequest {
	return ProposeTaskRequest{
		Title:       "Fix the thing",
		Description: "A description",
		Rationale:   "Because it needs fixing",
		WorkflowID:  f.workflowID,
	}
}

// TestProposeTask_Success covers AC-COORDINATOR-PROPOSALS-001.1: valid
// fields store one pending proposal and return the coordinator's open count.
func TestProposeTask_Success(t *testing.T) {
	f := newProposalTestFixture(t)
	req := f.baseRequest()
	req.RepositoryID = f.repository.ID
	req.SourceTaskID = f.sourceTask.ID
	req.StepID = "start"

	proposal, openCount, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
	if err != nil {
		t.Fatalf("ProposeTask() unexpected error: %v", err)
	}
	if proposal.Status != ProposalStatusPending {
		t.Fatalf("proposal.Status = %q, want %q", proposal.Status, ProposalStatusPending)
	}
	if proposal.CoordinatorID != f.coordinator.ID || proposal.WorkspaceID != f.workspaceID {
		t.Fatalf("proposal coordinator/workspace = %q/%q, want %q/%q",
			proposal.CoordinatorID, proposal.WorkspaceID, f.coordinator.ID, f.workspaceID)
	}
	if proposal.Spec.StepID != "start" {
		t.Fatalf("proposal.Spec.StepID = %q, want %q", proposal.Spec.StepID, "start")
	}
	if openCount != 1 {
		t.Fatalf("openCount = %d, want 1", openCount)
	}
}

// TestProposeTask_StepOmittedDefaultsToStartStep covers
// AC-COORDINATOR-PROPOSALS-001.2.
func TestProposeTask_StepOmittedDefaultsToStartStep(t *testing.T) {
	f := newProposalTestFixture(t)
	proposal, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, f.baseRequest())
	if err != nil {
		t.Fatalf("ProposeTask() unexpected error: %v", err)
	}
	if proposal.Spec.StepID != "start" {
		t.Fatalf("proposal.Spec.StepID = %q, want %q", proposal.Spec.StepID, "start")
	}
}

// TestProposeTask_TitleValidation covers the title clause of
// AC-COORDINATOR-PROPOSALS-001.3: empty after trim or over 60 characters is
// refused, naming "title".
func TestProposeTask_TitleValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		title string
	}{
		{name: "empty", title: ""},
		{name: "whitespace only", title: "   "},
		{name: "over 60 chars", title: strings.Repeat("a", 61)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProposalTestFixture(t)
			req := f.baseRequest()
			req.Title = tc.title
			_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
			assertFieldError(t, err, "title")
		})
	}
}

// TestProposeTask_DescriptionAndRationaleLengthValidation covers the
// description/rationale clause of AC-COORDINATOR-PROPOSALS-001.3.
func TestProposeTask_DescriptionAndRationaleLengthValidation(t *testing.T) {
	tooLong := strings.Repeat("a", 10001)

	t.Run("description too long", func(t *testing.T) {
		f := newProposalTestFixture(t)
		req := f.baseRequest()
		req.Description = tooLong
		_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
		assertFieldError(t, err, "description")
	})

	t.Run("rationale too long", func(t *testing.T) {
		f := newProposalTestFixture(t)
		req := f.baseRequest()
		req.Rationale = tooLong
		_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
		assertFieldError(t, err, "rationale")
	})
}

// TestProposeTask_WorkflowNotInWorkspace covers the workflow clause of
// AC-COORDINATOR-PROPOSALS-001.3, both for an unknown workflow id and a real
// workflow that belongs to a different workspace.
func TestProposeTask_WorkflowNotInWorkspace(t *testing.T) {
	t.Run("unknown workflow", func(t *testing.T) {
		f := newProposalTestFixture(t)
		req := f.baseRequest()
		req.WorkflowID = "wf-does-not-exist"
		_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
		assertFieldError(t, err, "workflow_id")
	})

	t.Run("foreign workspace", func(t *testing.T) {
		f := newProposalTestFixture(t)
		foreign := "wf-foreign"
		f.svc.SetProposalDeps(
			fakeWorkflowReader{workflows: map[string]*taskmodels.Workflow{
				f.workflowID: {ID: f.workflowID, WorkspaceID: f.workspaceID},
				foreign:      {ID: foreign, WorkspaceID: "ws-other"},
			}},
			fakeRepositoryReader{}, fakeSourceTaskReader{},
			fakeWorkflowStepReader{stepsByWorkflow: map[string][]*wfmodels.WorkflowStep{
				foreign: {{ID: "start", IsStartStep: true}},
			}},
		)
		req := f.baseRequest()
		req.WorkflowID = foreign
		_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
		assertFieldError(t, err, "workflow_id")
	})
}

// TestProposeTask_SourceTaskNotInWorkspace covers the source-task clause of
// AC-COORDINATOR-PROPOSALS-001.3.
func TestProposeTask_SourceTaskNotInWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sourceTaskID string
	}{
		{name: "unknown task", sourceTaskID: "task-does-not-exist"},
		{name: "foreign workspace task", sourceTaskID: "task-foreign"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProposalTestFixture(t)
			f.svc.proposalTasks = fakeSourceTaskReader{tasks: map[string]*taskmodels.Task{
				f.sourceTask.ID: f.sourceTask,
				"task-foreign":  {ID: "task-foreign", WorkspaceID: "ws-other"},
			}}
			req := f.baseRequest()
			req.SourceTaskID = tc.sourceTaskID
			_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
			assertFieldError(t, err, "source_task_id")
		})
	}
}

// TestProposeTask_RepositoryNotInWorkspace covers the repository clause of
// AC-COORDINATOR-PROPOSALS-001.3.
func TestProposeTask_RepositoryNotInWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name         string
		repositoryID string
	}{
		{name: "unknown repository", repositoryID: "repo-does-not-exist"},
		{name: "foreign workspace repository", repositoryID: "repo-foreign"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProposalTestFixture(t)
			f.svc.proposalRepositories = fakeRepositoryReader{repositories: map[string]*taskmodels.Repository{
				f.repository.ID: f.repository,
				"repo-foreign":  {ID: "repo-foreign", WorkspaceID: "ws-other"},
			}}
			req := f.baseRequest()
			req.RepositoryID = tc.repositoryID
			_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
			assertFieldError(t, err, "repository_id")
		})
	}
}

// TestProposeTask_StepNotBelongingToWorkflow covers the step-membership
// clause of AC-COORDINATOR-PROPOSALS-001.3.
func TestProposeTask_StepNotBelongingToWorkflow(t *testing.T) {
	f := newProposalTestFixture(t)
	req := f.baseRequest()
	req.StepID = "step-from-another-workflow"
	_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
	assertFieldError(t, err, "step_id")
}

// TestProposeTask_IneligibleStepRefused covers the eligibility clause of
// AC-COORDINATOR-PROPOSALS-001.3: an auto-start step, and a step that feeds
// one directly, are both refused naming "step_id". EligibleStep's own
// exhaustive coverage (direct and transitive feeders) lives in
// eligibility_test.go; this only proves resolveProposalStep wires the real
// step graph into it.
func TestProposeTask_IneligibleStepRefused(t *testing.T) {
	for _, stepID := range []string{"auto", "manual"} {
		t.Run(stepID, func(t *testing.T) {
			f := newProposalTestFixture(t)
			req := f.baseRequest()
			req.StepID = stepID
			_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
			assertFieldError(t, err, "step_id")
		})
	}
}

// TestProposeTask_ManualMoveStepEligible proves a non-start step that
// merely allows manual moves (and is not itself an auto-start feeder chain
// target) is accepted; only "manual" feeding "auto" makes it ineligible, so
// this fixture uses a fresh graph without that edge.
func TestProposeTask_ManualMoveStepEligible(t *testing.T) {
	f := newProposalTestFixture(t)
	f.svc.proposalSteps = fakeWorkflowStepReader{stepsByWorkflow: map[string][]*wfmodels.WorkflowStep{
		f.workflowID: {
			{ID: "start", IsStartStep: true},
			{ID: "manual", AllowManualMove: true},
		},
	}}
	req := f.baseRequest()
	req.StepID = "manual"
	proposal, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
	if err != nil {
		t.Fatalf("ProposeTask() unexpected error: %v", err)
	}
	if proposal.Spec.StepID != "manual" {
		t.Fatalf("proposal.Spec.StepID = %q, want %q", proposal.Spec.StepID, "manual")
	}
}

// TestProposeTask_CapReached covers AC-COORDINATOR-PROPOSALS-001.4: a 26th
// open proposal for the same coordinator is refused.
func TestProposeTask_CapReached(t *testing.T) {
	f := newProposalTestFixture(t)
	ctx := context.Background()
	for i := 0; i < maxOpenProposals; i++ {
		if _, _, err := f.svc.ProposeTask(ctx, f.coordinator.ID, f.baseRequest()); err != nil {
			t.Fatalf("seed ProposeTask() #%d unexpected error: %v", i, err)
		}
	}

	_, _, err := f.svc.ProposeTask(ctx, f.coordinator.ID, f.baseRequest())
	if !errors.Is(err, ErrCoordinatorProposalCapReached) {
		t.Fatalf("ProposeTask() error = %v, want ErrCoordinatorProposalCapReached", err)
	}
}

// TestProposeTask_NoDeduplication covers AC-COORDINATOR-PROPOSALS-001.5: two
// identical calls create two proposals.
func TestProposeTask_NoDeduplication(t *testing.T) {
	f := newProposalTestFixture(t)
	ctx := context.Background()
	req := f.baseRequest()

	first, _, err := f.svc.ProposeTask(ctx, f.coordinator.ID, req)
	if err != nil {
		t.Fatalf("first ProposeTask() unexpected error: %v", err)
	}
	second, openCount, err := f.svc.ProposeTask(ctx, f.coordinator.ID, req)
	if err != nil {
		t.Fatalf("second ProposeTask() unexpected error: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("two identical proposals were assigned the same id")
	}
	if openCount != 2 {
		t.Fatalf("openCount after two proposals = %d, want 2", openCount)
	}
}

// TestProposeTask_UnknownCoordinatorReturnsNotFound proves a coordinator id
// that does not exist fails the same way approve's own missing-row path
// does, rather than panicking on a nil coordinator.
func TestProposeTask_UnknownCoordinatorReturnsNotFound(t *testing.T) {
	f := newProposalTestFixture(t)
	_, _, err := f.svc.ProposeTask(context.Background(), "coordinator-does-not-exist", f.baseRequest())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ProposeTask() error = %v, want ErrNotFound", err)
	}
}

// TestProposeTask_ConcurrentProposesRespectCap runs 30 concurrent proposes
// against a coordinator with 0 open proposals and asserts exactly 25 succeed
// and 5 are refused, matching the store's own dual-dialect concurrency test
// (store_proposals_test.go) but exercised through the service entry point a
// coordinator session actually calls.
func TestProposeTask_ConcurrentProposesRespectCap(t *testing.T) {
	f := newProposalTestFixture(t)
	const attempts = 30

	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded, refused := 0, 0
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, f.baseRequest())
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, ErrCoordinatorProposalCapReached):
				refused++
			default:
				t.Errorf("unexpected ProposeTask() error: %v", err)
			}
		}()
	}
	wg.Wait()

	if succeeded != maxOpenProposals {
		t.Fatalf("succeeded = %d, want %d", succeeded, maxOpenProposals)
	}
	if refused != attempts-maxOpenProposals {
		t.Fatalf("refused = %d, want %d", refused, attempts-maxOpenProposals)
	}
}
