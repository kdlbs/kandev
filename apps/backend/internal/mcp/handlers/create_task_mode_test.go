package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mcpTestExternalContext(ctx context.Context) context.Context {
	return mcporigin.WithTrustedExternalTransport(ctx)
}

func mcpTestKanbanContext(
	ctx context.Context,
	workspaceID, taskID, sessionID string,
) context.Context {
	return mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		WorkspaceID:     workspaceID,
		CallerTaskID:    taskID,
		CallerSessionID: sessionID,
		Surface:         mcpprofile.SurfaceKanbanTask,
	})
}

func TestHandleCreateTask_KanbanSessionRejectsOfficeWorkspaceBeforePersistence(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)

	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	kanbanWorkspace := workspaces[0]
	kanbanWorkflow, err := svc.CreateWorkflow(ctx, &service.CreateWorkflowRequest{
		WorkspaceID: kanbanWorkspace.ID,
		Name:        "Kanban source workflow",
	})
	require.NoError(t, err)
	source, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: kanbanWorkspace.ID,
		WorkflowID:  kanbanWorkflow.ID,
		Title:       "Kanban source",
	})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "kanban-source-session",
		TaskID:    source.Task.ID,
		State:     models.TaskSessionStateWaitingForInput,
		IsPrimary: true,
		StartedAt: time.Now().UTC(),
	}))

	officeWorkspace := &models.Workspace{
		ID:   "office-mode-workspace",
		Name: "Office mode workspace",
	}
	require.NoError(t, repo.CreateWorkspace(ctx, officeWorkspace))
	officeWorkflowID, err := repo.EnsureOfficeWorkflow(ctx, officeWorkspace.ID)
	require.NoError(t, err)

	before, err := svc.ListTasks(ctx, officeWorkflowID)
	require.NoError(t, err)

	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	scoped := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		WorkspaceID:     kanbanWorkspace.ID,
		CallerTaskID:    source.Task.ID,
		CallerSessionID: "kanban-source-session",
		Surface:         mcpprofile.SurfaceKanbanTask,
	})
	resp, err := h.handleCreateTask(scoped, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     officeWorkspace.ID,
		"workflow_id":      officeWorkflowID,
		"title":            "Must not enter Office",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	after, err := svc.ListTasks(ctx, officeWorkflowID)
	require.NoError(t, err)
	require.Len(t, after, len(before), "rejected mode must not create a task")
}

// @covers AC-TASKS-MCP-WORKSPACE-MODE-001.2 AC-TASKS-MCP-WORKSPACE-MODE-002.1
func TestHandleCreateTask_OfficeSessionRejectsDirectMCPAction(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	kanbanWorkflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	require.Len(t, kanbanWorkflows, 1)

	officeWorkspace := &models.Workspace{ID: "office-direct-mcp", Name: "Office"}
	require.NoError(t, repo.CreateWorkspace(ctx, officeWorkspace))
	officeWorkflowID, err := repo.EnsureOfficeWorkflow(ctx, officeWorkspace.ID)
	require.NoError(t, err)
	before, err := svc.ListTasks(ctx, kanbanWorkflows[0].ID)
	require.NoError(t, err)

	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	officeCtx := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		CallerTaskID:    "office-task",
		CallerSessionID: "office-session",
		WorkspaceID:     officeWorkspace.ID,
		Surface:         mcpprofile.SurfaceOfficeTask,
	})
	resp, err := h.handleCreateTask(officeCtx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     workspaces[0].ID,
		"workflow_id":      kanbanWorkflows[0].ID,
		"title":            "Office MCP bypass",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	require.Contains(t, string(resp.Payload), "skills and runtime CLI")

	// A stale Office catalog is not the only protection. The same direct
	// action remains denied if it names the Office workflow explicitly.
	resp, err = h.handleCreateTask(officeCtx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id":     officeWorkspace.ID,
		"workflow_id":      officeWorkflowID,
		"title":            "Office MCP bypass 2",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)

	after, err := svc.ListTasks(ctx, kanbanWorkflows[0].ID)
	require.NoError(t, err)
	require.Len(t, after, len(before), "Office MCP rejection must not persist a task")
}

// @covers AC-TASKS-MCP-WORKSPACE-MODE-001.3 AC-TASKS-MCP-WORKSPACE-MODE-001.6
func TestHandleCreateTask_RequiresOneTrustedCallerIdentity(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	payload := map[string]interface{}{
		"workspace_id":     workspaces[0].ID,
		"workflow_id":      workflows[0].ID,
		"title":            "Identity gate",
		"agent_profile_id": "profile-1",
		"external_id":      "identity-gate-1",
		"start_agent":      false,
	}

	resp, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeUnauthorized)

	conflicting := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		CallerTaskID:    "caller-task",
		CallerSessionID: "caller-session",
		WorkspaceID:     workspaces[0].ID,
		Surface:         mcpprofile.SurfaceKanbanTask,
	})
	resp, err = h.handleCreateTask(mcpTestExternalContext(conflicting), makeWSMessage(t, ws.ActionMCPCreateTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeUnauthorized)

	tasks, err := svc.ListTasks(ctx, workflows[0].ID)
	require.NoError(t, err)
	require.Empty(t, tasks, "identity failures must not reach task creation")
}

// @covers AC-TASKS-MCP-WORKSPACE-MODE-001.3 AC-TASKS-MCP-WORKSPACE-MODE-001.4
func TestHandleCreateTask_KanbanSessionRejectsSpoofedSourceIdentity(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	source, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflows[0].ID,
		Title:       "Current task",
	})
	require.NoError(t, err)
	foreign, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflows[0].ID,
		Title:       "Foreign task",
	})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "current-session", TaskID: source.Task.ID, IsPrimary: true,
		State: models.TaskSessionStateWaitingForInput,
	}))

	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	caller := mcpTestKanbanContext(ctx, workspaces[0].ID, source.Task.ID, "current-session")
	resp, err := h.handleCreateTask(caller, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"source_task_id":    foreign.Task.ID,
		"source_session_id": "current-session",
		"workspace_id":      workspaces[0].ID,
		"workflow_id":       workflows[0].ID,
		"title":             "Spoofed source",
		"agent_profile_id":  "profile-1",
		"start_agent":       false,
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)

	tasks, err := svc.ListTasks(ctx, workflows[0].ID)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
}

// @covers AC-TASKS-MCP-WORKSPACE-MODE-001.4 AC-TASKS-MCP-WORKSPACE-MODE-001.5
func TestHandleCreateTask_KanbanRootDropsSourceRepositoriesAcrossWorkspaces(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	kanbanWorkspace := workspaces[0]
	kanbanWorkflows, err := svc.ListWorkflows(ctx, kanbanWorkspace.ID, false)
	require.NoError(t, err)
	require.Len(t, kanbanWorkflows, 1)

	otherWorkspace := &models.Workspace{ID: "kanban-other", Name: "Other Kanban"}
	require.NoError(t, repo.CreateWorkspace(ctx, otherWorkspace))
	otherWorkflow := &models.Workflow{ID: "kanban-other-workflow", WorkspaceID: otherWorkspace.ID, Name: "Other Board"}
	require.NoError(t, repo.CreateWorkflow(ctx, otherWorkflow))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "source-repository", WorkspaceID: kanbanWorkspace.ID, Name: "Source", DefaultBranch: "main",
	}))
	source, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:  kanbanWorkspace.ID,
		WorkflowID:   kanbanWorkflows[0].ID,
		Title:        "Source with repository",
		Repositories: []service.TaskRepositoryInput{{RepositoryID: "source-repository", BaseBranch: "main"}},
	})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "source-repository-session", TaskID: source.Task.ID, IsPrimary: true,
		State: models.TaskSessionStateWaitingForInput,
	}))

	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	caller := mcpTestKanbanContext(ctx, kanbanWorkspace.ID, source.Task.ID, "source-repository-session")
	resp, err := h.handleCreateTask(caller, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"source_task_id":   source.Task.ID,
		"workspace_id":     otherWorkspace.ID,
		"workflow_id":      otherWorkflow.ID,
		"title":            "Other workspace root",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &created))
	task, err := svc.GetTask(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, otherWorkspace.ID, task.WorkspaceID)
	require.Empty(t, task.Repositories, "a cross-workspace root must not inherit source repositories")
}

// @covers AC-TASKS-MCP-WORKSPACE-MODE-003.1 AC-TASKS-MCP-WORKSPACE-MODE-003.4
func TestHandleCreateTask_ExternalTransportReachesBothWorkspaceModes(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	kanbanWorkspace := workspaces[0]
	kanbanWorkflows, err := svc.ListWorkflows(ctx, kanbanWorkspace.ID, false)
	require.NoError(t, err)
	require.Len(t, kanbanWorkflows, 1)

	officeWorkspace := &models.Workspace{ID: "external-office", Name: "External Office"}
	require.NoError(t, repo.CreateWorkspace(ctx, officeWorkspace))
	officeWorkflowID, err := repo.EnsureOfficeWorkflow(ctx, officeWorkspace.ID)
	require.NoError(t, err)
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))

	for _, tc := range []struct {
		name, workspaceID, workflowID, title string
		wantOffice                           bool
	}{
		{name: "kanban", workspaceID: kanbanWorkspace.ID, workflowID: kanbanWorkflows[0].ID, title: "External Kanban", wantOffice: false},
		{name: "office", workspaceID: officeWorkspace.ID, workflowID: officeWorkflowID, title: "External Office", wantOffice: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := h.handleCreateTask(mcpTestExternalContext(ctx), makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
				"workspace_id":     tc.workspaceID,
				"workflow_id":      tc.workflowID,
				"title":            tc.title,
				"agent_profile_id": "profile-1",
				"start_agent":      false,
			}))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
			var created struct {
				ID string `json:"id"`
			}
			require.NoError(t, json.Unmarshal(resp.Payload, &created))
			task, err := svc.GetTask(ctx, created.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantOffice, task.IsFromOffice)
		})
	}

	beforeSpoofedSource, err := svc.ListTasks(ctx, kanbanWorkflows[0].ID)
	require.NoError(t, err)
	resp, err := h.handleCreateTask(mcpTestExternalContext(ctx), makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"source_task_id":    "caller-task-forged-by-client",
		"source_session_id": "caller-session-forged-by-client",
		"workspace_id":      kanbanWorkspace.ID,
		"workflow_id":       kanbanWorkflows[0].ID,
		"title":             "External source spoof",
		"agent_profile_id":  "profile-1",
		"external_id":       "external-source-spoof-1",
		"start_agent":       false,
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	afterSpoofedSource, err := svc.ListTasks(ctx, kanbanWorkflows[0].ID)
	require.NoError(t, err)
	require.Len(t, afterSpoofedSource, len(beforeSpoofedSource))

	beforeKanban, err := svc.ListTasks(ctx, kanbanWorkflows[0].ID)
	require.NoError(t, err)
	beforeOffice, err := svc.ListTasks(ctx, officeWorkflowID)
	require.NoError(t, err)
	resp, err = h.handleCreateTask(mcpTestExternalContext(ctx), makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":            "Ambiguous external root",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	afterKanban, err := svc.ListTasks(ctx, kanbanWorkflows[0].ID)
	require.NoError(t, err)
	afterOffice, err := svc.ListTasks(ctx, officeWorkflowID)
	require.NoError(t, err)
	require.Len(t, afterKanban, len(beforeKanban))
	require.Len(t, afterOffice, len(beforeOffice))
}

func TestHandleCreateTask_ExternalDefaultSelectsWritableWorkspace(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	initialWorkspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, initialWorkspaces, 1)
	require.NoError(t, repo.DeleteWorkspace(ctx, initialWorkspaces[0].ID))

	writableWorkspace := &models.Workspace{
		ID:      "external-default-writable",
		Name:    "External default writable",
		OwnerID: "external-user",
	}
	readableWorkspace := &models.Workspace{
		ID:      "external-default-readable",
		Name:    "External default readable",
		OwnerID: "other-user",
	}
	require.NoError(t, repo.CreateWorkspace(ctx, writableWorkspace))
	require.NoError(t, repo.CreateWorkspace(ctx, readableWorkspace))
	require.NoError(t, repo.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
		WorkspaceID: readableWorkspace.ID,
		UserID:      "external-user",
		Role:        string(authz.WorkspaceRoleViewer),
	}))
	writableWorkflow := &models.Workflow{
		ID:          "external-default-writable-workflow",
		WorkspaceID: writableWorkspace.ID,
		Name:        "Writable workflow",
	}
	readableWorkflow := &models.Workflow{
		ID:          "external-default-readable-workflow",
		WorkspaceID: readableWorkspace.ID,
		Name:        "Readable workflow",
	}
	require.NoError(t, repo.CreateWorkflow(ctx, writableWorkflow))
	require.NoError(t, repo.CreateWorkflow(ctx, readableWorkflow))

	externalCtx := authn.WithIdentity(
		mcpTestExternalContext(ctx),
		authn.Identity{UserID: "external-user", Role: authn.RoleMember},
	)
	visible, err := svc.ListWorkspaces(externalCtx)
	require.NoError(t, err)
	require.Len(t, visible, 2, "the caller can read both candidate workspaces")

	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	resp, err := h.handleCreateTask(externalCtx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workflow_id":      writableWorkflow.ID,
		"title":            "Default writable destination",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &created))
	task, err := svc.GetTask(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, writableWorkspace.ID, task.WorkspaceID)
}

func TestHandleCreateTask_KanbanCallerCannotExposeFoundOfficeIdentity(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	workspace := workspaces[0]
	workflows, err := svc.ListWorkflows(ctx, workspace.ID, false)
	require.NoError(t, err)
	require.Len(t, workflows, 1)
	source, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspace.ID,
		WorkflowID:  workflows[0].ID,
		Title:       "Kanban caller",
	})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "kanban-found-office-session",
		TaskID:    source.Task.ID,
		State:     models.TaskSessionStateWaitingForInput,
		IsPrimary: true,
	}))

	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	caller := mcpTestKanbanContext(ctx, workspace.ID, source.Task.ID, "kanban-found-office-session")
	for _, tc := range []struct {
		name    string
		settled bool
	}{
		{name: "settled", settled: true},
		{name: "unsettled", settled: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			externalID := "legacy-office-" + tc.name
			existing, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
				WorkspaceID: workspace.ID,
				WorkflowID:  workflows[0].ID,
				ProjectID:   "legacy-project-" + tc.name,
				Title:       "Hidden Office " + tc.name,
				ExternalID:  externalID,
			})
			require.NoError(t, err)
			persisted, err := svc.GetTask(ctx, existing.Task.ID)
			require.NoError(t, err)
			require.True(t, persisted.IsFromOffice, "fixture must be an Office projection in a mixed-mode workspace")
			if tc.settled {
				settled, err := repo.SettleTaskExternalID(ctx, persisted.ID, externalID, time.Now().UTC())
				require.NoError(t, err)
				require.True(t, settled)
			}

			resp, err := h.handleCreateTask(caller, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
				"workspace_id":     workspace.ID,
				"workflow_id":      workflows[0].ID,
				"title":            "Should not expose " + tc.name,
				"agent_profile_id": "profile-1",
				"external_id":      externalID,
				"start_agent":      false,
			}))
			require.NoError(t, err)
			assertWSError(t, resp, ws.ErrorCodeForbidden)
			assert.NotContains(t, string(resp.Payload), persisted.ID)
			assert.NotContains(t, string(resp.Payload), persisted.Title)

			tasks, err := svc.ListTasks(ctx, workflows[0].ID)
			require.NoError(t, err)
			matches := 0
			for _, task := range tasks {
				if task.ExternalID == externalID {
					matches++
				}
			}
			require.Equal(t, 1, matches, "the Found retry must not create or mutate a second task")
			afterExisting, err := svc.GetTask(ctx, persisted.ID)
			require.NoError(t, err)
			require.Equal(t, persisted.Title, afterExisting.Title)
			require.Equal(t, persisted.UpdatedAt, afterExisting.UpdatedAt)
		})
	}
}
