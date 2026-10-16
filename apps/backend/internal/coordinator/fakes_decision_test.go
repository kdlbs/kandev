package coordinator

import (
	"context"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// fakeDecisionTaskService is a test double for DecisionTaskService, used by
// spec-validation and (later) approve/reject tests. Missing ids fall back to
// the sentinel a real *taskservice.Service would return; an entry in the
// matching *Err map overrides that for tests that need a plain read error.
type fakeDecisionTaskService struct {
	workflows map[string]*taskmodels.Workflow
	repos     map[string]*taskmodels.Repository
	tasks     map[string]*taskmodels.Task

	workflowErr map[string]error
	repoErr     map[string]error
	taskErr     map[string]error

	lookupByExternalID map[string]*taskmodels.Task
	lookupErr          map[string]error

	createResult taskservice.CreateTaskResult
	createErr    error
	createCalls  []*taskservice.CreateTaskRequest

	// stepByID, stepErr, workspaces and workspaceErr drive the created-task
	// agent chain. A step not in stepByID reads as a step that names its own
	// agent, so tests that do not care see no addition.
	stepByID     map[string]*wfmodels.WorkflowStep
	stepErr      map[string]error
	workspaces   map[string]*taskmodels.Workspace
	workspaceErr map[string]error

	settled     bool
	survivor    *taskmodels.Task
	settleErr   error
	settleCalls []settleCall
}

// settleCall records one SettleExternalID invocation for assertions on which
// task/external-id pair was settled and how many times.
type settleCall struct {
	taskID     string
	externalID string
}

func (f *fakeDecisionTaskService) GetWorkflow(_ context.Context, id string) (*taskmodels.Workflow, error) {
	if err, ok := f.workflowErr[id]; ok {
		return nil, err
	}
	wf, ok := f.workflows[id]
	if !ok {
		return nil, repoerrors.ErrWorkflowNotFound
	}
	return wf, nil
}

func (f *fakeDecisionTaskService) GetRepository(_ context.Context, id string) (*taskmodels.Repository, error) {
	if err, ok := f.repoErr[id]; ok {
		return nil, err
	}
	r, ok := f.repos[id]
	if !ok {
		return nil, repoerrors.ErrRepositoryNotFound
	}
	return r, nil
}

func (f *fakeDecisionTaskService) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	if err, ok := f.taskErr[id]; ok {
		return nil, err
	}
	t, ok := f.tasks[id]
	if !ok {
		return nil, repoerrors.ErrTaskNotFound
	}
	return t, nil
}

func (f *fakeDecisionTaskService) CreateTask(_ context.Context, req *taskservice.CreateTaskRequest) (taskservice.CreateTaskResult, error) {
	f.createCalls = append(f.createCalls, req)
	return f.createResult, f.createErr
}

func (f *fakeDecisionTaskService) SettleExternalID(_ context.Context, taskID, externalID string) (bool, *taskmodels.Task, error) {
	f.settleCalls = append(f.settleCalls, settleCall{taskID: taskID, externalID: externalID})
	return f.settled, f.survivor, f.settleErr
}

func (f *fakeDecisionTaskService) GetTaskByExternalID(_ context.Context, _, externalID string) (*taskmodels.Task, error) {
	if err, ok := f.lookupErr[externalID]; ok {
		return nil, err
	}
	t, ok := f.lookupByExternalID[externalID]
	if !ok {
		return nil, repoerrors.ErrTaskNotFound
	}
	return t, nil
}

func (f *fakeDecisionTaskService) GetWorkflowStep(_ context.Context, id string) (*wfmodels.WorkflowStep, error) {
	if err, ok := f.stepErr[id]; ok {
		return nil, err
	}
	if st, ok := f.stepByID[id]; ok {
		return st, nil
	}
	return &wfmodels.WorkflowStep{ID: id, AgentProfileID: "step-agent"}, nil
}

func (f *fakeDecisionTaskService) GetWorkspace(_ context.Context, id string) (*taskmodels.Workspace, error) {
	if err, ok := f.workspaceErr[id]; ok {
		return nil, err
	}
	if w, ok := f.workspaces[id]; ok {
		return w, nil
	}
	return &taskmodels.Workspace{ID: id}, nil
}

var _ DecisionTaskService = (*fakeDecisionTaskService)(nil)
