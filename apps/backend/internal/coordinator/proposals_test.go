package coordinator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakeWorkflowReader, fakeRepositoryReader and fakeSourceTaskReader are
// keyed-map test doubles for the propose-time readers. A key present in errs
// returns that error (nil row), matching the real services' actual "not
// found" shape for a genuinely missing id (sqlite.Repository.GetTask/
// GetWorkflow/GetRepository wrap a repoerrors sentinel, they don't return
// (nil, nil)). A key absent from both maps returns (nil, nil), matching a
// foreign-workspace row's "found, but wrong workspace" case once the caller
// checks WorkspaceID. buildProposalSpec must turn both into the identical
// *FieldError (SEC-002). fakeWorkflowReader's foreign-workspace fixtures use
// errs, not workflows, ONLY for a scoped caller: the real WorkflowReader (the
// scoped task service) denies a foreign workflow inside GetWorkflow itself
// (authorizeWorkflowID), wrapping the same ErrWorkflowNotFound sentinel as a
// missing id. For an unscoped caller (no identity, or the auth-disabled
// default profile's synthetic identity), authorizeWorkflowID's scope check
// never runs and GetWorkflow returns the raw row, mismatched WorkspaceID
// included — that path uses the workflows map instead, and buildProposalSpec
// has to catch it itself via the WorkspaceID comparison, not via GetWorkflow.
type fakeWorkflowReader struct {
	workflows map[string]*taskmodels.Workflow
	errs      map[string]error
}

func (f fakeWorkflowReader) GetWorkflow(_ context.Context, id string) (*taskmodels.Workflow, error) {
	if err, ok := f.errs[id]; ok {
		return nil, err
	}
	return f.workflows[id], nil
}

type fakeRepositoryReader struct {
	repositories map[string]*taskmodels.Repository
	errs         map[string]error
}

func (f fakeRepositoryReader) GetRepository(_ context.Context, id string) (*taskmodels.Repository, error) {
	if err, ok := f.errs[id]; ok {
		return nil, err
	}
	return f.repositories[id], nil
}

type fakeSourceTaskReader struct {
	tasks map[string]*taskmodels.Task
	errs  map[string]error
}

func (f fakeSourceTaskReader) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	if err, ok := f.errs[id]; ok {
		return nil, err
	}
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

	t.Run("foreign workspace, scoped caller", func(t *testing.T) {
		f := newProposalTestFixture(t)
		foreign := "wf-foreign"
		f.svc.SetProposalDeps(
			fakeWorkflowReader{
				workflows: map[string]*taskmodels.Workflow{
					f.workflowID: {ID: f.workflowID, WorkspaceID: f.workspaceID},
				},
				// A scoped caller never sees a foreign-workspace row:
				// authorizeWorkflowID denies it before GetWorkflow returns,
				// wrapping the same ErrWorkflowNotFound sentinel as a
				// genuinely missing id.
				errs: map[string]error{
					foreign: fmt.Errorf("%w: %s", repoerrors.ErrWorkflowNotFound, foreign),
				},
			},
			fakeRepositoryReader{}, fakeSourceTaskReader{},
			fakeWorkflowStepReader{},
		)
		req := f.baseRequest()
		req.WorkflowID = foreign
		_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
		assertFieldError(t, err, "workflow_id")
	})

	// An unscoped caller (no identity, or the auth-disabled default
	// profile's synthetic identity) bypasses authorizeWorkflowID's scope
	// check entirely, so GetWorkflow returns the raw row with its real,
	// mismatched WorkspaceID. buildProposalSpec's own WorkspaceID comparison
	// is the only thing that catches this case.
	t.Run("foreign workspace, unscoped caller", func(t *testing.T) {
		f := newProposalTestFixture(t)
		foreign := "wf-foreign"
		f.svc.SetProposalDeps(
			fakeWorkflowReader{
				workflows: map[string]*taskmodels.Workflow{
					f.workflowID: {ID: f.workflowID, WorkspaceID: f.workspaceID},
					foreign:      {ID: foreign, WorkspaceID: "ws-other"},
				},
			},
			fakeRepositoryReader{}, fakeSourceTaskReader{},
			fakeWorkflowStepReader{},
		)
		req := f.baseRequest()
		req.WorkflowID = foreign
		_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
		assertFieldError(t, err, "workflow_id")
	})

	// An orphaned-workspace workflow (the workspace row itself is gone)
	// collapses to the same ErrWorkflowNotFound sentinel as a foreign or
	// missing workflow (internal/task/service.authorizeWorkflowID), so it
	// must refuse identically here too.
	t.Run("orphaned workspace", func(t *testing.T) {
		f := newProposalTestFixture(t)
		orphan := "wf-orphaned"
		f.svc.SetProposalDeps(
			fakeWorkflowReader{
				workflows: map[string]*taskmodels.Workflow{
					f.workflowID: {ID: f.workflowID, WorkspaceID: f.workspaceID},
				},
				errs: map[string]error{
					orphan: fmt.Errorf("%w: %s", repoerrors.ErrWorkflowNotFound, orphan),
				},
			},
			fakeRepositoryReader{}, fakeSourceTaskReader{},
			fakeWorkflowStepReader{},
		)
		req := f.baseRequest()
		req.WorkflowID = orphan
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

// TestProposeTask_NotFoundAndForeignWorkspaceProduceIdenticalFieldError
// covers SEC-002: a genuinely-missing referenced id and a foreign-workspace
// id must be indistinguishable to the caller. The real task-service readers
// return a non-nil error wrapping a repoerrors "not found" sentinel for a
// missing row, not (nil, nil); buildProposalSpec propagated that as a
// generic wrapped error while a foreign-workspace row (found, wrong
// workspace) became a *FieldError, letting a caller probe workspace
// membership of another workspace's ids from the response shape alone.
func TestProposeTask_NotFoundAndForeignWorkspaceProduceIdenticalFieldError(t *testing.T) {
	assertSameFieldError := func(t *testing.T, foreignErr, missingErr error) {
		t.Helper()
		var foreignFieldErr *FieldError
		if !errors.As(foreignErr, &foreignFieldErr) {
			t.Fatalf("foreign-workspace error = %v, want a *FieldError", foreignErr)
		}
		var missingFieldErr *FieldError
		if !errors.As(missingErr, &missingFieldErr) {
			t.Fatalf("missing-id error = %v (%T), want a *FieldError like the foreign-workspace case", missingErr, missingErr)
		}
		if foreignFieldErr.Field != missingFieldErr.Field || foreignFieldErr.Message != missingFieldErr.Message {
			t.Fatalf("foreign-workspace FieldError = %+v, missing-id FieldError = %+v, want identical", foreignFieldErr, missingFieldErr)
		}
	}

	t.Run("workflow_id", func(t *testing.T) {
		f := newProposalTestFixture(t)
		foreign := "wf-foreign"
		f.svc.SetProposalDeps(
			fakeWorkflowReader{
				workflows: map[string]*taskmodels.Workflow{
					f.workflowID: {ID: f.workflowID, WorkspaceID: f.workspaceID},
				},
				// The real WorkflowReader (the scoped task service) collapses a
				// foreign-workspace workflow to the same ErrWorkflowNotFound
				// sentinel as a genuinely missing id (authorizeWorkflowID denies
				// before GetWorkflow returns), so both keys use the errs map.
				errs: map[string]error{
					foreign:      fmt.Errorf("%w: %s", repoerrors.ErrWorkflowNotFound, foreign),
					"wf-missing": fmt.Errorf("%w: wf-missing", repoerrors.ErrWorkflowNotFound),
				},
			},
			fakeRepositoryReader{}, fakeSourceTaskReader{},
			fakeWorkflowStepReader{},
		)

		reqForeign := f.baseRequest()
		reqForeign.WorkflowID = foreign
		_, _, foreignErr := f.svc.ProposeTask(context.Background(), f.coordinator.ID, reqForeign)

		reqMissing := f.baseRequest()
		reqMissing.WorkflowID = "wf-missing"
		_, _, missingErr := f.svc.ProposeTask(context.Background(), f.coordinator.ID, reqMissing)

		assertSameFieldError(t, foreignErr, missingErr)
	})

	t.Run("source_task_id", func(t *testing.T) {
		f := newProposalTestFixture(t)
		f.svc.proposalTasks = fakeSourceTaskReader{
			tasks: map[string]*taskmodels.Task{
				f.sourceTask.ID: f.sourceTask,
				"task-foreign":  {ID: "task-foreign", WorkspaceID: "ws-other"},
			},
			errs: map[string]error{
				"task-missing": fmt.Errorf("%w: task-missing", repoerrors.ErrTaskNotFound),
			},
		}

		reqForeign := f.baseRequest()
		reqForeign.SourceTaskID = "task-foreign"
		_, _, foreignErr := f.svc.ProposeTask(context.Background(), f.coordinator.ID, reqForeign)

		reqMissing := f.baseRequest()
		reqMissing.SourceTaskID = "task-missing"
		_, _, missingErr := f.svc.ProposeTask(context.Background(), f.coordinator.ID, reqMissing)

		assertSameFieldError(t, foreignErr, missingErr)
	})

	t.Run("repository_id", func(t *testing.T) {
		f := newProposalTestFixture(t)
		f.svc.proposalRepositories = fakeRepositoryReader{
			repositories: map[string]*taskmodels.Repository{
				f.repository.ID: f.repository,
				"repo-foreign":  {ID: "repo-foreign", WorkspaceID: "ws-other"},
			},
			errs: map[string]error{
				"repo-missing": fmt.Errorf("%w: repo-missing", repoerrors.ErrRepositoryNotFound),
			},
		}

		reqForeign := f.baseRequest()
		reqForeign.RepositoryID = "repo-foreign"
		_, _, foreignErr := f.svc.ProposeTask(context.Background(), f.coordinator.ID, reqForeign)

		reqMissing := f.baseRequest()
		reqMissing.RepositoryID = "repo-missing"
		_, _, missingErr := f.svc.ProposeTask(context.Background(), f.coordinator.ID, reqMissing)

		assertSameFieldError(t, foreignErr, missingErr)
	})
}

// TestProposeTask_EveryNotFoundOrUnauthorizedBranchIsIndistinguishable is the
// class-level regression for SEC-002/R2-A/R3-A: propose_task's four
// workspace-scoped references (workflow_id, repository_id, source_task_id,
// step_id) must each produce the identical *FieldError no matter which
// underlying reason the reference failed to resolve for — missing
// altogether, belonging to a foreign-but-reachable workspace (both as a
// scoped caller sees it, denied inside the task-service authorize helpers,
// and as an unscoped/default-profile caller sees it, caught only by this
// package's own WorkspaceID comparison), or belonging to an orphaned
// workspace whose row no longer exists. proposeTaskErrorResponse
// (internal/mcp/handlers/coordinator_propose.go) maps any *FieldError to
// ws.ErrorCodeValidation regardless of field, a mapping already covered
// directly by TestHandleProposeTask_FieldErrorMapsToValidation, so proving
// every branch below produces a *FieldError (never some other error
// escaping as ws.ErrorCodeInternalError instead, which is exactly what
// R3-A's orphaned-workspace branch did) proves the HTTP status is identical
// across every branch too, without needing to duplicate that mapping test
// per field.
func TestProposeTask_EveryNotFoundOrUnauthorizedBranchIsIndistinguishable(t *testing.T) {
	type branch struct {
		name  string
		setup func(f proposalTestFixture, req *ProposeTaskRequest)
	}

	requireFieldError := func(t *testing.T, err error) *FieldError {
		t.Helper()
		var fieldErr *FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("error = %v (%T), want a *FieldError (anything else maps to ws.ErrorCodeInternalError at the boundary, not ws.ErrorCodeValidation)", err, err)
		}
		return fieldErr
	}

	runBranches := func(t *testing.T, field string, branches []branch) {
		t.Helper()
		results := make([]*FieldError, 0, len(branches))
		for _, b := range branches {
			t.Run(b.name, func(t *testing.T) {
				f := newProposalTestFixture(t)
				req := f.baseRequest()
				b.setup(f, &req)
				_, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req)
				fieldErr := requireFieldError(t, err)
				if fieldErr.Field != field {
					t.Fatalf("FieldError.Field = %q, want %q", fieldErr.Field, field)
				}
				results = append(results, fieldErr)
			})
		}
		for i := 1; i < len(results); i++ {
			if results[i].Message != results[0].Message {
				t.Errorf("branch %q message = %q, want identical to branch %q message %q",
					branches[i].name, results[i].Message, branches[0].name, results[0].Message)
			}
		}
	}

	t.Run("workflow_id", func(t *testing.T) {
		runBranches(t, "workflow_id", []branch{
			{"missing", func(f proposalTestFixture, req *ProposeTaskRequest) {
				req.WorkflowID = "wf-missing"
			}},
			{"foreign workspace, scoped caller", func(f proposalTestFixture, req *ProposeTaskRequest) {
				f.svc.proposalWorkflows = fakeWorkflowReader{
					workflows: map[string]*taskmodels.Workflow{f.workflowID: {ID: f.workflowID, WorkspaceID: f.workspaceID}},
					errs: map[string]error{
						"wf-foreign-scoped": fmt.Errorf("%w: wf-foreign-scoped", repoerrors.ErrWorkflowNotFound),
					},
				}
				req.WorkflowID = "wf-foreign-scoped"
			}},
			{"foreign workspace, unscoped caller", func(f proposalTestFixture, req *ProposeTaskRequest) {
				f.svc.proposalWorkflows = fakeWorkflowReader{
					workflows: map[string]*taskmodels.Workflow{
						f.workflowID:          {ID: f.workflowID, WorkspaceID: f.workspaceID},
						"wf-foreign-unscoped": {ID: "wf-foreign-unscoped", WorkspaceID: "ws-other"},
					},
				}
				req.WorkflowID = "wf-foreign-unscoped"
			}},
			{"orphaned workspace", func(f proposalTestFixture, req *ProposeTaskRequest) {
				f.svc.proposalWorkflows = fakeWorkflowReader{
					workflows: map[string]*taskmodels.Workflow{f.workflowID: {ID: f.workflowID, WorkspaceID: f.workspaceID}},
					errs: map[string]error{
						"wf-orphaned": fmt.Errorf("%w: wf-orphaned", repoerrors.ErrWorkflowNotFound),
					},
				}
				req.WorkflowID = "wf-orphaned"
			}},
		})
	})

	t.Run("repository_id", func(t *testing.T) {
		runBranches(t, "repository_id", []branch{
			{"missing", func(f proposalTestFixture, req *ProposeTaskRequest) {
				req.RepositoryID = "repo-missing"
			}},
			{"foreign workspace", func(f proposalTestFixture, req *ProposeTaskRequest) {
				f.svc.proposalRepositories = fakeRepositoryReader{
					repositories: map[string]*taskmodels.Repository{
						f.repository.ID: f.repository,
						"repo-foreign":  {ID: "repo-foreign", WorkspaceID: "ws-other"},
					},
				}
				req.RepositoryID = "repo-foreign"
			}},
			{"orphaned workspace", func(f proposalTestFixture, req *ProposeTaskRequest) {
				f.svc.proposalRepositories = fakeRepositoryReader{
					repositories: map[string]*taskmodels.Repository{f.repository.ID: f.repository},
					errs: map[string]error{
						"repo-orphaned": fmt.Errorf("%w: repo-orphaned", repoerrors.ErrRepositoryNotFound),
					},
				}
				req.RepositoryID = "repo-orphaned"
			}},
		})
	})

	t.Run("source_task_id", func(t *testing.T) {
		runBranches(t, "source_task_id", []branch{
			{"missing", func(f proposalTestFixture, req *ProposeTaskRequest) {
				req.SourceTaskID = "task-missing"
			}},
			{"foreign workspace", func(f proposalTestFixture, req *ProposeTaskRequest) {
				f.svc.proposalTasks = fakeSourceTaskReader{
					tasks: map[string]*taskmodels.Task{
						f.sourceTask.ID: f.sourceTask,
						"task-foreign":  {ID: "task-foreign", WorkspaceID: "ws-other"},
					},
				}
				req.SourceTaskID = "task-foreign"
			}},
			{"orphaned workspace", func(f proposalTestFixture, req *ProposeTaskRequest) {
				f.svc.proposalTasks = fakeSourceTaskReader{
					tasks: map[string]*taskmodels.Task{f.sourceTask.ID: f.sourceTask},
					errs: map[string]error{
						"task-orphaned": fmt.Errorf("%w: task-orphaned", repoerrors.ErrTaskNotFound),
					},
				}
				req.SourceTaskID = "task-orphaned"
			}},
		})
	})

	// step_id has no separate foreign/orphaned shape to walk: unlike the
	// other three fields it is not resolved through a workspace-scoped
	// reader at all. resolveProposalStep checks membership in the already
	// workspace-validated workflow's own step list
	// (proposalStepBelongsToWorkflow), so a step id belonging to a real but
	// different workflow and a step id that does not exist anywhere both
	// fail that same single membership check and must produce the identical
	// FieldError.
	t.Run("step_id", func(t *testing.T) {
		runBranches(t, ApproveFieldStepID, []branch{
			{"belongs to a different workflow", func(f proposalTestFixture, req *ProposeTaskRequest) {
				req.StepID = "step-from-another-workflow"
			}},
			{"does not exist anywhere", func(f proposalTestFixture, req *ProposeTaskRequest) {
				req.StepID = "step-does-not-exist-anywhere"
			}},
		})
	})
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
