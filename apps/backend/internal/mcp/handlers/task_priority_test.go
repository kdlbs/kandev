package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-MCP-PRIORITY-001.1 AC-TASKS-MCP-PRIORITY-001.2 AC-TASKS-MCP-PRIORITY-001.6 AC-TASKS-MCP-PRIORITY-001.7
func TestHandleCreateTaskPriority(t *testing.T) {
	ctx := context.Background()
	svc, repo, eventBus := newTestTaskServiceWithEventBus(t)
	workspace, workflow := mcpTaskPriorityDestination(t, svc)
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	eventsSeen := subscribeMCPTaskPriorityEvents(t, eventBus)

	for _, priority := range []string{
		models.TaskPriorityCritical,
		models.TaskPriorityHigh,
		models.TaskPriorityMedium,
		models.TaskPriorityLow,
	} {
		created := createMCPTaskWithPriority(t, h, ctx, workspace.ID, workflow.ID, map[string]interface{}{
			"title":       "Priority " + priority,
			"priority":    priority,
			"external_id": "priority-" + priority,
		})
		task, err := svc.GetTask(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, priority, task.Priority)
		assert.Equal(t, priority, created.Priority)
		assertMCPTaskPriorityEvent(t, eventsSeen, events.TaskCreated, created.ID, priority)
	}

	defaulted := createMCPTaskWithPriority(t, h, ctx, workspace.ID, workflow.ID, map[string]interface{}{
		"title": "Default priority",
	})
	assert.Equal(t, models.TaskPriorityMedium, defaulted.Priority)

	parent, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspace.ID,
		WorkflowID:  workflow.ID,
		Title:       "High priority parent",
		Priority:    models.TaskPriorityHigh,
	})
	require.NoError(t, err)
	child := createMCPTaskWithPriority(t, h, ctx, workspace.ID, workflow.ID, map[string]interface{}{
		"title":     "Default priority child",
		"parent_id": parent.Task.ID,
	})
	assert.Equal(t, models.TaskPriorityMedium, child.Priority, "priority is not inherited from the parent")

	first := createMCPTaskWithPriority(t, h, ctx, workspace.ID, workflow.ID, map[string]interface{}{
		"title":       "Retry priority",
		"priority":    models.TaskPriorityHigh,
		"external_id": "priority-retry",
	})
	changedRetry := createMCPTaskWithPriority(t, h, ctx, workspace.ID, workflow.ID, map[string]interface{}{
		"title":       "Retry priority changed",
		"priority":    models.TaskPriorityLow,
		"external_id": "priority-retry",
	})
	omittedRetry := createMCPTaskWithPriority(t, h, ctx, workspace.ID, workflow.ID, map[string]interface{}{
		"title":       "Retry priority omitted",
		"external_id": "priority-retry",
	})
	assert.Equal(t, first.ID, changedRetry.ID)
	assert.Equal(t, first.ID, omittedRetry.ID)
	assert.Equal(t, models.TaskPriorityHigh, changedRetry.Priority)
	assert.Equal(t, models.TaskPriorityHigh, omittedRetry.Priority)
}

// @covers AC-TASKS-MCP-PRIORITY-001.4 AC-TASKS-MCP-PRIORITY-001.5 AC-TASKS-MCP-PRIORITY-001.7
func TestHandleUpdateTaskPriority(t *testing.T) {
	ctx := context.Background()
	svc, repo, eventBus := newTestTaskServiceWithEventBus(t)
	_, workflow := mcpTaskPriorityDestination(t, svc)
	const taskID = "priority-update-task"
	const sessionID = "priority-update-session"
	initial := &models.Task{
		ID: taskID, WorkspaceID: workflow.WorkspaceID, WorkflowID: workflow.ID,
		WorkflowStepID: "step-original", Title: "Original title", Description: "Original description",
		State: v1.TaskStateTODO, Priority: models.TaskPriorityHigh, Position: 7,
		Metadata: map[string]interface{}{"keep": "sentinel"},
	}
	require.NoError(t, repo.CreateTask(ctx, initial))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, AgentProfileID: "profile-1",
		State: models.TaskSessionStateWaitingForInput, IsPrimary: true,
	}))
	baseline, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	eventsSeen := subscribeMCPTaskPriorityEvents(t, eventBus)
	h := updateTaskHandler(t, svc)

	resp, err := h.handleUpdateTask(ctx, makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
		"task_id":  taskID,
		"priority": models.TaskPriorityCritical,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
	var response struct {
		Priority string `json:"priority"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &response))
	assert.Equal(t, models.TaskPriorityCritical, response.Priority)

	updated, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskPriorityCritical, updated.Priority)
	assert.Equal(t, baseline.Title, updated.Title)
	assert.Equal(t, baseline.Description, updated.Description)
	assert.Equal(t, baseline.State, updated.State)
	assert.Equal(t, baseline.Position, updated.Position)
	assert.Equal(t, baseline.WorkflowID, updated.WorkflowID)
	assert.Equal(t, baseline.WorkflowStepID, updated.WorkflowStepID)
	assert.Equal(t, baseline.Metadata, updated.Metadata)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	assertMCPTaskPriorityEvent(t, eventsSeen, events.TaskUpdated, taskID, models.TaskPriorityCritical)

	// Repeating a priority succeeds, while omitting it preserves the stored value.
	for _, args := range []map[string]interface{}{
		{"task_id": taskID, "priority": models.TaskPriorityCritical},
		{"task_id": taskID},
	} {
		response, err := h.handleUpdateTask(ctx, makeWSMessage(t, ws.ActionMCPUpdateTask, args))
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
		persisted, err := repo.GetTask(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, models.TaskPriorityCritical, persisted.Priority)
	}

	combined, err := h.handleUpdateTask(ctx, makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
		"task_id":  taskID,
		"title":    "Updated title",
		"priority": models.TaskPriorityLow,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, combined.Type, string(combined.Payload))
	updated, err = repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, "Updated title", updated.Title)
	assert.Equal(t, models.TaskPriorityLow, updated.Priority)
}

// @covers AC-TASKS-MCP-PRIORITY-001.5
func TestMCPTaskPriorityValidationPrecedesMutation(t *testing.T) {
	t.Run("creation", func(t *testing.T) {
		ctx := context.Background()
		svc, repo := newTestTaskService(t)
		workspace, workflow := mcpTaskPriorityDestination(t, svc)
		h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
		before, err := svc.ListTasks(ctx, workflow.ID)
		require.NoError(t, err)

		resp, err := h.handleCreateTask(mcpTestExternalContext(ctx), makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
			"workspace_id":     workspace.ID,
			"workflow_id":      workflow.ID,
			"title":            "Invalid priority must not be created",
			"agent_profile_id": "profile-1",
			"start_agent":      false,
			"priority":         "urgent",
		}))
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeValidation)
		after, err := svc.ListTasks(ctx, workflow.ID)
		require.NoError(t, err)
		assert.Len(t, after, len(before))
	})

	t.Run("deferred prompt replacement", func(t *testing.T) {
		svc, repo := newTestTaskService(t)
		seedPendingLaunchTask(t, repo)
		h := updateTaskHandler(t, svc)

		resp, err := h.handleUpdateTask(context.Background(), makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
			"task_id":                pendingLaunchTaskID,
			"priority":               "urgent",
			"deferred_launch_prompt": "must not replace the original brief",
		}))
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeValidation)
		assert.Equal(t, "stale brief", launchPrompt(t, repo))
	})
}

// @covers AC-TASKS-MCP-PRIORITY-001.3 AC-TASKS-MCP-PRIORITY-001.8
func TestMCPTaskPriorityAutomationScope(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspace, workflow := mcpTaskPriorityDestination(t, svc)

	createTask := func(title, workspaceID, workflowID string) *models.Task {
		t.Helper()
		created, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
			WorkspaceID: workspaceID,
			WorkflowID:  workflowID,
			Title:       title,
			Priority:    models.TaskPriorityMedium,
		})
		require.NoError(t, err)
		return created.Task
	}

	self := createTask("Automation self", workspace.ID, workflow.ID)
	target := createTask("Automation target", workspace.ID, workflow.ID)
	foreignWorkspace := &models.Workspace{ID: "mcp-priority-foreign-workspace", Name: "Foreign"}
	require.NoError(t, repo.CreateWorkspace(ctx, foreignWorkspace))
	foreignWorkflow := &models.Workflow{ID: "mcp-priority-foreign-workflow", WorkspaceID: foreignWorkspace.ID, Name: "Foreign"}
	require.NoError(t, repo.CreateWorkflow(ctx, foreignWorkflow))
	foreign := createTask("Foreign target", foreignWorkspace.ID, foreignWorkflow.ID)

	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	principal := mcpscope.Principal{
		AutomationID:    "priority-automation",
		WorkspaceID:     workspace.ID,
		CallerTaskID:    self.ID,
		CallerSessionID: "priority-automation-session",
		Surface:         mcpprofile.SurfaceAutomation,
	}
	automationCtx := mcpscope.WithPrincipal(ctx, principal)

	update := func(taskID, priority string) *ws.Message {
		t.Helper()
		response, err := dispatcher.Dispatch(automationCtx, makeWSMessage(t, ws.ActionMCPUpdateTask, map[string]interface{}{
			"task_id":  taskID,
			"priority": priority,
		}))
		require.NoError(t, err)
		return response
	}

	allowed := update(target.ID, models.TaskPriorityHigh)
	require.Equal(t, ws.MessageTypeResponse, allowed.Type, string(allowed.Payload))
	updatedTarget, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskPriorityHigh, updatedTarget.Priority)

	deniedForeign := update(foreign.ID, models.TaskPriorityCritical)
	assertWSError(t, deniedForeign, ws.ErrorCodeNotFound)
	unchangedForeign, err := svc.GetTask(ctx, foreign.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskPriorityMedium, unchangedForeign.Priority)

	deniedSelf := update(self.ID, models.TaskPriorityLow)
	assertWSError(t, deniedSelf, ws.ErrorCodeNotFound)
	unchangedSelf, err := svc.GetTask(ctx, self.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskPriorityMedium, unchangedSelf.Priority)
}

type mcpTaskPriorityEvent struct {
	Type string
	Data interface{}
}

func mcpTaskPriorityDestination(t *testing.T, svc *service.Service) (*models.Workspace, *models.Workflow) {
	t.Helper()
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, workspaces)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	require.NotEmpty(t, workflows)
	return workspaces[0], workflows[0]
}

type mcpPriorityCreateResult struct {
	ID       string `json:"id"`
	Priority string `json:"priority"`
	Deduped  bool   `json:"deduplicated"`
}

func createMCPTaskWithPriority(
	t *testing.T,
	h *Handlers,
	ctx context.Context,
	workspaceID, workflowID string,
	fields map[string]interface{},
) mcpPriorityCreateResult {
	t.Helper()
	payload := map[string]interface{}{
		"workspace_id":     workspaceID,
		"workflow_id":      workflowID,
		"agent_profile_id": "profile-1",
		"start_agent":      false,
	}
	for key, value := range fields {
		payload[key] = value
	}
	resp, err := h.handleCreateTask(mcpTestExternalContext(ctx), makeWSMessage(t, ws.ActionMCPCreateTask, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
	var created mcpPriorityCreateResult
	require.NoError(t, json.Unmarshal(resp.Payload, &created))
	return created
}

type mcpTaskPriorityEvents struct {
	mu     sync.Mutex
	values []mcpTaskPriorityEvent
}

func subscribeMCPTaskPriorityEvents(t *testing.T, eventBus *bus.MemoryEventBus) *mcpTaskPriorityEvents {
	t.Helper()
	eventsSeen := &mcpTaskPriorityEvents{}
	sub, err := eventBus.Subscribe(">", func(_ context.Context, event *bus.Event) error {
		if event.Type == events.TaskCreated || event.Type == events.TaskUpdated {
			eventsSeen.mu.Lock()
			eventsSeen.values = append(eventsSeen.values, mcpTaskPriorityEvent{Type: event.Type, Data: event.Data})
			eventsSeen.mu.Unlock()
		}
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return eventsSeen
}

func assertMCPTaskPriorityEvent(t *testing.T, eventsSeen *mcpTaskPriorityEvents, eventType, taskID, priority string) {
	t.Helper()
	eventsSeen.mu.Lock()
	defer eventsSeen.mu.Unlock()
	for _, event := range eventsSeen.values {
		if event.Type != eventType {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok || data["task_id"] != taskID {
			continue
		}
		assert.Equal(t, priority, data["priority"])
		return
	}
	t.Fatalf("event %q for task %q was not published", eventType, taskID)
}
