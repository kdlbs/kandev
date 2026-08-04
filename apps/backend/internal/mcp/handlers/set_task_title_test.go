package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSetTaskTitle_UpdatesPendingTitleOnce(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-title", Name: "Titles", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          "task-title",
		WorkspaceID: "ws-title",
		Title:       "Build the useful feature now",
		Description: "Build the useful feature now",
		State:       v1.TaskStateInProgress,
		Metadata:    map[string]interface{}{models.MetaKeyAgentTitlePending: true, models.MetaKeyAgentTitleOwnerSessionID: "session-title"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}

	resp, err := h.handleSetTaskTitle(ctx, makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id":    "task-title",
		"session_id": "session-title",
		"title":      "Useful Feature",
	}))
	require.NoError(t, err)
	assertTaskTitleResponse(t, resp, map[string]interface{}{
		"accepted": true,
		"task_id":  "task-title",
		"title":    "Useful Feature",
	})

	updated, err := svc.GetTask(ctx, "task-title")
	require.NoError(t, err)
	assert.Equal(t, "Useful Feature", updated.Title)
	assert.False(t, models.IsAgentTitlePending(updated.Metadata))

	resp, err = h.handleSetTaskTitle(ctx, makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id":    "task-title",
		"session_id": "session-title",
		"title":      "Late Agent Title",
	}))
	require.NoError(t, err)
	assertTaskTitleResponse(t, resp, map[string]interface{}{
		"accepted": false,
		"task_id":  "task-title",
		"title":    "Useful Feature",
		"reason":   "title_not_pending",
	})
}

func assertTaskTitleResponse(t *testing.T, resp *ws.Message, want map[string]interface{}) {
	t.Helper()
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &got))
	for key, value := range want {
		assert.Equal(t, value, got[key], "response field %s", key)
	}
}

func TestHandleSetTaskTitle_ValidatesInput(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}
	resp, err := h.handleSetTaskTitle(context.Background(), makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id": "task-title",
		"title":   "  ",
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

type titleBranchRenamerStub struct {
	calls int
	err   error
}

func (s *titleBranchRenamerStub) RenameGeneratedBranchesForTaskTitle(_ context.Context, _, _, title string) (orchestrator.TitleBranchRenameResult, error) {
	s.calls++
	if s.err != nil {
		return orchestrator.TitleBranchRenameResult{}, s.err
	}
	return orchestrator.TitleBranchRenameResult{
		Status:  orchestrator.TitleBranchStatusRenamed,
		Renamed: []orchestrator.TitleBranchRename{{To: "feature/" + strings.ToLower(strings.ReplaceAll(title, " ", "-"))}},
	}, nil
}

func TestHandleSetTaskTitleKeepsAcceptedTitleWhenBranchRenameFails(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-title-branch-failure", Name: "Titles", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-title-branch-failure", WorkspaceID: "ws-title-branch-failure", Title: "Temporary title", Description: "Prompt",
		State:     v1.TaskStateInProgress,
		Metadata:  map[string]interface{}{models.MetaKeyAgentTitlePending: true, models.MetaKeyAgentTitleOwnerSessionID: "session-title-branch-failure"},
		CreatedAt: now, UpdatedAt: now,
	}))
	renamer := &titleBranchRenamerStub{err: errors.New("git rename failed")}
	h := &Handlers{taskSvc: svc, titleBranchRenamer: renamer, logger: testLogger(t).WithFields()}
	resp, err := h.handleSetTaskTitle(ctx, makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id": "task-title-branch-failure", "session_id": "session-title-branch-failure", "title": "Final Title",
	}))
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Equal(t, true, payload["accepted"])
	require.Equal(t, "failed", payload["branch_rename"].(map[string]interface{})["status"])
	updated, err := svc.GetTask(ctx, "task-title-branch-failure")
	require.NoError(t, err)
	require.Equal(t, "Final Title", updated.Title)
	require.False(t, models.IsAgentTitlePending(updated.Metadata))
}

func TestHandleSetTaskTitleIncludesBranchRenameOutcomeOnlyAfterAcceptedTitle(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-title-branch", Name: "Titles", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-title-branch", WorkspaceID: "ws-title-branch", Title: "Temporary title", Description: "Prompt",
		State:     v1.TaskStateInProgress,
		Metadata:  map[string]interface{}{models.MetaKeyAgentTitlePending: true, models.MetaKeyAgentTitleOwnerSessionID: "session-title-branch"},
		CreatedAt: now, UpdatedAt: now,
	}))
	rename := &titleBranchRenamerStub{}
	h := &Handlers{taskSvc: svc, titleBranchRenamer: rename, logger: testLogger(t).WithFields()}

	resp, err := h.handleSetTaskTitle(ctx, makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id": "task-title-branch", "session_id": "session-title-branch", "title": "Final Title",
	}))
	require.NoError(t, err)
	assertTaskTitleResponse(t, resp, map[string]interface{}{"accepted": true})
	var accepted map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &accepted))
	require.Equal(t, "renamed", accepted["branch_rename"].(map[string]interface{})["status"])
	require.Equal(t, 1, rename.calls)

	resp, err = h.handleSetTaskTitle(ctx, makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id": "task-title-branch", "session_id": "session-title-branch", "title": "Late Title",
	}))
	require.NoError(t, err)
	var rejected map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &rejected))
	_, hasBranchRename := rejected["branch_rename"]
	require.False(t, hasBranchRename, "rejected title must not invoke branch rename")
	require.Equal(t, 1, rename.calls)
}

func TestHandleSetTaskTitleRejectsNonOwner(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-title-owner", Name: "Titles", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          "task-title-owner",
		WorkspaceID: "ws-title-owner",
		Title:       "Temporary title",
		Description: "Prompt",
		State:       v1.TaskStateInProgress,
		Metadata:    map[string]interface{}{models.MetaKeyAgentTitlePending: true, models.MetaKeyAgentTitleOwnerSessionID: "session-owner"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}

	resp, err := h.handleSetTaskTitle(ctx, makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id":    "task-title-owner",
		"session_id": "session-other",
		"title":      "Other title",
	}))
	require.NoError(t, err)
	assertTaskTitleResponse(t, resp, map[string]interface{}{
		"accepted": false,
		"task_id":  "task-title-owner",
		"title":    "Temporary title",
		"reason":   "title_not_owner",
	})
}

func TestHandleSetTaskTitle_RejectsOverlongTitle(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-title-long", Name: "Titles", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          "task-title-long",
		WorkspaceID: "ws-title-long",
		Title:       "Temporary title",
		Description: "Prompt",
		State:       v1.TaskStateInProgress,
		Metadata:    map[string]interface{}{models.MetaKeyAgentTitlePending: true, models.MetaKeyAgentTitleOwnerSessionID: "session-title-long"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}

	resp, err := h.handleSetTaskTitle(ctx, makeWSMessage(t, ws.ActionMCPSetTaskTitle, map[string]interface{}{
		"task_id":    "task-title-long",
		"session_id": "session-title-long",
		"title":      strings.Repeat("x", 501),
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}
