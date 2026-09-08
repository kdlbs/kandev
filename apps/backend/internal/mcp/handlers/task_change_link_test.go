package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTaskChangeLinkRequiresProviderRepositoryAndNumber(t *testing.T) {
	valid, err := validateTaskChangeLink("GitLab", "repo-1", 42)
	if err != nil || valid.Provider != "gitlab" {
		t.Fatalf("validateTaskChangeLink() = %#v, %v", valid, err)
	}
	for _, input := range []TaskChangeLink{
		{Provider: "", RepositoryID: "repo-1", Number: 1},
		{Provider: "github", RepositoryID: "", Number: 1},
		{Provider: "github", RepositoryID: "repo-1", Number: 0},
		{Provider: "other", RepositoryID: "repo-1", Number: 1},
	} {
		if _, err := validateTaskChangeLink(input.Provider, input.RepositoryID, input.Number); err == nil {
			t.Fatalf("validateTaskChangeLink(%#v) succeeded", input)
		}
	}
}

func TestTaskChangeLinkRequestRejectsCrossWorkspaceCaller(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-target", Name: "Target", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-caller", Name: "Caller", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-target", WorkspaceID: "ws-target", Title: "Target", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-caller", WorkspaceID: "ws-caller", Title: "Caller", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now}))

	h := &Handlers{taskSvc: svc}
	msg := makeWSMessage(t, ws.ActionMCPLinkTaskPR, map[string]any{
		"task_id": "task-target", "caller_task_id": "task-caller",
		"provider": "github", "repository_id": "repo-1", "number": 1,
	})

	_, resp, err := h.taskChangeLinkRequest(ctx, msg, false)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
}

func TestTaskChangeLinkRequestRejectsMissingCallerTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-target", Name: "Target", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-target", WorkspaceID: "ws-target", Title: "Target", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now}))

	h := &Handlers{taskSvc: svc}
	msg := makeWSMessage(t, ws.ActionMCPLinkTaskPR, map[string]any{
		"task_id":  "task-target",
		"provider": "github", "repository_id": "repo-1", "number": 1,
	})

	_, resp, err := h.taskChangeLinkRequest(ctx, msg, false)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestTaskChangeLinkRequestParsesReplacementIdentity(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-target", WorkspaceID: "ws-1", Title: "Target", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-caller", WorkspaceID: "ws-1", Title: "Caller", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now}))

	h := &Handlers{taskSvc: svc}
	msg := makeWSMessage(t, ws.ActionMCPReplaceTaskPR, map[string]any{
		"task_id": "task-target", "caller_task_id": "task-caller",
		"provider": "gitlab", "repository_id": "repo-new", "number": 42,
		"old_provider": "github", "old_repository_id": "repo-old", "old_number": 7,
	})

	req, resp, err := h.taskChangeLinkRequest(ctx, msg, true)
	require.NoError(t, err)
	require.Nil(t, resp)
	assert.Equal(t, TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-new", Number: 42}, req.Link)
	require.NotNil(t, req.Old)
	assert.Equal(t, TaskChangeLink{Provider: "github", RepositoryID: "repo-old", Number: 7}, *req.Old)
}

func TestTaskChangeLinkRequestRejectsInvalidReplacementIdentity(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-target", WorkspaceID: "ws-1", Title: "Target", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-caller", WorkspaceID: "ws-1", Title: "Caller", State: v1.TaskStateTODO, CreatedAt: now, UpdatedAt: now}))

	h := &Handlers{taskSvc: svc}
	msg := makeWSMessage(t, ws.ActionMCPReplaceTaskPR, map[string]any{
		"task_id": "task-target", "caller_task_id": "task-caller",
		"provider": "gitlab", "repository_id": "repo-new", "number": 42,
	})

	_, resp, err := h.taskChangeLinkRequest(ctx, msg, true)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}
