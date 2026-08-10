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
)

type fakePluginTaskWriteService struct {
	lastCreate *taskservice.CreateTaskRequest
	lastUpdate *taskservice.UpdateTaskRequest
	lastID     string
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

func TestPluginsTaskWriter_UpdateMapsFieldMask(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}

	title := "Renamed"
	state := "IN_PROGRESS"
	step := "step-2"
	_, err := a.UpdateTask(context.Background(), plugins.TaskUpdateInput{ID: "task-1", Title: &title, State: &state, WorkflowStepID: &step})
	require.NoError(t, err)
	require.Equal(t, "task-1", svc.lastID)
	require.Equal(t, "Renamed", *svc.lastUpdate.Title)
	require.NotNil(t, svc.lastUpdate.State)
	require.Equal(t, v1.TaskStateInProgress, *svc.lastUpdate.State)
	require.Equal(t, "step-2", *svc.lastUpdate.WorkflowStepID)
	require.Nil(t, svc.lastUpdate.Description, "an unset field stays nil")
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
