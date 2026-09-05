package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type fakePluginTaskWriteService struct {
	lastCreate *taskservice.CreateTaskRequest
	lastUpdate *taskservice.UpdateTaskRequest
	lastID     string
	deletedID  string

	getTaskResult *taskmodels.Task
	getTaskErr    error
	lastGetTaskID string

	moveResult   *taskservice.MoveTaskResult
	moveErr      error
	moveCalls    int
	lastMoveID   string
	lastMoveWfID string
	lastMoveStep string
	lastMovePos  int
	lastMoveOpts taskservice.MoveTaskOptions
	lastMoveCtx  context.Context
}

func (f *fakePluginTaskWriteService) CreateTask(_ context.Context, req *taskservice.CreateTaskRequest) (taskservice.CreateTaskResult, error) {
	f.lastCreate = req
	task := &taskmodels.Task{ID: "task-1", WorkspaceID: req.WorkspaceID, WorkflowID: req.WorkflowID, Title: req.Title}
	return taskservice.CreateTaskResult{Task: task, Outcome: taskservice.CreateTaskOutcomeCreated}, nil
}

func (f *fakePluginTaskWriteService) UpdateTask(_ context.Context, id string, req *taskservice.UpdateTaskRequest) (*taskmodels.Task, error) {
	f.lastID = id
	f.lastUpdate = req
	return &taskmodels.Task{ID: id}, nil
}

func (f *fakePluginTaskWriteService) DeleteTask(_ context.Context, id string) error {
	f.deletedID = id
	return nil
}

func (f *fakePluginTaskWriteService) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	f.lastGetTaskID = id
	if f.getTaskErr != nil {
		return nil, f.getTaskErr
	}
	if f.getTaskResult != nil {
		return f.getTaskResult, nil
	}
	return &taskmodels.Task{ID: id}, nil
}

func (f *fakePluginTaskWriteService) MoveTaskWithOptions(ctx context.Context, id, workflowID, workflowStepID string, position int, opts taskservice.MoveTaskOptions) (*taskservice.MoveTaskResult, error) {
	f.moveCalls++
	f.lastMoveCtx = ctx
	f.lastMoveID = id
	f.lastMoveWfID = workflowID
	f.lastMoveStep = workflowStepID
	f.lastMovePos = position
	f.lastMoveOpts = opts
	if f.moveErr != nil {
		return nil, f.moveErr
	}
	if f.moveResult != nil {
		return f.moveResult, nil
	}
	return &taskservice.MoveTaskResult{
		Task:         &taskmodels.Task{ID: id, WorkflowID: workflowID, WorkflowStepID: workflowStepID},
		Transitioned: true,
		FromStepID:   "step-from",
	}, nil
}

func TestPluginsTaskWriter_CreateMapsSourceToMetadata(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}

	_, err := a.CreateTask(context.Background(), plugins.TaskCreateInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Title: "Investigate", Description: "details", ParentID: "parent-1", Source: "plugin:acme",
	})
	require.NoError(t, err)
	require.Equal(t, "ws-1", svc.lastCreate.WorkspaceID)
	require.Equal(t, "wf-1", svc.lastCreate.WorkflowID)
	require.Equal(t, "step-1", svc.lastCreate.WorkflowStepID)
	require.Equal(t, "parent-1", svc.lastCreate.ParentID)
	require.Equal(t, "plugin:acme", svc.lastCreate.Metadata["source"], "provenance is stamped into task metadata")
}

func TestPluginsTaskWriter_CreateWithoutSourceOmitsMetadata(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}

	_, err := a.CreateTask(context.Background(), plugins.TaskCreateInput{WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "x"})
	require.NoError(t, err)
	require.Nil(t, svc.lastCreate.Metadata, "no source → no metadata map")
}

func TestPluginsTaskWriter_CreateMapsRichPluginTaskInput(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}
	base, checkout := "main", "pr-42"
	pullRequest := int64(42)

	_, err := a.CreateTask(context.Background(), plugins.TaskCreateInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Investigate", Source: "plugin:acme", PlanMode: true,
		Metadata: map[string]any{"source": "plugin:acme", "plugin:acme": map[string]any{"watch_id": "watch-1"}},
		Repositories: []pluginsdk.PluginTaskRepository{{
			Remote: &pluginsdk.RemoteRepositoryDescriptor{
				ProviderID: "custom-provider", ProviderHost: "https://forge.example.test",
				OwnerOrProject: "TEAM", ProviderRepositoryID: "repo-99", Name: "widgets",
				CloneURL:      "https://forge.example.test/context/scm/TEAM/widgets.git",
				DefaultBranch: &base, HeadBranch: &checkout, PullRequestNumber: &pullRequest,
			},
		}},
	})
	require.NoError(t, err)
	require.True(t, svc.lastCreate.PlanMode)
	require.Equal(t, "plugin:acme", svc.lastCreate.Metadata["source"])
	require.Equal(t, map[string]any{"watch_id": "watch-1"}, svc.lastCreate.Metadata["plugin:acme"])
	require.Len(t, svc.lastCreate.Repositories, 1)
	got := svc.lastCreate.Repositories[0]
	require.True(t, got.TrustedProviderDescriptor)
	require.Equal(t, "custom-provider", got.Provider)
	require.Equal(t, "https://forge.example.test", got.ProviderHost)
	require.Equal(t, "https://forge.example.test/context/scm/TEAM/widgets.git", got.RemoteURL)
	require.Equal(t, "pr-42", got.CheckoutBranch)
	require.Equal(t, 42, got.PRNumber)
}

func TestPluginsTaskWriter_DeleteRoutesThroughTaskService(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	require.NoError(t, (pluginsTaskWriterAdapter{svc: svc}).DeleteTask(context.Background(), "task-1"))
	require.Equal(t, "task-1", svc.deletedID)
}

func TestPluginsTaskWriter_UpdateMapsFieldMask(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}

	title := "Renamed"
	state := "IN_PROGRESS"
	_, err := a.UpdateTask(context.Background(), plugins.TaskUpdateInput{ID: "task-1", Title: &title, State: &state})
	require.NoError(t, err)
	require.Equal(t, "task-1", svc.lastID)
	require.Equal(t, "Renamed", *svc.lastUpdate.Title)
	require.NotNil(t, svc.lastUpdate.State)
	require.Equal(t, v1.TaskStateInProgress, *svc.lastUpdate.State)
	require.Nil(t, svc.lastUpdate.Description, "an unset field stays nil")
}

// TestPluginsTaskWriter_UpdateRejectsWorkflowStepID pins AC-004.3: UpdateTask
// never calls publishTaskMovedEvent, so writing workflow_step_id directly
// through it would move the card without firing on_enter actions like
// auto-start. A plugin must use MoveTask instead.
func TestPluginsTaskWriter_UpdateRejectsWorkflowStepID(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}

	step := "step-2"
	_, err := a.UpdateTask(context.Background(), plugins.TaskUpdateInput{ID: "task-1", WorkflowStepID: &step})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Nil(t, svc.lastUpdate, "the service is never called when workflow_step_id is present")
}

// TestPluginsTaskWriter_UpdateRejectsEmptyWorkflowStepID pins that the
// rejection fires on presence, not on a non-empty value — a plugin cannot
// smuggle a move through by clearing the field to "".
func TestPluginsTaskWriter_UpdateRejectsEmptyWorkflowStepID(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}

	empty := ""
	_, err := a.UpdateTask(context.Background(), plugins.TaskUpdateInput{ID: "task-1", WorkflowStepID: &empty})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Nil(t, svc.lastUpdate)
}

// TestPluginsTaskWriter_UpdateWorkflowStepIDGuardRunsBeforeStateValidation
// pins AC-004.3's guard ordering: the workflow_step_id-presence rejection
// must be the first check in UpdateTask, before state validation, so a
// request carrying both an invalid state AND workflow_step_id is named as a
// rejected move (not a state error) — per the doc comment on that check. Both
// TestPluginsTaskWriter_UpdateRejectsWorkflowStepID and
// TestPluginsTaskWriter_UpdateRejectsUnknownState alone only pin
// codes.InvalidArgument, which either check alone satisfies — this test
// combines both invalid fields in one request and asserts on the message to
// prove which check actually fired, so a future reorder that runs state
// validation first would fail this test even though the status code is
// unchanged.
func TestPluginsTaskWriter_UpdateWorkflowStepIDGuardRunsBeforeStateValidation(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}

	step := "step-2"
	badState := "NOT_A_STATE"
	_, err := a.UpdateTask(context.Background(), plugins.TaskUpdateInput{
		ID: "task-1", WorkflowStepID: &step, State: &badState,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "workflow_step_id",
		"the workflow_step_id guard must fire first, not the state-validation error")
	require.Nil(t, svc.lastUpdate, "the service is never called when workflow_step_id is present")
}

func TestPluginsTaskWriter_UpdateRejectsUnknownState(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}

	bad := "NOT_A_STATE"
	_, err := a.UpdateTask(context.Background(), plugins.TaskUpdateInput{ID: "task-1", State: &bad})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "a bogus state must be rejected before reaching the service")
	require.Nil(t, svc.lastUpdate, "the service is never called with an invalid state")
}

// TestPluginsTaskWriter_UpdateRejectsSchedulingState pins that the
// orchestrator-owned SCHEDULING transient is not plugin-settable.
func TestPluginsTaskWriter_UpdateRejectsSchedulingState(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}

	scheduling := string(v1.TaskStateScheduling)
	_, err := a.UpdateTask(context.Background(), plugins.TaskUpdateInput{ID: "task-1", State: &scheduling})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Nil(t, svc.lastUpdate)
}

// A plugin that creates a task with start_agent launches it right after
// CreateTask returns, so its create carries the same start intent the REST, WS
// and MCP surfaces carry — otherwise step resolution parks the task on the
// start step and the plugin's agent runs in a column configured to run nothing.
func TestPluginsTaskWriter_CreateCarriesStartAgentIntent(t *testing.T) {
	for name, startAgent := range map[string]bool{
		"starting an agent": true,
		"create only":       false,
	} {
		t.Run(name, func(t *testing.T) {
			svc := &fakePluginTaskWriteService{}
			a := pluginsTaskWriterAdapter{svc: svc}

			_, err := a.CreateTask(context.Background(), plugins.TaskCreateInput{
				WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "x", StartAgent: startAgent,
			})
			require.NoError(t, err)
			require.Equal(t, startAgent, svc.lastCreate.StartAgent)
		})
	}
}
