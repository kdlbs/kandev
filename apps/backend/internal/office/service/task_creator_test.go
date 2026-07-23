package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
)

// mockTaskCreator records CreateOfficeTask calls and returns a configurable error.
type mockTaskCreator struct {
	calls      []createTaskCall
	agentCalls []createTaskCall
	taskID     string
	err        error
}

func (m *mockTaskCreator) CreateOfficeTaskAsAgent(
	_ context.Context, workspaceID, projectID, assigneeAgentID, title, description string,
) (string, error) {
	m.agentCalls = append(m.agentCalls, createTaskCall{
		WorkspaceID:     workspaceID,
		ProjectID:       projectID,
		AssigneeAgentID: assigneeAgentID,
		Title:           title,
		Description:     description,
	})
	id := m.taskID
	if id == "" {
		id = "new-task-id"
	}
	return id, m.err
}

type createTaskCall struct {
	WorkspaceID     string
	ProjectID       string
	AssigneeAgentID string
	Title           string
	Description     string
}

func (m *mockTaskCreator) CreateOfficeTask(
	_ context.Context, workspaceID, projectID, assigneeAgentID, title, description string,
) (string, error) {
	m.calls = append(m.calls, createTaskCall{
		WorkspaceID:     workspaceID,
		ProjectID:       projectID,
		AssigneeAgentID: assigneeAgentID,
		Title:           title,
		Description:     description,
	})
	id := m.taskID
	if id == "" {
		id = "new-task-id"
	}
	return id, m.err
}

func TestCreateOfficeTaskAsAgent_WorkerCanCreate(t *testing.T) {
	mock := &mockTaskCreator{}
	svc := newTestService(t, service.ServiceOptions{TaskCreator: mock})
	ctx := context.Background()

	// Worker has can_create_tasks: true by default.
	caller := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "creator-worker",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, caller); err != nil {
		t.Fatalf("create caller: %v", err)
	}

	taskID, err := svc.CreateOfficeTaskAsAgent(
		ctx, caller.ID, "ws-1", "project-1", "agent-assignee", "Build widget", "A description",
	)
	if err != nil {
		t.Fatalf("CreateOfficeTaskAsAgent failed for worker: %v", err)
	}
	if taskID == "" {
		t.Error("expected non-empty task ID")
	}
	if len(mock.agentCalls) != 1 {
		t.Fatalf("expected 1 CreateOfficeTaskAsAgent call, got %d", len(mock.agentCalls))
	}
	if len(mock.calls) != 0 {
		t.Fatalf("CreateOfficeTask was called %d times; agent creation must use the agent-specific path", len(mock.calls))
	}
	if mock.agentCalls[0].Title != "Build widget" {
		t.Errorf("title = %q, want %q", mock.agentCalls[0].Title, "Build widget")
	}
	if mock.agentCalls[0].ProjectID != "project-1" || mock.agentCalls[0].AssigneeAgentID != "agent-assignee" {
		t.Errorf("project/assignee = %q/%q, want project-1/agent-assignee",
			mock.agentCalls[0].ProjectID, mock.agentCalls[0].AssigneeAgentID)
	}
}

func TestCreateOfficeTaskAsAgent_NilCreatorReturnsError(t *testing.T) {
	svc := newTestService(t) // no TaskCreator
	ctx := context.Background()

	// Empty callerAgentID bypasses permission check; the nil creator path is reached.
	_, err := svc.CreateOfficeTaskAsAgent(ctx, "", "ws-1", "", "", "Task", "")
	if err == nil {
		t.Fatal("expected error when task creator is nil")
	}
}

func TestCreateOfficeTaskAsAgent_EmptyCallerSkipsCheck(t *testing.T) {
	mock := &mockTaskCreator{}
	svc := newTestService(t, service.ServiceOptions{TaskCreator: mock})
	ctx := context.Background()

	// Empty callerAgentID = internal caller; no permission check performed.
	_, err := svc.CreateOfficeTaskAsAgent(ctx, "", "ws-1", "", "", "Internal task", "")
	if err != nil {
		t.Fatalf("expected success with empty caller: %v", err)
	}
	if len(mock.agentCalls) != 1 {
		t.Fatalf("expected 1 agent call, got %d", len(mock.agentCalls))
	}
}

func TestCreateOfficeTaskAsAgent_ForbiddenWhenPermissionMissing(t *testing.T) {
	mock := &mockTaskCreator{}
	svc := newTestService(t, service.ServiceOptions{TaskCreator: mock})
	ctx := context.Background()

	// Create a specialist with can_create_tasks explicitly revoked.
	caller := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "no-create-specialist",
		Role:        models.AgentRoleSpecialist,
		Permissions: `{"can_create_tasks": false}`,
	}
	if err := svc.CreateAgentInstance(ctx, caller); err != nil {
		t.Fatalf("create caller: %v", err)
	}

	_, err := svc.CreateOfficeTaskAsAgent(ctx, caller.ID, "ws-1", "", "", "Blocked task", "")
	if err == nil {
		t.Fatal("expected ErrForbidden, got nil")
	}
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got: %v", err)
	}
	if len(mock.calls) != 0 || len(mock.agentCalls) != 0 {
		t.Error("task creator should not be called when forbidden")
	}
}
