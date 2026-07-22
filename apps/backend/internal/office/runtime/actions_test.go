package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

func TestCapabilitiesMarshalProjectCapabilityKeys(t *testing.T) {
	raw, err := MarshalCapabilities(Capabilities{})
	if err != nil {
		t.Fatalf("marshal capabilities: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	for _, key := range []string{"list_projects", "create_project"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("capability JSON missing %q: %s", key, raw)
		}
	}
}

func TestRunContextAllowsCurrentAndScopedTaskMutations(t *testing.T) {
	ctx := RunContext{
		TaskID: "task-current",
		Capabilities: Capabilities{
			CanUpdateTaskStatus: true,
			AllowedTaskIDs:      []string{"task-extra"},
		},
	}

	for _, taskID := range []string{"task-current", "task-extra"} {
		if !ctx.CanMutateTask(taskID) {
			t.Fatalf("expected task %q to be mutable", taskID)
		}
	}

	if ctx.CanMutateTask("task-other") {
		t.Fatal("expected unrelated task to be denied")
	}
}

func TestRunContextWildcardAllowsAnyTaskMutation(t *testing.T) {
	ctx := RunContext{
		TaskID: "task-current",
		Capabilities: Capabilities{
			CanUpdateTaskStatus: true,
			AllowedTaskIDs:      []string{WildcardTaskScope},
		},
	}

	if !ctx.CanMutateTask("task-other") {
		t.Fatal("expected wildcard task scope to allow unrelated task")
	}
}

func TestActionsPostCommentDeniesMissingCapability(t *testing.T) {
	writer := &recordingCommentWriter{}
	actions := NewActions(ActionDependencies{Comments: writer})
	runCtx := RunContext{AgentID: "agent-1", TaskID: "task-1"}

	err := actions.PostComment(context.Background(), runCtx, "task-1", "hello")
	if !errors.Is(err, ErrCapabilityDenied) || !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(writer.comments) != 0 {
		t.Fatal("comment writer should not be called when capability is missing")
	}
}

func TestActionsPostCommentWritesAgentComment(t *testing.T) {
	writer := &recordingCommentWriter{}
	actions := NewActions(ActionDependencies{Comments: writer})
	runCtx := RunContext{
		AgentID:   "agent-1",
		TaskID:    "task-1",
		RunID:     "run-1",
		SessionID: "session-1",
		Capabilities: Capabilities{
			CanPostComments: true,
		},
	}

	if err := actions.PostComment(context.Background(), runCtx, "task-1", "hello"); err != nil {
		t.Fatalf("post comment: %v", err)
	}

	if len(writer.comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(writer.comments))
	}
	comment := writer.comments[0]
	if comment.AuthorType != "agent" || comment.AuthorID != "agent-1" {
		t.Fatalf("unexpected author: %s/%s", comment.AuthorType, comment.AuthorID)
	}
	if comment.Source != "agent" || comment.Body != "hello" || comment.TaskID != "task-1" {
		t.Fatalf("unexpected comment payload: %+v", comment)
	}
	if comment.ID == "" || comment.CreatedAt.IsZero() {
		t.Fatalf("expected generated id and timestamp: %+v", comment)
	}
}

func TestActionsCreateSubtaskDeniesWithoutCapability(t *testing.T) {
	creator := &recordingTaskCreator{}
	actions := NewActions(ActionDependencies{Tasks: creator})
	runCtx := RunContext{AgentID: "agent-1", WorkspaceID: "ws-1", TaskID: "task-1"}

	_, err := actions.CreateSubtask(context.Background(), runCtx, CreateSubtaskInput{
		Title: "child",
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(creator.calls) != 0 {
		t.Fatal("task creator should not be called when capability is missing")
	}
}

func TestActionsCreateSubtaskPreservesCallerIdentity(t *testing.T) {
	creator := &recordingTaskCreator{taskID: "created-task"}
	actions := NewActions(ActionDependencies{Tasks: creator})
	runCtx := RunContext{
		AgentID:     "agent-1",
		WorkspaceID: "ws-1",
		TaskID:      "task-parent",
		Capabilities: Capabilities{
			CanCreateSubtasks: true,
		},
	}

	taskID, err := actions.CreateSubtask(context.Background(), runCtx, CreateSubtaskInput{
		AssigneeAgentID: "agent-2",
		Title:           "child",
		Description:     "details",
	})
	if err != nil {
		t.Fatalf("create subtask: %v", err)
	}
	if taskID != "created-task" {
		t.Fatalf("task id = %q, want created-task", taskID)
	}
	if len(creator.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(creator.calls))
	}
	call := creator.calls[0]
	if call.CallerAgentID != "agent-1" {
		t.Fatalf("unexpected caller: %+v", call)
	}
	if call.ParentTaskID != "task-parent" || call.AssigneeAgentID != "agent-2" {
		t.Fatalf("unexpected task routing: %+v", call)
	}
}

func TestActionsCreateTaskUsesTokenWorkspaceAndAssignment(t *testing.T) {
	creator := &recordingTaskCreator{taskID: "created-task"}
	projects := &recordingProjectManager{projects: []*models.Project{
		{ID: "project-1", WorkspaceID: "ws-1"},
	}}
	agents := &recordingAgentModifier{agents: map[string]*models.AgentInstance{
		"agent-2": {ID: "agent-2", WorkspaceID: "ws-1"},
	}}
	actions := NewActions(ActionDependencies{Tasks: creator, Projects: projects, AgentModifier: agents})
	runCtx := RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1",
		Capabilities: Capabilities{CanCreateTasks: true},
	}

	taskID, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
		Title: "Root task", ProjectID: "project-1", AssigneeAgentID: "agent-2",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if taskID != "created-task" || len(creator.calls) != 1 {
		t.Fatalf("task id/calls = %q/%d", taskID, len(creator.calls))
	}
	call := creator.calls[0]
	if !call.Root || call.WorkspaceID != "ws-1" || call.ProjectID != "project-1" || call.AssigneeAgentID != "agent-2" {
		t.Fatalf("create call = %#v", call)
	}
}

func TestActionsCreateTaskWithParentRequiresSubtaskCapabilityBeforeLookup(t *testing.T) {
	creator := &recordingTaskCreator{taskScopes: map[string]taskScope{
		"parent-1": {WorkspaceID: "ws-1"},
	}}
	actions := NewActions(ActionDependencies{Tasks: creator})
	runCtx := RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1", TaskID: "parent-1",
		Capabilities: Capabilities{CanCreateTasks: true},
	}

	_, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
		Title: "child", ParentTaskID: "parent-1",
	})
	if !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("error = %v, want subtask capability denial", err)
	}
	if len(creator.calls) != 0 {
		t.Fatalf("creator called without subtask capability: %#v", creator.calls)
	}
	if len(creator.workspaceLookups) != 0 || len(creator.projectLookups) != 0 {
		t.Fatalf("parent relations looked up before capability denial: %#v/%#v", creator.workspaceLookups, creator.projectLookups)
	}
}

func TestActionsCreateTaskWithParentRequiresParentScope(t *testing.T) {
	creator := &recordingTaskCreator{taskScopes: map[string]taskScope{
		"parent-2": {WorkspaceID: "ws-1"},
	}}
	actions := NewActions(ActionDependencies{Tasks: creator})
	runCtx := RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1", TaskID: "task-1",
		Capabilities: Capabilities{CanCreateSubtasks: true},
	}

	_, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
		Title: "child", ParentTaskID: "parent-2",
	})
	if !errors.Is(err, ErrTaskOutOfScope) {
		t.Fatalf("error = %v, want task scope denial", err)
	}
	if len(creator.calls) != 0 {
		t.Fatalf("creator called for out-of-scope parent: %#v", creator.calls)
	}
	if len(creator.workspaceLookups) != 0 || len(creator.projectLookups) != 0 {
		t.Fatalf("parent relations looked up before scope denial: %#v/%#v", creator.workspaceLookups, creator.projectLookups)
	}
}

func TestActionsCreateTaskWithParentUsesScopedSubtaskCapability(t *testing.T) {
	creator := &recordingTaskCreator{taskID: "child-1", taskScopes: map[string]taskScope{
		"parent-1": {WorkspaceID: "ws-1", ProjectID: "project-1"},
	}}
	actions := NewActions(ActionDependencies{Tasks: creator})
	runCtx := RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1", TaskID: "task-1",
		Capabilities: Capabilities{
			CanCreateSubtasks: true,
			AllowedTaskIDs:    []string{"parent-1"},
		},
	}

	taskID, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
		Title: "child", ParentTaskID: "parent-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if taskID != "child-1" || len(creator.calls) != 1 || creator.calls[0].ParentTaskID != "parent-1" {
		t.Fatalf("task id/calls = %q/%#v", taskID, creator.calls)
	}
}

func TestActionsCreateTaskDeniesNonCanonicalSameWorkspaceAssigneeBeforePersistence(t *testing.T) {
	creator := &recordingTaskCreator{taskID: "created-task"}
	agents := &recordingAgentModifier{agents: map[string]*models.AgentInstance{
		"Platform worker": {ID: "agent-2", WorkspaceID: "ws-1"},
	}}
	actions := NewActions(ActionDependencies{Tasks: creator, AgentModifier: agents})
	runCtx := RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1",
		Capabilities: Capabilities{CanCreateTasks: true},
	}

	_, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
		Title: "Root task", AssigneeAgentID: "Platform worker",
	})
	if !errors.Is(err, ErrWorkspaceOutOfScope) {
		t.Fatalf("error = %v, want workspace scope denial", err)
	}
	if len(creator.calls) != 0 {
		t.Fatalf("creator called for non-canonical assignee: %#v", creator.calls)
	}
}

func TestActionsCreateTaskTrimsCanonicalAssigneeBeforePersistence(t *testing.T) {
	creator := &recordingTaskCreator{taskID: "created-task"}
	agents := &recordingAgentModifier{agents: map[string]*models.AgentInstance{
		"agent-2": {ID: "agent-2", WorkspaceID: "ws-1"},
	}}
	actions := NewActions(ActionDependencies{Tasks: creator, AgentModifier: agents})
	runCtx := RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1",
		Capabilities: Capabilities{CanCreateTasks: true},
	}

	_, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
		Title: "Root task", AssigneeAgentID: " agent-2 ",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if len(creator.calls) != 1 || creator.calls[0].AssigneeAgentID != "agent-2" {
		t.Fatalf("creator calls = %#v, want canonical assignee agent-2", creator.calls)
	}
}

func TestActionsCreateTaskNormalizesTitleBeforePersistence(t *testing.T) {
	creator := &recordingTaskCreator{taskID: "created-task"}
	actions := NewActions(ActionDependencies{Tasks: creator})
	runCtx := RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1",
		Capabilities: Capabilities{CanCreateTasks: true},
	}

	_, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{Title: "  Root task  "})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if len(creator.calls) != 1 || creator.calls[0].Title != "Root task" {
		t.Fatalf("creator calls = %#v, want normalized title", creator.calls)
	}
}

func TestActionsCreateTaskRejectsBlankRootAndChildTitlesBeforePersistence(t *testing.T) {
	for _, parentID := range []string{"", "parent-1"} {
		name := "root"
		if parentID != "" {
			name = "child"
		}
		t.Run(name, func(t *testing.T) {
			creator := &recordingTaskCreator{taskScopes: map[string]taskScope{
				"parent-1": {WorkspaceID: "ws-1"},
			}}
			actions := NewActions(ActionDependencies{Tasks: creator})
			runCtx := RunContext{
				AgentID: "agent-1", WorkspaceID: "ws-1",
				Capabilities: Capabilities{
					CanCreateTasks:    true,
					CanCreateSubtasks: true,
					AllowedTaskIDs:    []string{"parent-1"},
				},
			}

			_, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
				Title: " \t ", ParentTaskID: parentID,
			})
			if err == nil || !strings.Contains(err.Error(), "task title is required") {
				t.Fatalf("error = %v, want required title error", err)
			}
			if len(creator.calls) != 0 {
				t.Fatalf("creator called for blank title: %#v", creator.calls)
			}
		})
	}
}

func TestActionsCreateTaskDeniesCrossWorkspaceRelationsBeforePersistence(t *testing.T) {
	tests := []struct {
		name     string
		input    CreateTaskInput
		projects []*models.Project
		tasks    map[string]taskScope
		agents   map[string]*models.AgentInstance
	}{
		{
			name: "project", input: CreateTaskInput{Title: "task", ProjectID: "project-2"},
			projects: []*models.Project{{ID: "project-2", WorkspaceID: "ws-2"}},
		},
		{
			name: "parent", input: CreateTaskInput{Title: "task", ParentTaskID: "parent-2"},
			tasks: map[string]taskScope{"parent-2": {WorkspaceID: "ws-2"}},
		},
		{
			name: "assignee", input: CreateTaskInput{Title: "task", AssigneeAgentID: "agent-2"},
			agents: map[string]*models.AgentInstance{"agent-2": {ID: "agent-2", WorkspaceID: "ws-2"}},
		},
		{
			name: "missing assignee", input: CreateTaskInput{Title: "task", AssigneeAgentID: "missing-agent"},
			agents: map[string]*models.AgentInstance{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creator := &recordingTaskCreator{taskScopes: tc.tasks}
			actions := NewActions(ActionDependencies{
				Tasks:         creator,
				Projects:      &recordingProjectManager{projects: tc.projects},
				AgentModifier: &recordingAgentModifier{agents: tc.agents},
			})
			runCtx := RunContext{
				AgentID: "agent-1", WorkspaceID: "ws-1",
				Capabilities: Capabilities{
					CanCreateTasks:    true,
					CanCreateSubtasks: true,
					AllowedTaskIDs:    []string{"parent-2"},
				},
			}

			_, err := actions.CreateTask(context.Background(), runCtx, tc.input)
			if !errors.Is(err, ErrWorkspaceOutOfScope) {
				t.Fatalf("error = %v, want workspace scope denial", err)
			}
			if len(creator.calls) != 0 {
				t.Fatalf("creator called on denied relation: %#v", creator.calls)
			}
		})
	}
}

func TestActionsCreateTaskDeniesParentProjectMismatchBeforePersistence(t *testing.T) {
	creator := &recordingTaskCreator{taskScopes: map[string]taskScope{
		"parent-1": {WorkspaceID: "ws-1", ProjectID: "project-1"},
	}}
	projects := &recordingProjectManager{projects: []*models.Project{
		{ID: "project-2", WorkspaceID: "ws-1"},
	}}
	actions := NewActions(ActionDependencies{Tasks: creator, Projects: projects})
	runCtx := RunContext{
		AgentID: "agent-1", WorkspaceID: "ws-1",
		Capabilities: Capabilities{
			CanCreateSubtasks: true,
			AllowedTaskIDs:    []string{"parent-1"},
		},
	}

	_, err := actions.CreateTask(context.Background(), runCtx, CreateTaskInput{
		Title: "child", ParentTaskID: "parent-1", ProjectID: "project-2",
	})
	if !errors.Is(err, ErrWorkspaceOutOfScope) {
		t.Fatalf("error = %v, want project scope denial", err)
	}
	if len(creator.calls) != 0 {
		t.Fatalf("creator called on mismatch: %#v", creator.calls)
	}
}

func TestActionsCreateTaskDeniesMissingCapabilityBeforePersistence(t *testing.T) {
	creator := &recordingTaskCreator{}
	actions := NewActions(ActionDependencies{Tasks: creator})
	_, err := actions.CreateTask(context.Background(), RunContext{WorkspaceID: "ws-1"}, CreateTaskInput{Title: "task"})
	if !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("error = %v, want capability denial", err)
	}
	if len(creator.calls) != 0 {
		t.Fatalf("creator called without capability: %#v", creator.calls)
	}
}

func TestActionsUpdateTaskStatusDeniesUnscopedTask(t *testing.T) {
	updater := &recordingStatusUpdater{}
	actions := NewActions(ActionDependencies{TaskStatus: updater})
	runCtx := RunContext{
		AgentID: "agent-1",
		TaskID:  "task-1",
		Capabilities: Capabilities{
			CanUpdateTaskStatus: true,
		},
	}

	err := actions.UpdateTaskStatus(context.Background(), runCtx, "task-2", "done", "")
	if !errors.Is(err, ErrTaskOutOfScope) || !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(updater.calls) != 0 {
		t.Fatal("status updater should not be called for an unscoped task")
	}
}

func TestCapabilitiesAllowsByKey(t *testing.T) {
	caps := Capabilities{
		CanCreateAgents:   true,
		CanListProjects:   true,
		CanCreateProjects: true,
		CanReadMemory:     true,
	}

	if !caps.Allows(CapabilityCreateAgent) {
		t.Fatal("expected create_agent to be allowed")
	}
	if !caps.Allows(CapabilityReadMemory) {
		t.Fatal("expected read_memory to be allowed")
	}
	if !caps.Allows("list_projects") {
		t.Fatal("expected list_projects to be allowed")
	}
	if !caps.Allows("create_project") {
		t.Fatal("expected create_project to be allowed")
	}
	if caps.Allows(CapabilityDeleteSkills) {
		t.Fatal("expected delete_skills to be denied")
	}
}

func TestFromAgentMapsExistingPermissions(t *testing.T) {
	ceo := &models.AgentInstance{
		Role: models.AgentRoleCEO,
	}
	worker := &models.AgentInstance{
		Role: models.AgentRoleWorker,
	}

	ceoCaps := FromAgent(ceo)
	workerCaps := FromAgent(worker)
	if !ceoCaps.Allows(CapabilityCreateAgent) {
		t.Fatal("CEO should be allowed to create agents")
	}
	if !ceoCaps.CanListProjects || !ceoCaps.CanCreateProjects {
		t.Fatal("CEO should be allowed to list and create projects")
	}
	if workerCaps.Allows(CapabilityCreateAgent) {
		t.Fatal("worker should not be allowed to create agents")
	}
	if !workerCaps.CanListProjects || workerCaps.CanCreateProjects {
		t.Fatal("worker should list projects but not create them by default")
	}
	if !ceoCaps.Allows("create_task") || !workerCaps.Allows("create_task") {
		t.Fatal("can_create_tasks should grant the create_task runtime capability")
	}

	securityCaps := FromAgent(&models.AgentInstance{Role: models.AgentRoleSecurity})
	if securityCaps.Allows("create_task") {
		t.Fatal("security should not receive create_task without an explicit permission override")
	}
}

func TestActionsCreateAgentUsesCallerAndReportsToDefault(t *testing.T) {
	creator := &recordingAgentCreator{}
	actions := NewActions(ActionDependencies{Agents: creator})
	runCtx := RunContext{
		AgentID:     "ceo-1",
		WorkspaceID: "ws-1",
		Capabilities: Capabilities{
			CanCreateAgents: true,
		},
	}
	caller := &models.AgentInstance{ID: "ceo-1", Name: "CEO"}

	agent, err := actions.CreateAgent(context.Background(), runCtx, caller, CreateAgentInput{
		Name:   "Builder",
		Role:   string(models.AgentRoleWorker),
		Reason: "need implementation help",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if agent.ReportsTo != "ceo-1" || agent.WorkspaceID != "ws-1" {
		t.Fatalf("unexpected created agent: %+v", agent)
	}
	if len(creator.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(creator.calls))
	}
	if creator.calls[0].Caller != caller || creator.calls[0].Reason != "need implementation help" {
		t.Fatalf("unexpected creator call: %+v", creator.calls[0])
	}
}

func TestActionsListProjectsUsesRunWorkspace(t *testing.T) {
	projects := &recordingProjectManager{
		projects: []*models.Project{{ID: "project-1", WorkspaceID: "ws-1", Name: "Alpha"}},
	}
	actions := NewActions(ActionDependencies{Projects: projects})
	runCtx := RunContext{
		WorkspaceID:  "ws-1",
		Capabilities: Capabilities{CanListProjects: true},
	}

	got, err := actions.ListProjects(context.Background(), runCtx)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(got) != 1 || got[0].ID != "project-1" {
		t.Fatalf("projects = %+v", got)
	}
	if len(projects.listWorkspaceIDs) != 1 || projects.listWorkspaceIDs[0] != "ws-1" {
		t.Fatalf("list workspaces = %+v, want [ws-1]", projects.listWorkspaceIDs)
	}
}

func TestActionsListProjectsRejectsMissingWorkspace(t *testing.T) {
	projects := &recordingProjectManager{}
	actions := NewActions(ActionDependencies{Projects: projects})
	runCtx := RunContext{Capabilities: Capabilities{CanListProjects: true}}

	_, err := actions.ListProjects(context.Background(), runCtx)
	if !errors.Is(err, ErrWorkspaceOutOfScope) {
		t.Fatalf("expected workspace scope error, got %v", err)
	}
	if len(projects.listWorkspaceIDs) != 0 {
		t.Fatalf("project manager called with missing workspace: %+v", projects.listWorkspaceIDs)
	}
}

func TestActionsCreateProjectRequiresCapability(t *testing.T) {
	projects := &recordingProjectManager{}
	actions := NewActions(ActionDependencies{Projects: projects})
	runCtx := RunContext{WorkspaceID: "ws-1"}

	_, err := actions.CreateProject(context.Background(), runCtx, CreateProjectInput{Name: "Alpha"})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if len(projects.created) != 0 {
		t.Fatal("project manager should not be called when capability is missing")
	}
}

func TestActionsCreateProjectForcesRunWorkspace(t *testing.T) {
	projects := &recordingProjectManager{}
	actions := NewActions(ActionDependencies{Projects: projects})
	runCtx := RunContext{
		WorkspaceID:  "ws-1",
		Capabilities: Capabilities{CanCreateProjects: true},
	}

	project, err := actions.CreateProject(context.Background(), runCtx, CreateProjectInput{
		Name:         "Alpha",
		Repositories: []string{"github.com/acme/alpha"},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.WorkspaceID != "ws-1" || len(projects.created) != 1 {
		t.Fatalf("created project = %+v, calls = %d", project, len(projects.created))
	}
	if project.Repositories != `["github.com/acme/alpha"]` {
		t.Fatalf("repositories = %q", project.Repositories)
	}
}

func TestActionsCreateProjectValidatesLeadWorkspaceBeforePersistence(t *testing.T) {
	tests := []struct {
		name   string
		agents map[string]*models.AgentInstance
	}{
		{
			name: "cross workspace",
			agents: map[string]*models.AgentInstance{
				"lead-2": {ID: "lead-2", WorkspaceID: "ws-2"},
			},
		},
		{name: "missing", agents: map[string]*models.AgentInstance{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectManager := &recordingProjectManager{}
			actions := NewActions(ActionDependencies{
				Projects:      projectManager,
				AgentModifier: &recordingAgentModifier{agents: tc.agents},
			})
			runCtx := RunContext{
				WorkspaceID:  "ws-1",
				Capabilities: Capabilities{CanCreateProjects: true},
			}

			_, err := actions.CreateProject(context.Background(), runCtx, CreateProjectInput{
				Name: "Alpha", LeadAgentProfileID: "lead-2",
			})
			if !errors.Is(err, ErrWorkspaceOutOfScope) {
				t.Fatalf("error = %v, want workspace scope denial", err)
			}
			if len(projectManager.created) != 0 {
				t.Fatalf("project manager called on denied lead: %#v", projectManager.created)
			}
		})
	}
}

func TestActionsCreateProjectAllowsEmptyAndSameWorkspaceLead(t *testing.T) {
	tests := []struct {
		name       string
		leadInput  string
		wantLeadID string
	}{
		{name: "empty"},
		{name: "same workspace ID", leadInput: "lead-1", wantLeadID: "lead-1"},
		{name: "trimmed same workspace ID", leadInput: " lead-1 ", wantLeadID: "lead-1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectManager := &recordingProjectManager{}
			actions := NewActions(ActionDependencies{
				Projects: projectManager,
				AgentModifier: &recordingAgentModifier{agents: map[string]*models.AgentInstance{
					"lead-1": {ID: "lead-1", WorkspaceID: "ws-1"},
				}},
			})
			runCtx := RunContext{
				WorkspaceID:  "ws-1",
				Capabilities: Capabilities{CanCreateProjects: true},
			}

			project, err := actions.CreateProject(context.Background(), runCtx, CreateProjectInput{
				Name: "Alpha", LeadAgentProfileID: tc.leadInput,
			})
			if err != nil {
				t.Fatalf("create project: %v", err)
			}
			if project.LeadAgentProfileID != tc.wantLeadID || len(projectManager.created) != 1 {
				t.Fatalf("project/calls = %#v/%d", project, len(projectManager.created))
			}
		})
	}
}

func TestActionsCreateProjectDeniesNonCanonicalSameWorkspaceLeadBeforePersistence(t *testing.T) {
	projectManager := &recordingProjectManager{}
	actions := NewActions(ActionDependencies{
		Projects: projectManager,
		AgentModifier: &recordingAgentModifier{agents: map[string]*models.AgentInstance{
			"Platform lead": {ID: "lead-1", WorkspaceID: "ws-1"},
		}},
	})
	runCtx := RunContext{
		WorkspaceID:  "ws-1",
		Capabilities: Capabilities{CanCreateProjects: true},
	}

	_, err := actions.CreateProject(context.Background(), runCtx, CreateProjectInput{
		Name: "Alpha", LeadAgentProfileID: "Platform lead",
	})
	if !errors.Is(err, ErrWorkspaceOutOfScope) {
		t.Fatalf("error = %v, want workspace scope denial", err)
	}
	if len(projectManager.created) != 0 {
		t.Fatalf("project manager called for non-canonical lead: %#v", projectManager.created)
	}
}

func TestActionsCreateProjectRejectsMissingWorkspace(t *testing.T) {
	projects := &recordingProjectManager{}
	actions := NewActions(ActionDependencies{Projects: projects})
	runCtx := RunContext{Capabilities: Capabilities{CanCreateProjects: true}}

	_, err := actions.CreateProject(context.Background(), runCtx, CreateProjectInput{Name: "Alpha"})
	if !errors.Is(err, ErrWorkspaceOutOfScope) {
		t.Fatalf("expected workspace scope error, got %v", err)
	}
	if len(projects.created) != 0 {
		t.Fatalf("project manager called with missing workspace: %+v", projects.created)
	}
}

func TestActionsUpdateTaskStatusPreservesRunIdentity(t *testing.T) {
	updater := &recordingStatusUpdater{}
	actions := NewActions(ActionDependencies{TaskStatus: updater})
	runCtx := RunContext{
		AgentID:   "agent-1",
		TaskID:    "task-1",
		RunID:     "run-1",
		SessionID: "session-1",
		Capabilities: Capabilities{
			CanUpdateTaskStatus: true,
		},
	}

	if err := actions.UpdateTaskStatus(context.Background(), runCtx, "task-1", "done", "complete"); err != nil {
		t.Fatalf("update status: %v", err)
	}
	if len(updater.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(updater.calls))
	}
	call := updater.calls[0]
	if call.TaskID != "task-1" || call.NewStatus != "done" || call.Comment != "complete" {
		t.Fatalf("unexpected status call: %+v", call)
	}
	if call.ActorAgentID != "agent-1" || call.RunID != "run-1" || call.SessionID != "session-1" {
		t.Fatalf("missing run identity: %+v", call)
	}
}

func TestActionsRequestApprovalCreatesRunScopedPayload(t *testing.T) {
	requester := &recordingApprovalRequester{}
	actions := NewActions(ActionDependencies{Approvals: requester})
	runCtx := RunContext{
		AgentID:     "agent-1",
		WorkspaceID: "ws-1",
		TaskID:      "task-1",
		RunID:       "run-1",
		SessionID:   "sess-1",
		Capabilities: Capabilities{
			CanRequestApproval: true,
		},
	}

	approval, err := actions.RequestApproval(context.Background(), runCtx, RequestApprovalInput{
		Type:       models.ApprovalTypeTaskReview,
		TargetType: "task",
		TargetID:   "task-1",
		Reason:     "needs human signoff",
		Payload:    map[string]interface{}{"extra": "value"},
	})
	if err != nil {
		t.Fatalf("request approval: %v", err)
	}
	if approval.WorkspaceID != "ws-1" || approval.RequestedByAgentProfileID != "agent-1" {
		t.Fatalf("unexpected approval identity: %+v", approval)
	}
	if len(requester.approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(requester.approvals))
	}
	if !strings.Contains(approval.Payload, `"run_id":"run-1"`) ||
		!strings.Contains(approval.Payload, `"reason":"needs human signoff"`) {
		t.Fatalf("payload missing run context: %s", approval.Payload)
	}
}

func TestActionsSpawnAgentRunDeniesCrossWorkspaceTarget(t *testing.T) {
	agents := &recordingAgentModifier{
		agents: map[string]*models.AgentInstance{
			"agent-2": {ID: "agent-2", WorkspaceID: "ws-2"},
		},
	}
	runs := &recordingRunSpawner{}
	actions := NewActions(ActionDependencies{Runs: runs, AgentModifier: agents})
	runCtx := RunContext{
		WorkspaceID: "ws-1",
		Capabilities: Capabilities{
			CanSpawnAgentRun: true,
		},
	}

	err := actions.SpawnAgentRun(context.Background(), runCtx, SpawnAgentRunInput{
		AgentID: "agent-2",
		Reason:  "heartbeat",
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if len(runs.calls) != 0 {
		t.Fatal("run spawner should not be called for cross-workspace target")
	}
}

func TestActionsModifyAgentUpdatesSameWorkspaceAgent(t *testing.T) {
	name := "Runtime QA"
	agents := &recordingAgentModifier{
		agents: map[string]*models.AgentInstance{
			"agent-2": {ID: "agent-2", WorkspaceID: "ws-1", Name: "Old", Role: models.AgentRoleWorker},
		},
	}
	actions := NewActions(ActionDependencies{AgentModifier: agents})
	runCtx := RunContext{
		WorkspaceID: "ws-1",
		Capabilities: Capabilities{
			CanModifyAgents: true,
		},
	}

	agent, err := actions.ModifyAgent(context.Background(), runCtx, "agent-2", ModifyAgentInput{Name: &name})
	if err != nil {
		t.Fatalf("modify agent: %v", err)
	}
	if agent.Name != "Runtime QA" || len(agents.updated) != 1 {
		t.Fatalf("agent not updated: %+v updated=%d", agent, len(agents.updated))
	}
}

func TestActionsDeleteSkillRequiresWorkspaceScope(t *testing.T) {
	skills := &recordingSkillManager{
		skills: map[string]*models.Skill{
			"skill-1": {ID: "skill-1", WorkspaceID: "ws-1"},
		},
	}
	actions := NewActions(ActionDependencies{Skills: skills})
	runCtx := RunContext{
		WorkspaceID: "ws-1",
		Capabilities: Capabilities{
			CanDeleteSkills: true,
		},
	}

	if err := actions.DeleteSkill(context.Background(), runCtx, "skill-1"); err != nil {
		t.Fatalf("delete skill: %v", err)
	}
	if len(skills.deleted) != 1 || skills.deleted[0] != "skill-1" {
		t.Fatalf("unexpected deleted skills: %+v", skills.deleted)
	}
}

type recordingCommentWriter struct {
	comments []*models.TaskComment
}

func (w *recordingCommentWriter) CreateComment(_ context.Context, comment *models.TaskComment) error {
	w.comments = append(w.comments, comment)
	return nil
}

type recordingTaskCreator struct {
	calls            []createTaskCall
	taskID           string
	taskScopes       map[string]taskScope
	workspaceLookups []string
	projectLookups   []string
}

type taskScope struct {
	WorkspaceID string
	ProjectID   string
}

func (c *recordingTaskCreator) CreateOfficeTaskAsAgent(
	_ context.Context,
	callerAgentID string,
	workspaceID string,
	projectID string,
	assigneeAgentID string,
	title string,
	description string,
) (string, error) {
	c.calls = append(c.calls, createTaskCall{
		CallerAgentID: callerAgentID, WorkspaceID: workspaceID, ProjectID: projectID,
		AssigneeAgentID: assigneeAgentID, Title: title, Description: description, Root: true,
	})
	if c.taskID != "" {
		return c.taskID, nil
	}
	return "task-id", nil
}

type createTaskCall struct {
	CallerAgentID   string
	WorkspaceID     string
	ProjectID       string
	ParentTaskID    string
	AssigneeAgentID string
	Title           string
	Description     string
	Root            bool
}

func (c *recordingTaskCreator) GetTaskWorkspaceID(_ context.Context, taskID string) (string, error) {
	c.workspaceLookups = append(c.workspaceLookups, taskID)
	return c.taskScopes[taskID].WorkspaceID, nil
}

func (c *recordingTaskCreator) GetTaskProjectID(_ context.Context, taskID string) (string, error) {
	c.projectLookups = append(c.projectLookups, taskID)
	return c.taskScopes[taskID].ProjectID, nil
}

func (c *recordingTaskCreator) CreateOfficeSubtaskAsAgent(
	_ context.Context,
	callerAgentID string,
	parentTaskID string,
	assigneeAgentID string,
	title string,
	description string,
) (string, error) {
	c.calls = append(c.calls, createTaskCall{
		CallerAgentID:   callerAgentID,
		ParentTaskID:    parentTaskID,
		AssigneeAgentID: assigneeAgentID,
		Title:           title,
		Description:     description,
	})
	if c.taskID != "" {
		return c.taskID, nil
	}
	return "task-id", nil
}

type recordingStatusUpdater struct {
	calls []TaskStatusUpdate
}

func (u *recordingStatusUpdater) UpdateTaskStatusAsAgent(
	_ context.Context,
	update TaskStatusUpdate,
) error {
	u.calls = append(u.calls, update)
	return nil
}

type recordingAgentCreator struct {
	calls []agentCreateCall
}

type recordingProjectManager struct {
	projects         []*models.Project
	listWorkspaceIDs []string
	created          []*models.Project
}

func (m *recordingProjectManager) ListProjects(
	_ context.Context,
	workspaceID string,
) ([]*models.Project, error) {
	m.listWorkspaceIDs = append(m.listWorkspaceIDs, workspaceID)
	return m.projects, nil
}

func (m *recordingProjectManager) CreateProject(_ context.Context, project *models.Project) error {
	m.created = append(m.created, project)
	project.ID = "project-created"
	return nil
}

type agentCreateCall struct {
	Agent  *models.AgentInstance
	Caller *models.AgentInstance
	Reason string
}

func (c *recordingAgentCreator) CreateAgentInstanceWithCaller(
	_ context.Context,
	agent *models.AgentInstance,
	caller *models.AgentInstance,
	reason string,
) error {
	c.calls = append(c.calls, agentCreateCall{Agent: agent, Caller: caller, Reason: reason})
	return nil
}

type recordingApprovalRequester struct {
	approvals []*models.Approval
}

func (r *recordingApprovalRequester) CreateApprovalWithActivity(
	_ context.Context,
	approval *models.Approval,
) error {
	r.approvals = append(r.approvals, approval)
	return nil
}

type recordingRunSpawner struct {
	calls []spawnRunCall
}

type spawnRunCall struct {
	AgentID        string
	Reason         string
	Payload        string
	IdempotencyKey string
}

func (r *recordingRunSpawner) QueueRun(
	_ context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
) error {
	r.calls = append(r.calls, spawnRunCall{
		AgentID:        agentInstanceID,
		Reason:         reason,
		Payload:        payload,
		IdempotencyKey: idempotencyKey,
	})
	return nil
}

type recordingAgentModifier struct {
	agents  map[string]*models.AgentInstance
	updated []*models.AgentInstance
}

func (r *recordingAgentModifier) GetAgentInstance(
	_ context.Context,
	idOrName string,
) (*models.AgentInstance, error) {
	if agent, ok := r.agents[idOrName]; ok {
		return agent, nil
	}
	return nil, errors.New("agent not found")
}

func (r *recordingAgentModifier) UpdateAgentInstance(
	_ context.Context,
	agent *models.AgentInstance,
) error {
	r.updated = append(r.updated, agent)
	return nil
}

type recordingSkillManager struct {
	skills  map[string]*models.Skill
	deleted []string
}

func (r *recordingSkillManager) GetSkill(_ context.Context, id string) (*models.Skill, error) {
	if skill, ok := r.skills[id]; ok {
		return skill, nil
	}
	return nil, errors.New("skill not found")
}

func (r *recordingSkillManager) DeleteSkill(_ context.Context, id string) error {
	r.deleted = append(r.deleted, id)
	return nil
}
