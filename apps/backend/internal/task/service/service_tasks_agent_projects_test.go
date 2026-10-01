package service

import (
	"context"
	"testing"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func TestGenericTaskCreateRejectsAgentProjectIdentity(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO agent_projects (id, workspace_id, name)
		VALUES (?, ?, ?)
	`, "project-1", "ws-1", "Project"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	workspace, err := repo.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}

	_, err = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     workspace.OfficeWorkflowID,
		AgentProjectID: "project-1",
		Title:          "Forged project task",
	})
	if err == nil {
		t.Fatal("generic task creation accepted AgentProjectID")
	}
	var count int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE agent_project_id = ?`, "project-1").Scan(&count); err != nil {
		t.Fatalf("count project tasks: %v", err)
	}
	if count != 0 {
		t.Fatalf("created %d project tasks from generic request", count)
	}
}

func TestGenericChildAndCoordinatorLifecycleRequireProjectBoundary(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()
	if _, err := repo.DB().ExecContext(ctx, `INSERT INTO agent_projects (id, workspace_id, name) VALUES (?, ?, ?)`, "project-boundary", "ws-1", "Boundary"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	coordinator := &models.Task{
		ID: "task-project-boundary", WorkspaceID: "ws-1", Title: "Coordinator",
		Origin: models.TaskOriginAgentProject, AgentProjectID: "project-boundary",
		AgentProjectTier: "coordinator", AgentProjectProfileID: "profile-coordinator",
	}
	if err := repo.CreateTask(ctx, coordinator); err != nil {
		t.Fatalf("CreateTask(coordinator): %v", err)
	}
	if _, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", ParentID: coordinator.ID, Title: "Forged child",
	}); err != ErrAgentProjectTaskForbidden {
		t.Fatalf("generic child creation error = %v, want project boundary rejection", err)
	}
	if err := svc.ArchiveTask(ctx, coordinator.ID); err != ErrAgentProjectCoordinatorActionRequired {
		t.Fatalf("generic coordinator archive error = %v, want project boundary rejection", err)
	}
	if err := svc.DeleteTask(ctx, coordinator.ID); err != ErrAgentProjectCoordinatorActionRequired {
		t.Fatalf("generic coordinator delete error = %v, want project boundary rejection", err)
	}
	if _, err := svc.MoveTask(ctx, coordinator.ID, "wf", "step", 0); err != ErrAgentProjectWorkflowOperation {
		t.Fatalf("generic coordinator workflow move error = %v, want project boundary rejection", err)
	}
	log, err := commonlogger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	handoff := NewHandoffService(repo, repo, NewDocumentService(repo, log), nil, nil, nil)
	if _, err := handoff.ArchiveTaskTree(ctx, coordinator.ID, true); err != ErrAgentProjectCoordinatorActionRequired {
		t.Fatalf("generic Handoff archive error = %v, want project boundary rejection", err)
	}
}

func TestWorkflowListsExcludeAgentProjectTasks(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()
	workspace, err := repo.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO agent_projects (id, workspace_id, name)
		VALUES (?, ?, ?)
	`, "project-list", "ws-1", "Project list"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-project-list", WorkspaceID: "ws-1", Title: "Project task",
		AgentProjectID: "project-list", Origin: models.TaskOriginAgentProject,
	}); err != nil {
		t.Fatalf("create project task: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-workflow-list", WorkspaceID: "ws-1", WorkflowID: workspace.OfficeWorkflowID,
		Title: "Workflow task",
	}); err != nil {
		t.Fatalf("create workflow task: %v", err)
	}

	listed, total, err := svc.ListTasksByWorkspace(ctx, "ws-1", workspace.OfficeWorkflowID, "", "", 1, 20, "updated", false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace: %v", err)
	}
	if total != 1 || len(listed) != 1 || listed[0].ID != "task-workflow-list" {
		t.Fatalf("workflow list = %#v (total %d), want only the workflow task", listed, total)
	}
}

func TestTypedAgentProjectTaskCreationIsWorkflowFreeAndFlagGated(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO agent_projects (id, workspace_id, name)
		VALUES (?, ?, ?)
	`, "project-typed", "ws-1", "Typed project"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	request := &CreateTaskRequest{
		WorkspaceID:            "ws-1",
		AgentProjectID:         "project-typed",
		AgentProjectTier:       "coordinator",
		AgentProjectProfileID:  "profile-coordinator",
		AssigneeAgentProfileID: "profile-coordinator",
		Title:                  "Coordinator",
	}
	if _, err := svc.CreateAgentProjectTask(ctx, request); err != ErrAgentProjectsDisabled {
		t.Fatalf("disabled CreateAgentProjectTask error = %v, want %v", err, ErrAgentProjectsDisabled)
	}
	var count int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE agent_project_id = ?`, "project-typed").Scan(&count); err != nil {
		t.Fatalf("count project tasks after disabled create: %v", err)
	}
	if count != 0 {
		t.Fatalf("disabled create wrote %d project tasks", count)
	}

	svc.SetAgentProjectsEnabled(true)
	svc.SetAgentProjectTaskAuthorizer(func(_ context.Context, req *CreateTaskRequest) error {
		if req.AgentProjectID != "project-typed" || req.WorkspaceID != "ws-1" {
			t.Fatalf("authorizer saw unexpected request: %#v", req)
		}
		return nil
	})
	result, err := svc.CreateAgentProjectTask(ctx, request)
	if err != nil {
		t.Fatalf("CreateAgentProjectTask: %v", err)
	}
	if result.Task == nil || result.Task.AgentProjectID != "project-typed" || result.Task.WorkflowID != "" || result.Task.WorkflowStepID != "" || result.Task.ProjectID != "" {
		t.Fatalf("created task = %#v, want workflow-free project task", result.Task)
	}
}
