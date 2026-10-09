package backendapp

import (
	"context"
	"testing"
	"time"

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
	claimResult   *taskmodels.TaskManagementClaim
	claimErr      error

	moveResult         *taskservice.MoveTaskResult
	moveErr            error
	moveCalls          int
	moveAlreadyApplied bool
	lastMoveID         string
	lastMoveWfID       string
	lastMoveStep       string
	lastMovePos        int
	lastMoveOpts       taskservice.MoveTaskOptions
	lastMoveCtx        context.Context

	exactRequest        taskservice.ExactTaskUpdateRequest
	exactResult         *taskservice.ExactTaskUpdateResult
	exactErr            error
	exactAlreadyApplied bool
	archiveRequest      taskservice.ExactTaskArchiveRequest
	archiveResult       *taskservice.ExactTaskArchiveResult
	archiveErr          error
	settledTaskID       string
	settledExternalID   string
	settleErr           error
	settleOK            bool
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
		task := *f.getTaskResult
		if task.WorkspaceID == "" {
			task.WorkspaceID = "ws-1"
		}
		if task.UpdatedAt.IsZero() {
			task.UpdatedAt = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
		}
		return &task, nil
	}
	return &taskmodels.Task{
		ID: id, WorkspaceID: "ws-1", UpdatedAt: time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakePluginTaskWriteService) GetTaskManagementClaim(context.Context, string) (*taskmodels.TaskManagementClaim, error) {
	return f.claimResult, f.claimErr
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
		Task:           &taskmodels.Task{ID: id, WorkflowID: workflowID, WorkflowStepID: workflowStepID},
		AlreadyApplied: f.moveAlreadyApplied,
		Transitioned:   true,
		FromStepID:     "step-from",
	}, nil
}

func TestPluginsTaskWriter_ExactMoveMapsOperationAndVersion(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	updated, alreadyApplied, err := (pluginsTaskWriterAdapter{svc: svc}).MoveTaskExact(context.Background(), plugins.ExactTaskMoveInput{
		TaskID: "task-exact-move", WorkspaceID: "workspace-exact-move",
		ExpectedResourceVersion: "2026-09-26T11:00:00Z", OperationID: "operation-exact-move",
		PayloadDigest: "sha256:exact-move-digest",
		Move: plugins.TaskMoveInput{
			TaskID: "task-exact-move", WorkflowID: stringPointer("workflow-target"),
			WorkflowStepID: "step-target", Position: 4, Source: "plugin:coordinator",
		},
	})
	require.NoError(t, err)
	require.False(t, alreadyApplied)
	require.Equal(t, "task-exact-move", updated.ID)
	require.Equal(t, 1, svc.moveCalls)
	require.Equal(t, "task-exact-move", svc.lastMoveID)
	require.Equal(t, "workflow-target", svc.lastMoveWfID)
	require.Equal(t, "step-target", svc.lastMoveStep)
	require.Equal(t, 4, svc.lastMovePos)
	require.Equal(t, taskservice.ExactTaskMoveOperation{
		WorkspaceID: "workspace-exact-move", ExpectedResourceVersion: "2026-09-26T11:00:00Z",
		OperationID: "operation-exact-move", PayloadDigest: "sha256:exact-move-digest",
	}, *svc.lastMoveOpts.ExactOperation)
	require.NotNil(t, svc.lastMoveOpts.AlreadyApplied)
}

func TestPluginsTaskWriter_ExactArchiveMapsOperationAndVersion(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	archived, alreadyApplied, err := (pluginsTaskWriterAdapter{svc: svc}).ArchiveTaskExact(context.Background(), plugins.ExactTaskArchiveInput{
		TaskID: "task-exact-archive", WorkspaceID: "workspace-exact-archive",
		ExpectedResourceVersion: "2026-09-26T11:00:00Z", OperationID: "operation-exact-archive",
		PayloadDigest: "sha256:exact-archive-digest",
	})
	require.NoError(t, err)
	require.False(t, alreadyApplied)
	require.Equal(t, "task-exact-archive", archived.ID)
	require.Equal(t, taskservice.ExactTaskArchiveRequest{
		WorkspaceID: "workspace-exact-archive", ExpectedResourceVersion: "2026-09-26T11:00:00Z",
		OperationID: "operation-exact-archive", PayloadDigest: "sha256:exact-archive-digest",
	}, svc.archiveRequest)
}

func (f *fakePluginTaskWriteService) UpdateTaskExact(_ context.Context, id string, req taskservice.ExactTaskUpdateRequest) (*taskservice.ExactTaskUpdateResult, error) {
	f.lastID = id
	f.exactRequest = req
	if f.exactErr != nil {
		return nil, f.exactErr
	}
	if f.exactResult != nil {
		return f.exactResult, nil
	}
	return &taskservice.ExactTaskUpdateResult{
		Task: &taskmodels.Task{ID: id, WorkspaceID: req.WorkspaceID}, AlreadyApplied: f.exactAlreadyApplied,
	}, nil
}

func (f *fakePluginTaskWriteService) ArchiveTaskExact(_ context.Context, id string, req taskservice.ExactTaskArchiveRequest) (*taskservice.ExactTaskArchiveResult, error) {
	f.lastID = id
	f.archiveRequest = req
	if f.archiveErr != nil {
		return nil, f.archiveErr
	}
	if f.archiveResult != nil {
		return f.archiveResult, nil
	}
	archivedAt := time.Now().UTC()
	return &taskservice.ExactTaskArchiveResult{
		Task: &taskmodels.Task{ID: id, WorkspaceID: req.WorkspaceID, ArchivedAt: &archivedAt},
	}, nil
}

func (f *fakePluginTaskWriteService) SettleExternalID(_ context.Context, taskID, externalID string) (bool, *taskmodels.Task, error) {
	f.settledTaskID = taskID
	f.settledExternalID = externalID
	if f.settleErr != nil {
		return false, nil, f.settleErr
	}
	if !f.settleOK {
		return false, &taskmodels.Task{ID: taskID, WorkspaceID: "ws-1"}, nil
	}
	return true, nil, nil
}

func TestPluginsTaskWriter_CreateMapsSourceToMetadata(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	a := pluginsTaskWriterAdapter{svc: svc}

	_, err := a.CreateTask(context.Background(), plugins.TaskCreateInput{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Title: "Investigate", Description: "details", ParentID: "parent-1", Source: "plugin:acme", Priority: "critical",
	})
	require.NoError(t, err)
	require.Equal(t, "ws-1", svc.lastCreate.WorkspaceID)
	require.Equal(t, "wf-1", svc.lastCreate.WorkflowID)
	require.Equal(t, "step-1", svc.lastCreate.WorkflowStepID)
	require.Equal(t, "parent-1", svc.lastCreate.ParentID)
	require.Equal(t, "plugin:acme", svc.lastCreate.Metadata["source"], "provenance is stamped into task metadata")
	require.Equal(t, "critical", svc.lastCreate.Priority)
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
	priority := "low"
	_, err := a.UpdateTask(context.Background(), plugins.TaskUpdateInput{ID: "task-1", Title: &title, State: &state, Priority: &priority})
	require.NoError(t, err)
	require.Equal(t, "task-1", svc.lastID)
	require.Equal(t, "Renamed", *svc.exactRequest.Title)
	require.Equal(t, v1.TaskStateInProgress, *svc.exactRequest.State)
	require.Equal(t, "low", *svc.exactRequest.Priority)
	require.Nil(t, svc.exactRequest.Description, "an unset field stays nil")
	require.Empty(t, svc.exactRequest.ClaimFence, "legacy writes have no claim identity and are allowed only while unclaimed")
	require.NotEmpty(t, svc.exactRequest.OperationID)
	require.NotEmpty(t, svc.exactRequest.PayloadDigest)
}

func TestPluginsTaskWriter_LegacyUpdateUsesCurrentReleasedClaimGeneration(t *testing.T) {
	svc := &fakePluginTaskWriteService{claimResult: &taskmodels.TaskManagementClaim{Generation: 4}}
	title := "Updated after release"
	_, err := (pluginsTaskWriterAdapter{svc: svc}).UpdateTask(context.Background(), plugins.TaskUpdateInput{ID: "task-1", Title: &title})
	require.NoError(t, err)
	require.Equal(t, int64(4), svc.exactRequest.ClaimFence.Generation)
}

func TestPluginsTaskWriter_LegacyUpdateRejectsActiveManager(t *testing.T) {
	svc := &fakePluginTaskWriteService{claimResult: &taskmodels.TaskManagementClaim{
		OwnerKind: "plugin", InstallationID: "coordinator", InstanceKey: "run-1", Generation: 2,
	}}
	title := "Must not update"
	_, err := (pluginsTaskWriterAdapter{svc: svc}).UpdateTask(context.Background(), plugins.TaskUpdateInput{ID: "task-1", Title: &title})
	require.Equal(t, codes.Aborted, status.Code(err))
	require.Empty(t, svc.exactRequest.OperationID)
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

func TestPluginsTaskWriter_ExactUpdateMapsLabelsAndAssignee(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	labels := []string{"urgent", "coordination"}
	assignee := "user-42"
	updated, alreadyApplied, err := (pluginsTaskWriterAdapter{svc: svc}).UpdateTaskExact(context.Background(), plugins.ExactTaskUpdateInput{
		TaskID: "task-1", WorkspaceID: "ws-1", ExpectedResourceVersion: "2026-09-26T10:00:00Z",
		OperationID: "operation-1", PayloadDigest: "sha256:labels", Labels: &labels, AssigneeUserID: &assignee,
	})
	require.NoError(t, err)
	require.False(t, alreadyApplied)
	require.Equal(t, "task-1", svc.lastID)
	require.NotNil(t, svc.exactRequest.Labels)
	require.Equal(t, labels, *svc.exactRequest.Labels)
	require.Equal(t, &assignee, svc.exactRequest.AssigneeUserID)
	require.NotNil(t, updated)
}

func TestPluginsTaskWriter_ExactCreateSettlesNormalizedSourceIdentity(t *testing.T) {
	svc := &fakePluginTaskWriteService{settleOK: true}
	task, alreadyApplied, err := (pluginsTaskWriterAdapter{svc: svc}).CreateTaskExact(context.Background(), plugins.ExactTaskCreateInput{
		ExternalID: "  coordinator:proposal-17  ",
		Task: plugins.TaskCreateInput{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Add task source identity", Source: "plugin:coordinator",
		},
	})
	require.NoError(t, err)
	require.False(t, alreadyApplied)
	require.Equal(t, "task-1", task.ID)
	require.Equal(t, "coordinator:proposal-17", svc.lastCreate.ExternalID)
	require.Equal(t, "task-1", svc.settledTaskID)
	require.Equal(t, "coordinator:proposal-17", svc.settledExternalID)
}

func TestPluginsTaskWriter_ExactCreateRejectsLostSourceIdentity(t *testing.T) {
	svc := &fakePluginTaskWriteService{}
	task, alreadyApplied, err := (pluginsTaskWriterAdapter{svc: svc}).CreateTaskExact(context.Background(), plugins.ExactTaskCreateInput{
		ExternalID: "coordinator:proposal-18",
		Task: plugins.TaskCreateInput{
			WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Do not acknowledge unsettled source identity", Source: "plugin:coordinator",
		},
	})
	require.ErrorContains(t, err, "task source identity was lost before settlement")
	require.Nil(t, task)
	require.False(t, alreadyApplied)
}
