package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/projects"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

type handlerHarness struct {
	router     *gin.Engine
	token      string
	comments   *handlerCommentWriter
	status     *recordingTaskStatusUpdater
	projects   *recordingProjectManager
	tasks      *handlerTaskCreator
	runEvents  *recordingRunEvents
	agentSvc   *agents.AgentService
	repository *sqlite.Repository
}

type handlerCommentWriter struct {
	comments []*models.TaskComment
}

func (r *handlerCommentWriter) CreateComment(_ context.Context, comment *models.TaskComment) error {
	r.comments = append(r.comments, comment)
	return nil
}

type recordingTaskStatusUpdater struct {
	updates []TaskStatusUpdate
}

func (r *recordingTaskStatusUpdater) UpdateTaskStatusAsAgent(_ context.Context, update TaskStatusUpdate) error {
	r.updates = append(r.updates, update)
	return nil
}

type handlerTaskCreator struct {
	calls          int
	rootWorkspaces []string
	rootProjects   []string
	assignees      []string
	taskScopes     map[string]taskScope
	workspaceReads []string
	projectReads   []string
}

func (r *handlerTaskCreator) GetTaskWorkspaceID(_ context.Context, taskID string) (string, error) {
	r.workspaceReads = append(r.workspaceReads, taskID)
	if scope, ok := r.taskScopes[taskID]; ok {
		return scope.WorkspaceID, nil
	}
	return "ws-1", nil
}

func (r *handlerTaskCreator) GetTaskProjectID(_ context.Context, taskID string) (string, error) {
	r.projectReads = append(r.projectReads, taskID)
	if scope, ok := r.taskScopes[taskID]; ok {
		return scope.ProjectID, nil
	}
	return "", nil
}

func (r *handlerTaskCreator) CreateOfficeTaskAsAgent(
	_ context.Context,
	_ string,
	workspaceID string,
	projectID string,
	assigneeAgentID string,
	_ string,
	_ string,
) (string, error) {
	r.calls++
	r.rootWorkspaces = append(r.rootWorkspaces, workspaceID)
	r.rootProjects = append(r.rootProjects, projectID)
	r.assignees = append(r.assignees, assigneeAgentID)
	return "task-created", nil
}

func (r *handlerTaskCreator) CreateOfficeSubtaskAsAgent(
	_ context.Context,
	_ string,
	_ string,
	assigneeAgentID string,
	_ string,
	_ string,
) (string, error) {
	r.calls++
	r.assignees = append(r.assignees, assigneeAgentID)
	return "task-created", nil
}

type recordingRunEvent struct {
	runID     string
	eventType string
	level     string
	payload   map[string]interface{}
}

type recordingRunEvents struct {
	events []recordingRunEvent
}

func (r *recordingRunEvents) AppendRunEvent(
	_ context.Context,
	runID string,
	eventType string,
	level string,
	payload map[string]interface{},
) {
	r.events = append(r.events, recordingRunEvent{
		runID:     runID,
		eventType: eventType,
		level:     level,
		payload:   payload,
	})
}

func TestRuntimeHandler_PostCommentUsesRuntimeToken(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{
		CanPostComments: true,
	}.WithTaskScope("task-1"))

	resp := h.request(t, http.MethodPost, "/runtime/comments", map[string]string{
		"body": "runtime comment",
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	if len(h.comments.comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(h.comments.comments))
	}
	comment := h.comments.comments[0]
	if comment.TaskID != "task-1" || comment.AuthorID != "agent-1" || comment.AuthorType != "agent" {
		t.Fatalf("comment identity = %#v", comment)
	}
	assertActionRunEvent(t, h.runEvents, "post_comment", "task", "task-1")
}

func TestRuntimeHandler_WriteAndReadMemoryUsesRuntimeNamespace(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{
		CanReadMemory:  true,
		CanWriteMemory: true,
	}.WithTaskScope("task-1"))

	path := "/runtime/memory/workspaces/ws-1/memory/agents/agent-1/knowledge/runtime-note"
	put := h.request(t, http.MethodPut, path, map[string]string{"content": "remember this"})
	if put.Code != http.StatusOK {
		t.Fatalf("put status = %d, want %d; body=%s", put.Code, http.StatusOK, put.Body.String())
	}

	get := h.request(t, http.MethodGet, path, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d; body=%s", get.Code, http.StatusOK, get.Body.String())
	}
	var body struct {
		Memory models.AgentMemory `json:"memory"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode memory: %v", err)
	}
	if body.Memory.Content != "remember this" || body.Memory.Layer != "knowledge" {
		t.Fatalf("unexpected memory: %+v", body.Memory)
	}
	assertActionRunEvent(t, h.runEvents, "write_memory", "memory",
		"/workspaces/ws-1/memory/agents/agent-1/knowledge/runtime-note")
}

func TestRuntimeHandler_CreateAgentBindsSnakeCasePayload(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{
		CanCreateAgents: true,
	}.WithTaskScope("task-1"))

	resp := h.request(t, http.MethodPost, "/runtime/agents", map[string]interface{}{
		"name":                    "Runtime Worker",
		"role":                    "worker",
		"desired_skills":          `["runtime-skill"]`,
		"executor_preference":     `{"type":"local_pc"}`,
		"max_concurrent_sessions": 2,
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	var body struct {
		Agent models.AgentInstance `json:"agent"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode agent: %v", err)
	}
	if body.Agent.DesiredSkills != `["runtime-skill"]` ||
		body.Agent.ExecutorPreference != `{"type":"local_pc"}` ||
		body.Agent.MaxConcurrentSessions != 2 {
		t.Fatalf("snake-case fields did not bind: %+v", body.Agent)
	}
	assertActionRunEvent(t, h.runEvents, "create_agent", "agent", body.Agent.ID)
}

func TestRuntimeHandler_ListProjectsUsesTokenWorkspace(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanListProjects: true})
	h.projects.projects = []*models.Project{
		{ID: "project-1", WorkspaceID: "ws-1", Name: "Alpha"},
	}

	resp := h.request(t, http.MethodGet, "/runtime/projects", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if len(h.projects.listWorkspaceIDs) != 1 || h.projects.listWorkspaceIDs[0] != "ws-1" {
		t.Fatalf("list workspaces = %+v, want [ws-1]", h.projects.listWorkspaceIDs)
	}
	var body struct {
		Projects []*models.Project `json:"projects"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode projects: %v", err)
	}
	if len(body.Projects) != 1 || body.Projects[0].ID != "project-1" {
		t.Fatalf("projects = %+v", body.Projects)
	}
}

func TestRuntimeHandler_CreateProjectLogsRunEvent(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateProjects: true})

	resp := h.request(t, http.MethodPost, "/runtime/projects", map[string]interface{}{
		"name":         "Alpha",
		"workspace_id": "ws-other",
		"repositories": []string{"github.com/acme/alpha"},
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	if len(h.projects.created) != 1 || h.projects.created[0].WorkspaceID != "ws-1" {
		t.Fatalf("created projects = %+v", h.projects.created)
	}
	assertActionRunEvent(t, h.runEvents, "create_project", "project", "project-created")
}

func TestRuntimeHandler_CreateProjectDeniedLogsRunEvent(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanListProjects: true})

	resp := h.request(t, http.MethodPost, "/runtime/projects", map[string]interface{}{"name": "Alpha"})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if len(h.projects.created) != 0 {
		t.Fatalf("created projects = %d, want 0", len(h.projects.created))
	}
	assertDeniedRunEvent(t, h.runEvents, "create_project", "project", "")
}

func TestRuntimeHandler_CreateProjectDeniesCrossWorkspaceLeadAndLogsRunEvent(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateProjects: true})
	if err := h.repository.CreateAgentInstance(context.Background(), &models.AgentInstance{
		ID: "lead-2", WorkspaceID: "ws-2", Name: "Other lead", Role: models.AgentRoleWorker,
	}); err != nil {
		t.Fatalf("create cross-workspace lead: %v", err)
	}

	resp := h.request(t, http.MethodPost, "/runtime/projects", map[string]interface{}{
		"name": "Alpha", "lead_agent_profile_id": "lead-2",
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if len(h.projects.created) != 0 {
		t.Fatalf("created projects = %d, want 0", len(h.projects.created))
	}
	assertDeniedRunEvent(t, h.runEvents, "create_project", "project", "lead-2")
}

func TestRuntimeHandler_CreateProjectDeniesMissingLeadAndLogsRunEvent(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateProjects: true})

	resp := h.request(t, http.MethodPost, "/runtime/projects", map[string]interface{}{
		"name": "Alpha", "lead_agent_profile_id": "missing-lead",
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if len(h.projects.created) != 0 {
		t.Fatalf("created projects = %d, want 0", len(h.projects.created))
	}
	assertDeniedRunEvent(t, h.runEvents, "create_project", "project", "missing-lead")
}

func TestRuntimeHandler_CreateProjectDeniesSameWorkspaceLeadAliasAndLogsRunEvent(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateProjects: true})
	if err := h.repository.CreateAgentInstance(context.Background(), &models.AgentInstance{
		ID: "lead-1", WorkspaceID: "ws-1", Name: "Platform lead", Role: models.AgentRoleWorker,
	}); err != nil {
		t.Fatalf("create same-workspace lead: %v", err)
	}

	resp := h.request(t, http.MethodPost, "/runtime/projects", map[string]interface{}{
		"name": "Alpha", "lead_agent_profile_id": "Platform lead",
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if len(h.projects.created) != 0 {
		t.Fatalf("created projects = %d, want 0", len(h.projects.created))
	}
	assertDeniedRunEvent(t, h.runEvents, "create_project", "project", "Platform lead")
}

func TestRuntimeHandler_CreateTaskUsesRuntimeToken(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateTasks: true})

	resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
		"title": "Runtime task",
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
}

func TestRuntimeHandler_CreateTaskWithParentRequiresSubtaskCapability(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateTasks: true}.WithTaskScope("task-1"))

	resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
		"title": "child", "parent_id": "task-1",
	})

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if h.tasks.calls != 0 {
		t.Fatalf("task creator called without subtask capability: %d", h.tasks.calls)
	}
	if len(h.tasks.workspaceReads) != 0 || len(h.tasks.projectReads) != 0 {
		t.Fatalf("parent relations read before capability denial: %#v/%#v", h.tasks.workspaceReads, h.tasks.projectReads)
	}
	assertDeniedRunEvent(t, h.runEvents, "create_task", "task", "task-1")
}

func TestRuntimeHandler_CreateTaskWithParentRequiresParentScope(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateSubtasks: true}.WithTaskScope("task-1"))
	h.tasks.taskScopes = map[string]taskScope{"parent-2": {WorkspaceID: "ws-1"}}

	resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
		"title": "child", "parent_id": "parent-2",
	})

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if h.tasks.calls != 0 {
		t.Fatalf("task creator called for out-of-scope parent: %d", h.tasks.calls)
	}
	if len(h.tasks.workspaceReads) != 0 || len(h.tasks.projectReads) != 0 {
		t.Fatalf("parent relations read before scope denial: %#v/%#v", h.tasks.workspaceReads, h.tasks.projectReads)
	}
	assertDeniedRunEvent(t, h.runEvents, "create_task", "task", "parent-2")
}

func TestRuntimeHandler_CreateTaskWithParentUsesScopedSubtaskCapability(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateSubtasks: true}.WithTaskScope("task-1"))

	resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
		"title": "child", "parent_id": "task-1",
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	if h.tasks.calls != 1 {
		t.Fatalf("task creator calls = %d, want 1", h.tasks.calls)
	}
	assertActionRunEvent(t, h.runEvents, "create_task", "task", "task-created")
}

func TestRuntimeHandler_CreateTaskRejectsBlankTitleAndLogsRunEvent(t *testing.T) {
	for _, parentID := range []string{"", "parent-1"} {
		name := "root"
		if parentID != "" {
			name = "child"
		}
		t.Run(name, func(t *testing.T) {
			h := newRuntimeHandlerHarness(t, Capabilities{
				CanCreateTasks:    true,
				CanCreateSubtasks: true,
			}.WithTaskScope("parent-1"))

			resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
				"title": " \t ", "parent_id": parentID,
			})

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
			}
			if h.tasks.calls != 0 {
				t.Fatalf("task creator called for blank title: %d", h.tasks.calls)
			}
			assertDeniedRunEvent(t, h.runEvents, "create_task", "task", parentID)
		})
	}
}

func TestRuntimeHandler_CreateTaskRejectsUnsupportedFields(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateTasks: true})

	resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
		"title": "Runtime task", "priority": "high",
	})

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	if h.tasks.calls != 0 {
		t.Fatalf("task creator called for unsupported input: %d", h.tasks.calls)
	}
}

func TestRuntimeHandler_CreateTaskForcesSignedWorkspaceAndAssignment(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateTasks: true})
	h.projects.projects = []*models.Project{{ID: "project-1", WorkspaceID: "ws-1"}}

	resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
		"title": "Runtime task", "workspace_id": "ws-2", "project_id": "project-1", "assignee": "agent-1",
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	if len(h.tasks.rootWorkspaces) != 1 || h.tasks.rootWorkspaces[0] != "ws-1" {
		t.Fatalf("root workspaces = %#v, want signed ws-1", h.tasks.rootWorkspaces)
	}
	if h.tasks.rootProjects[0] != "project-1" || h.tasks.assignees[0] != "agent-1" {
		t.Fatalf("project/assignee = %#v/%#v", h.tasks.rootProjects, h.tasks.assignees)
	}
	assertActionRunEvent(t, h.runEvents, "create_task", "task", "task-created")
}

func TestRuntimeHandler_CreateTaskDeniesCrossWorkspaceRelations(t *testing.T) {
	tests := []struct {
		name          string
		prepare       func(*handlerHarness)
		payload       map[string]interface{}
		auditTargetID string
	}{
		{
			name: "project",
			prepare: func(h *handlerHarness) {
				h.projects.projects = []*models.Project{{ID: "project-2", WorkspaceID: "ws-2"}}
			},
			payload: map[string]interface{}{"title": "task", "project_id": "project-2"},
		},
		{
			name: "parent",
			prepare: func(h *handlerHarness) {
				h.tasks.taskScopes = map[string]taskScope{"parent-2": {WorkspaceID: "ws-2"}}
			},
			payload:       map[string]interface{}{"title": "task", "parent_id": "parent-2"},
			auditTargetID: "parent-2",
		},
		{
			name: "assignee",
			prepare: func(h *handlerHarness) {
				if err := h.repository.CreateAgentInstance(context.Background(), &models.AgentInstance{
					ID: "agent-2", WorkspaceID: "ws-2", Name: "Other worker", Role: models.AgentRoleWorker,
				}); err != nil {
					t.Fatalf("create cross-workspace agent: %v", err)
				}
			},
			payload: map[string]interface{}{"title": "task", "assignee": "agent-2"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newRuntimeHandlerHarness(t, Capabilities{
				CanCreateTasks:    true,
				CanCreateSubtasks: true,
			}.WithTaskScope("parent-2"))
			tc.prepare(h)

			resp := h.request(t, http.MethodPost, "/runtime/tasks", tc.payload)
			if resp.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
			}
			if h.tasks.calls != 0 {
				t.Fatalf("task creator called on denied relation: %d", h.tasks.calls)
			}
			assertDeniedRunEvent(t, h.runEvents, "create_task", "task", tc.auditTargetID)
		})
	}
}

func TestRuntimeHandler_CreateTaskDeniesSameWorkspaceAssigneeAliasAndLogsRunEvent(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{CanCreateTasks: true})
	if err := h.repository.CreateAgentInstance(context.Background(), &models.AgentInstance{
		ID: "agent-2", WorkspaceID: "ws-1", Name: "Platform worker", Role: models.AgentRoleWorker,
	}); err != nil {
		t.Fatalf("create same-workspace assignee: %v", err)
	}

	resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{
		"title": "task", "assignee": "Platform worker",
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if h.tasks.calls != 0 {
		t.Fatalf("task creator called for non-canonical assignee: %d", h.tasks.calls)
	}
	assertDeniedRunEvent(t, h.runEvents, "create_task", "task", "")
}

func TestRuntimeHandler_CreateTaskDeniedWithoutCapability(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{})
	resp := h.request(t, http.MethodPost, "/runtime/tasks", map[string]interface{}{"title": "task"})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if h.tasks.calls != 0 {
		t.Fatalf("task creator called without capability: %d", h.tasks.calls)
	}
	assertDeniedRunEvent(t, h.runEvents, "create_task", "task", "")
}

func TestRuntimeHandler_CreateProjectPersistsOnlyInSignedTokenWorkspace(t *testing.T) {
	h := newRuntimeHandlerHarnessWithProjectManager(t, Capabilities{CanCreateProjects: true},
		func(repo *sqlite.Repository) ProjectManager {
			return projects.NewProjectService(repo, logger.Default(), nil)
		})

	resp := h.request(t, http.MethodPost, "/runtime/projects", map[string]interface{}{
		"name":         "Alpha",
		"workspace_id": "ws-other",
		"repositories": []string{"https://github.com/acme/alpha"},
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}

	var body struct {
		Project models.Project `json:"project"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	persisted, err := h.repository.GetProject(context.Background(), body.Project.ID)
	if err != nil {
		t.Fatalf("get persisted project: %v", err)
	}
	if persisted.WorkspaceID != "ws-1" {
		t.Fatalf("persisted workspace = %q, want token workspace ws-1", persisted.WorkspaceID)
	}
	otherWorkspaceProjects, err := h.repository.ListProjects(context.Background(), "ws-other")
	if err != nil {
		t.Fatalf("list other workspace projects: %v", err)
	}
	if len(otherWorkspaceProjects) != 0 {
		t.Fatalf("cross-workspace projects = %+v, want none", otherWorkspaceProjects)
	}
}

func TestRuntimeHandler_DeniedProjectCreationDoesNotPersist(t *testing.T) {
	h := newRuntimeHandlerHarnessWithProjectManager(t, Capabilities{CanListProjects: true},
		func(repo *sqlite.Repository) ProjectManager {
			return projects.NewProjectService(repo, logger.Default(), nil)
		})

	resp := h.request(t, http.MethodPost, "/runtime/projects", map[string]interface{}{"name": "Alpha"})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	persisted, err := h.repository.ListProjects(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list persisted projects: %v", err)
	}
	if len(persisted) != 0 {
		t.Fatalf("persisted projects = %+v, want none", persisted)
	}
}

func TestRuntimeHandler_DeniesOutOfScopeStatusUpdateAndLogsRunEvent(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{
		CanUpdateTaskStatus: true,
	}.WithTaskScope("task-1"))

	resp := h.request(t, http.MethodPost, "/runtime/tasks/task-2/status", map[string]string{
		"status": "in_review",
	})

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if len(h.status.updates) != 0 {
		t.Fatalf("status updates = %d, want 0", len(h.status.updates))
	}
	assertDeniedRunEvent(t, h.runEvents, "update_task_status", "task", "task-2")
}

func TestRuntimeHandler_DeniesMissingCapabilityAndLogsRunEvent(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{}.WithTaskScope("task-1"))

	resp := h.request(t, http.MethodPost, "/runtime/tasks/task-1/status", map[string]string{
		"status": "in_review",
	})

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if len(h.status.updates) != 0 {
		t.Fatalf("status updates = %d, want 0", len(h.status.updates))
	}
	assertDeniedRunEvent(t, h.runEvents, "update_task_status", "task", "task-1")
}

func newRuntimeHandlerHarness(t *testing.T, caps Capabilities) *handlerHarness {
	return newRuntimeHandlerHarnessWithProjectManager(t, caps, nil)
}

func newRuntimeHandlerHarnessWithProjectManager(
	t *testing.T,
	caps Capabilities,
	projectManagerFactory func(*sqlite.Repository) ProjectManager,
) *handlerHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	agentSvc := agents.NewAgentService(repo, logger.Default(), nil)
	agentSvc.SetAuth(agents.NewAgentAuth("runtime-handler-test-key"))
	agent := &models.AgentInstance{
		ID:          "agent-1",
		WorkspaceID: "ws-1",
		Name:        "CEO",
		Role:        models.AgentRoleCEO,
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	capabilityJSON, err := MarshalCapabilities(caps)
	if err != nil {
		t.Fatalf("marshal capabilities: %v", err)
	}
	token, err := agentSvc.MintRuntimeJWT("agent-1", "task-1", "ws-1", "run-1", "sess-1", capabilityJSON)
	if err != nil {
		t.Fatalf("mint runtime token: %v", err)
	}
	comments := &handlerCommentWriter{}
	status := &recordingTaskStatusUpdater{}
	tasks := &handlerTaskCreator{}
	projects := &recordingProjectManager{}
	var projectManager ProjectManager = projects
	if projectManagerFactory != nil {
		projectManager = projectManagerFactory(repo)
	}
	runEvents := &recordingRunEvents{}
	router := gin.New()
	RegisterRoutes(router.Group(""), NewHandler(
		agentSvc,
		NewActions(ActionDependencies{
			Comments:      comments,
			Tasks:         tasks,
			TaskStatus:    status,
			Agents:        agentSvc,
			Projects:      projectManager,
			AgentModifier: agentSvc,
		}),
		nil,
		runEvents,
	))
	return &handlerHarness{
		router:     router,
		token:      token,
		comments:   comments,
		status:     status,
		projects:   projects,
		tasks:      tasks,
		runEvents:  runEvents,
		agentSvc:   agentSvc,
		repository: repo,
	}
}

func (h *handlerHarness) request(
	t *testing.T,
	method string,
	path string,
	payload interface{},
) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, req)
	return resp
}

func assertActionRunEvent(
	t *testing.T,
	runEvents *recordingRunEvents,
	action string,
	targetType string,
	targetID string,
) {
	t.Helper()
	if len(runEvents.events) != 1 {
		t.Fatalf("run events = %d, want 1", len(runEvents.events))
	}
	event := runEvents.events[0]
	if event.runID != "run-1" || event.eventType != "runtime.action" || event.level != "info" {
		t.Fatalf("event identity = %#v", event)
	}
	if event.payload["action"] != action ||
		event.payload["target_type"] != targetType ||
		event.payload["target_id"] != targetID ||
		event.payload["agent_id"] != "agent-1" ||
		event.payload["session_id"] != "sess-1" {
		t.Fatalf("event payload = %#v", event.payload)
	}
}

func assertDeniedRunEvent(
	t *testing.T,
	runEvents *recordingRunEvents,
	action string,
	targetType string,
	targetID string,
) {
	t.Helper()
	if len(runEvents.events) != 1 {
		t.Fatalf("run events = %d, want 1", len(runEvents.events))
	}
	event := runEvents.events[0]
	if event.runID != "run-1" || event.eventType != "runtime.denied" || event.level != "warn" {
		t.Fatalf("event identity = %#v", event)
	}
	if event.payload["action"] != action ||
		event.payload["target_type"] != targetType ||
		event.payload["target_id"] != targetID ||
		event.payload["agent_id"] != "agent-1" ||
		event.payload["session_id"] != "sess-1" {
		t.Fatalf("event payload = %#v", event.payload)
	}
}
