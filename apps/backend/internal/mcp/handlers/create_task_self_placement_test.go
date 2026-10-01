package handlers

import (
	"context"
	"encoding/json"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type selfPlacementFixture struct {
	ctx       context.Context
	svc       *service.Service
	repo      *sqliterepo.Repository
	workspace *models.Workspace
	workflow  *models.Workflow
	parent    *models.Task
	child     *models.Task
	h         *Handlers
}

func newSelfPlacementFixture(t *testing.T) selfPlacementFixture {
	t.Helper()
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	require.NotEmpty(t, workflows)
	parentResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflows[0].ID,
		Title:       "Parent task",
	})
	require.NoError(t, err)
	childResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflows[0].ID,
		ParentID:    parentResult.Task.ID,
		Title:       "Child task",
	})
	require.NoError(t, err)
	const sessionID = "self-placement-session"
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        sessionID,
		TaskID:    childResult.Task.ID,
		State:     models.TaskSessionStateWaitingForInput,
		IsPrimary: true,
	}))

	return selfPlacementFixture{
		ctx:       ctx,
		svc:       svc,
		repo:      repo,
		workspace: workspaces[0],
		workflow:  workflows[0],
		parent:    parentResult.Task,
		child:     childResult.Task,
		h:         NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t)),
	}
}

func (f selfPlacementFixture) caller() context.Context {
	return mcpTestKanbanContext(f.ctx, f.workspace.ID, f.child.ID, "self-placement-session")
}

func selfPlacementCreatePayload(f selfPlacementFixture, parentID string) map[string]interface{} {
	return map[string]interface{}{
		"parent_id":        parentID,
		"parent_is_self":   parentID == f.child.ID,
		"workspace_id":     f.workspace.ID,
		"workflow_id":      f.workflow.ID,
		"title":            "Created sibling",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
	}
}

func (f selfPlacementFixture) create(t *testing.T, ctx context.Context, payload map[string]interface{}) *ws.Message {
	t.Helper()
	resp, err := f.h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, payload))
	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp
}

func selfPlacementResult(t *testing.T, resp *ws.Message) mcpCreateTaskResult {
	t.Helper()
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
	var result mcpCreateTaskResult
	require.NoError(t, json.Unmarshal(resp.Payload, &result))
	return result
}

func (f selfPlacementFixture) taskCount(t *testing.T) int {
	t.Helper()
	var count int
	require.NoError(t, f.repo.DB().QueryRowContext(f.ctx, "SELECT COUNT(*) FROM tasks").Scan(&count))
	return count
}

type selfPlacementPolicyAttacher func(context.Context, string, string, service.WorkspacePolicy) error

func (a selfPlacementPolicyAttacher) AttachWorkspacePolicy(ctx context.Context, taskID, parentID string, policy service.WorkspacePolicy) error {
	return a(ctx, taskID, parentID, policy)
}

// @covers AC-TASKS-SELF-SIBLING-001.1 AC-TASKS-SELF-SIBLING-001.4
func TestMCPCreateTaskSelfPlacement(t *testing.T) {
	f := newSelfPlacementFixture(t)

	resp, err := f.h.handleCreateTask(f.caller(), makeWSMessage(t, ws.ActionMCPCreateTask, selfPlacementCreatePayload(f, f.child.ID)))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))

	var result mcpCreateTaskResult
	require.NoError(t, json.Unmarshal(resp.Payload, &result))
	assert.Equal(t, f.parent.ID, result.ParentID)
	require.NotNil(t, result.ParentResolution)
	assert.Equal(t, f.child.ID, result.ParentResolution.RequestedParentID)
	assert.Equal(t, f.parent.ID, result.ParentResolution.ResolvedParentID)
	assert.Equal(t, "kanban_depth_limit", result.ParentResolution.Reason)
	assert.Contains(t, result.ParentResolution.Message, "sibling")
	assert.Contains(t, result.ParentResolution.Message, f.parent.ID)
	assert.Contains(t, result.ParentResolution.Message, "coordination")
}

func TestMCPCreateTaskSelfPlacementExplicitParentKeepsDepthGuard(t *testing.T) {
	f := newSelfPlacementFixture(t)
	payload := selfPlacementCreatePayload(f, f.child.ID)
	delete(payload, "parent_is_self")

	resp, err := f.h.handleCreateTask(f.caller(), makeWSMessage(t, ws.ActionMCPCreateTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assert.Contains(t, string(resp.Payload), "subtask")
}

// @covers AC-TASKS-SELF-SIBLING-001.2
func TestMCPCreateTaskSelfPlacementOmittedParent(t *testing.T) {
	f := newSelfPlacementFixture(t)
	payload := selfPlacementCreatePayload(f, "")
	delete(payload, "parent_id")
	delete(payload, "parent_is_self")
	result := selfPlacementResult(t, f.create(t, f.caller(), payload))
	require.Empty(t, result.ParentID)
	require.Nil(t, result.ParentResolution)
	task, err := f.svc.GetTask(f.ctx, result.ID)
	require.NoError(t, err)
	require.Empty(t, task.ParentID)
}

func TestMCPCreateTaskSelfPlacementRejectsForgedMarker(t *testing.T) {
	f := newSelfPlacementFixture(t)
	foreign, err := f.svc.CreateTask(f.ctx, &service.CreateTaskRequest{
		WorkspaceID: f.workspace.ID,
		WorkflowID:  f.workflow.ID,
		Title:       "Foreign parent",
	})
	require.NoError(t, err)
	payload := selfPlacementCreatePayload(f, foreign.Task.ID)
	payload["parent_is_self"] = true

	resp, err := f.h.handleCreateTask(f.caller(), makeWSMessage(t, ws.ActionMCPCreateTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
}

func TestMCPCreateTaskSelfPlacementRootAndBoundaryCallers(t *testing.T) {
	f := newSelfPlacementFixture(t)
	require.NoError(t, f.repo.CreateTaskSession(f.ctx, &models.TaskSession{
		ID:        "root-placement-session",
		TaskID:    f.parent.ID,
		State:     models.TaskSessionStateWaitingForInput,
		IsPrimary: true,
	}))

	rootCaller := mcpTestKanbanContext(f.ctx, f.workspace.ID, f.parent.ID, "root-placement-session")
	rootPayload := selfPlacementCreatePayload(f, f.parent.ID)
	rootPayload["parent_is_self"] = true
	resp, err := f.h.handleCreateTask(rootCaller, makeWSMessage(t, ws.ActionMCPCreateTask, rootPayload))
	require.NoError(t, err)
	var rootResult mcpCreateTaskResult
	require.NoError(t, json.Unmarshal(resp.Payload, &rootResult))
	assert.Equal(t, f.parent.ID, rootResult.ParentID)
	assert.Nil(t, rootResult.ParentResolution)

	resp, err = f.h.handleCreateTask(mcpTestExternalContext(f.ctx), makeWSMessage(t, ws.ActionMCPCreateTask, selfPlacementCreatePayload(f, f.child.ID)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)

	automation := mcpscope.WithPrincipal(f.ctx, mcpscope.Principal{
		AutomationID:    "automation-1",
		WorkspaceID:     f.workspace.ID,
		CallerTaskID:    f.child.ID,
		CallerSessionID: "self-placement-session",
		Surface:         mcpprofile.SurfaceAutomation,
	})
	resp, err = f.h.handleCreateTask(automation, makeWSMessage(t, ws.ActionMCPCreateTask, selfPlacementCreatePayload(f, f.child.ID)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
}

func TestMCPCreateTaskSelfPlacementDeduplicatedResultExplainsActualParent(t *testing.T) {
	f := newSelfPlacementFixture(t)
	payload := selfPlacementCreatePayload(f, f.child.ID)
	payload["external_id"] = "self-placement-dedup"

	first, err := f.h.handleCreateTask(f.caller(), makeWSMessage(t, ws.ActionMCPCreateTask, payload))
	require.NoError(t, err)
	var created mcpCreateTaskResult
	require.NoError(t, json.Unmarshal(first.Payload, &created))
	require.NotNil(t, created.ParentResolution)
	require.False(t, created.Deduplicated)

	second, err := f.h.handleCreateTask(f.caller(), makeWSMessage(t, ws.ActionMCPCreateTask, payload))
	require.NoError(t, err)
	var found mcpCreateTaskResult
	require.NoError(t, json.Unmarshal(second.Payload, &found))
	require.True(t, found.Deduplicated)
	require.NotNil(t, found.ParentResolution)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, f.parent.ID, found.ParentID)
	assert.Contains(t, found.ParentResolution.Message, "without creation or reparenting")
	assert.Contains(t, found.ParentResolution.Message, "actual parent")
	assert.Contains(t, found.ParentResolution.Message, "Kanban subtask depth limit")
}
