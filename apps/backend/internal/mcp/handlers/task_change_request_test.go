package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskChangeRequestRecorder struct {
	operation string
	request   TaskChangeLinkRequest
	err       error
}

type richTaskChangeRequestRecorder struct {
	*taskChangeRequestRecorder
	result TaskChangeLinkMutationResult
}

func (r *richTaskChangeRequestRecorder) ManageTaskChangeRequest(
	_ context.Context, req TaskChangeLinkRequest,
) (TaskChangeLinkMutationResult, error) {
	r.operation, r.request = req.Operation, req
	if r.err != nil {
		return r.result, r.err
	}
	return r.result, nil
}

func (r *taskChangeRequestRecorder) LinkTaskChange(_ context.Context, req TaskChangeLinkRequest) ([]TaskChangeLink, error) {
	r.operation, r.request = "link", req
	if r.err != nil {
		return nil, r.err
	}
	return []TaskChangeLink{req.Link}, nil
}

func (r *taskChangeRequestRecorder) UnlinkTaskChange(_ context.Context, req TaskChangeLinkRequest) ([]TaskChangeLink, error) {
	r.operation, r.request = "unlink", req
	if r.err != nil {
		return nil, r.err
	}
	return nil, nil
}

func (r *taskChangeRequestRecorder) ReplaceTaskChange(_ context.Context, req TaskChangeLinkRequest) ([]TaskChangeLink, error) {
	r.operation, r.request = "replace", req
	if r.err != nil {
		return nil, r.err
	}
	return []TaskChangeLink{req.Link}, nil
}

func TestManageTaskChangeRequestDispatchesWithTrustedCaller(t *testing.T) {
	svc, repo := newTestTaskService(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "workspace-1", Name: "Workspace", CreatedAt: now, UpdatedAt: now,
	}))
	for _, task := range []*models.Task{
		{ID: "task-target", WorkspaceID: "workspace-1", Title: "Target", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now},
		{ID: "task-caller", WorkspaceID: "workspace-1", Title: "Caller", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateTask(context.Background(), task))
	}
	recorder := &taskChangeRequestRecorder{}
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetTaskChangeLinkService(recorder)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "workspace-1", CallerTaskID: "task-caller", CallerSessionID: "session-1",
		Surface: mcpprofile.SurfaceKanbanTask,
	})
	msg := makeWSMessage(t, ws.ActionMCPManageTaskChangeRequest, map[string]interface{}{
		"operation": "link", "task_id": "task-target", "caller_task_id": "task-caller",
		"provider": "gitlab", "repository_id": "repo-1", "number": 42,
	})

	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	response, err := dispatcher.Dispatch(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, ws.MessageTypeResponse, response.Type)
	assert.Equal(t, "link", recorder.operation)
	assert.Equal(t, "task-target", recorder.request.TaskID)
	assert.Equal(t, "gitlab", recorder.request.Link.Provider)
	assert.Equal(t, "repo-1", recorder.request.Link.RepositoryID)
	assert.Equal(t, 42, recorder.request.Link.Number)
}

func TestManageTaskChangeRequestCarriesFailureState(t *testing.T) {
	svc, repo := newTestTaskService(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "workspace-1", Name: "Workspace", CreatedAt: now, UpdatedAt: now,
	}))
	for _, task := range []*models.Task{
		{ID: "task-target", WorkspaceID: "workspace-1", Title: "Target", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now},
		{ID: "task-caller", WorkspaceID: "workspace-1", Title: "Caller", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateTask(context.Background(), task))
	}
	recorder := &taskChangeRequestRecorder{err: errors.New("old unlink failed; rollback failed")}
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetTaskChangeLinkService(recorder)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "workspace-1", CallerTaskID: "task-caller", CallerSessionID: "session-1",
		Surface: mcpprofile.SurfaceKanbanTask,
	})
	msg := makeWSMessage(t, ws.ActionMCPManageTaskChangeRequest, map[string]interface{}{
		"operation": "replace", "task_id": "task-target", "caller_task_id": "task-caller",
		"provider": "gitlab", "repository_id": "repo-new", "number": 42,
		"old_provider": "gitlab", "old_repository_id": "repo-old", "old_number": 7,
	})

	response, err := h.handleManageTaskChangeRequest(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)
	var errorPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &errorPayload))
	assert.Equal(t, ws.ErrorCodeValidation, errorPayload.Code)
	assert.Equal(t, "old unlink failed; rollback failed", errorPayload.Details["operation_error"])
}

func TestManageTaskChangeRequestRichServiceReceivesOldIdentity(t *testing.T) {
	svc, repo := newTestTaskService(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "workspace-1", Name: "Workspace", CreatedAt: now, UpdatedAt: now,
	}))
	for _, task := range []*models.Task{
		{ID: "task-target", WorkspaceID: "workspace-1", Title: "Target", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now},
		{ID: "task-caller", WorkspaceID: "workspace-1", Title: "Caller", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateTask(context.Background(), task))
	}
	recorder := &richTaskChangeRequestRecorder{
		taskChangeRequestRecorder: &taskChangeRequestRecorder{},
		result: TaskChangeLinkMutationResult{
			TaskID: "task-target", Links: []TaskChangeLink{{Provider: "gitlab", RepositoryID: "repo-new", Number: 42}}, StateKnown: true,
		},
	}
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetTaskChangeLinkService(recorder)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "workspace-1", CallerTaskID: "task-caller", CallerSessionID: "session-1",
		Surface: mcpprofile.SurfaceKanbanTask,
	})
	msg := makeWSMessage(t, ws.ActionMCPManageTaskChangeRequest, map[string]interface{}{
		"operation": "replace", "task_id": "task-target", "caller_task_id": "task-caller",
		"provider": "gitlab", "repository_id": "repo-new", "number": 42,
		"old_provider": "gitlab", "old_repository_id": "repo-old", "old_number": 7,
	})

	response, err := h.handleManageTaskChangeRequest(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, ws.MessageTypeResponse, response.Type)
	require.NotNil(t, recorder.request.Old)
	assert.Equal(t, TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-old", Number: 7}, *recorder.request.Old)
	assert.Equal(t, taskChangeOperationReplace, recorder.operation)
}

func TestManageTaskChangeRequestRichServiceReturnsCompensationState(t *testing.T) {
	svc, repo := newTestTaskService(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "workspace-1", Name: "Workspace", CreatedAt: now, UpdatedAt: now,
	}))
	for _, task := range []*models.Task{
		{ID: "task-target", WorkspaceID: "workspace-1", Title: "Target", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now},
		{ID: "task-caller", WorkspaceID: "workspace-1", Title: "Caller", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateTask(context.Background(), task))
	}
	recorder := &richTaskChangeRequestRecorder{
		taskChangeRequestRecorder: &taskChangeRequestRecorder{err: errors.New("replacement failed")},
		result: TaskChangeLinkMutationResult{
			TaskID: "task-target",
			Links: []TaskChangeLink{
				{Provider: "gitlab", RepositoryID: "repo-old", Number: 7},
				{Provider: "gitlab", RepositoryID: "repo-new", Number: 42},
			},
			OperationError: "old unlink failed", RollbackError: "rollback failed", StateKnown: true,
		},
	}
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetTaskChangeLinkService(recorder)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "workspace-1", CallerTaskID: "task-caller", CallerSessionID: "session-1",
		Surface: mcpprofile.SurfaceKanbanTask,
	})
	msg := makeWSMessage(t, ws.ActionMCPManageTaskChangeRequest, map[string]interface{}{
		"operation": "replace", "task_id": "task-target", "caller_task_id": "task-caller",
		"provider": "gitlab", "repository_id": "repo-new", "number": 42,
		"old_provider": "gitlab", "old_repository_id": "repo-old", "old_number": 7,
	})

	response, err := h.handleManageTaskChangeRequest(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)
	var errorPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &errorPayload))
	assert.Equal(t, "old unlink failed", errorPayload.Details["operation_error"])
	assert.Equal(t, "rollback failed", errorPayload.Details["rollback_error"])
	assert.Equal(t, true, errorPayload.Details["state_known"])
	assert.Len(t, errorPayload.Details["links"], 2)
}
