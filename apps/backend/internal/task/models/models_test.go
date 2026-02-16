package models

import (
	"testing"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestTaskStateConstants(t *testing.T) {
	tests := []struct {
		name     string
		state    v1.TaskState
		expected string
	}{
		{"CREATED state", v1.TaskStateCreated, "CREATED"},
		{"SCHEDULING state", v1.TaskStateScheduling, "SCHEDULING"},
		{"TODO state", v1.TaskStateTODO, "TODO"},
		{"IN_PROGRESS state", v1.TaskStateInProgress, "IN_PROGRESS"},
		{"REVIEW state", v1.TaskStateReview, "REVIEW"},
		{"BLOCKED state", v1.TaskStateBlocked, "BLOCKED"},
		{"WAITING_FOR_INPUT state", v1.TaskStateWaitingForInput, "WAITING_FOR_INPUT"},
		{"COMPLETED state", v1.TaskStateCompleted, "COMPLETED"},
		{"FAILED state", v1.TaskStateFailed, "FAILED"},
		{"CANCELLED state", v1.TaskStateCancelled, "CANCELLED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.state))
			}
		})
	}
}

func TestTaskStructInitialization(t *testing.T) {
	now := time.Now().UTC()
	repos := []*TaskRepository{
		{
			ID:           "task-repo-1",
			TaskID:       "task-123",
			RepositoryID: "repo-123",
			BaseBranch:   "main",
			Position:     0,
			Metadata:     map[string]interface{}{"role": "primary"},
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
	task := Task{
		ID:             "task-123",
		WorkspaceID:    "workspace-001",
		WorkflowID:     "workflow-456",
		WorkflowStepID: "workflow-step-789",
		Title:          "Test Task",
		Description:    "A test task description",
		State:          v1.TaskStateTODO,
		Priority:       5,
		Position:       1,
		Metadata:       map[string]interface{}{"key": "value"},
		Repositories:   repos,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if task.ID != "task-123" {
		t.Errorf("expected ID task-123, got %s", task.ID)
	}
	if task.WorkspaceID != "workspace-001" {
		t.Errorf("expected WorkspaceID workspace-001, got %s", task.WorkspaceID)
	}
	if task.WorkflowID != "workflow-456" {
		t.Errorf("expected WorkflowID workflow-456, got %s", task.WorkflowID)
	}
	if task.WorkflowStepID != "workflow-step-789" {
		t.Errorf("expected WorkflowStepID workflow-step-789, got %s", task.WorkflowStepID)
	}
	if task.Title != "Test Task" {
		t.Errorf("expected Title 'Test Task', got %s", task.Title)
	}
	if task.Description != "A test task description" {
		t.Errorf("expected Description 'A test task description', got %s", task.Description)
	}
	if task.State != v1.TaskStateTODO {
		t.Errorf("expected State TODO, got %s", task.State)
	}
	if task.Priority != 5 {
		t.Errorf("expected Priority 5, got %d", task.Priority)
	}
	if len(task.Repositories) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(task.Repositories))
	}
	if task.Repositories[0].RepositoryID != "repo-123" {
		t.Errorf("expected RepositoryID 'repo-123', got %s", task.Repositories[0].RepositoryID)
	}
	if task.Repositories[0].BaseBranch != "main" {
		t.Errorf("expected BaseBranch 'main', got %s", task.Repositories[0].BaseBranch)
	}
	if task.Position != 1 {
		t.Errorf("expected Position 1, got %d", task.Position)
	}
	if task.Metadata["key"] != "value" {
		t.Errorf("expected Metadata key 'value', got %v", task.Metadata["key"])
	}
}

func TestWorkflowStructInitialization(t *testing.T) {
	now := time.Now().UTC()
	wf := Workflow{
		ID:          "workflow-123",
		WorkspaceID: "workspace-001",
		Name:        "Test Workflow",
		Description: "A test workflow",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if wf.ID != "workflow-123" {
		t.Errorf("expected ID workflow-123, got %s", wf.ID)
	}
	if wf.WorkspaceID != "workspace-001" {
		t.Errorf("expected WorkspaceID workspace-001, got %s", wf.WorkspaceID)
	}
	if wf.Name != "Test Workflow" {
		t.Errorf("expected Name 'Test Workflow', got %s", wf.Name)
	}
	if wf.Description != "A test workflow" {
		t.Errorf("expected Description 'A test workflow', got %s", wf.Description)
	}
}

func TestTaskToAPI(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{
		ID:             "task-123",
		WorkspaceID:    "workspace-001",
		WorkflowID:     "workflow-456",
		WorkflowStepID: "step-789",
		Title:       "Test Task",
		Description: "A test task description",
		State:       v1.TaskStateInProgress,
		Priority:    3,
		Repositories: []*TaskRepository{
			{
				ID:           "task-repo-1",
				TaskID:       "task-123",
				RepositoryID: "repo-123",
				BaseBranch:   "main",
				Position:     0,
				Metadata:     map[string]interface{}{"role": "primary"},
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		Position:  2,
		Metadata:  map[string]interface{}{"key": "value"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	apiTask := task.ToAPI()

	if apiTask.ID != task.ID {
		t.Errorf("expected ID %s, got %s", task.ID, apiTask.ID)
	}
	if apiTask.WorkspaceID != task.WorkspaceID {
		t.Errorf("expected WorkspaceID %s, got %s", task.WorkspaceID, apiTask.WorkspaceID)
	}
	if apiTask.WorkflowID != task.WorkflowID {
		t.Errorf("expected WorkflowID %s, got %s", task.WorkflowID, apiTask.WorkflowID)
	}
	if apiTask.Title != task.Title {
		t.Errorf("expected Title %s, got %s", task.Title, apiTask.Title)
	}
	if apiTask.Description != task.Description {
		t.Errorf("expected Description %s, got %s", task.Description, apiTask.Description)
	}
	if apiTask.State != task.State {
		t.Errorf("expected State %s, got %s", task.State, apiTask.State)
	}
	if apiTask.Priority != task.Priority {
		t.Errorf("expected Priority %d, got %d", task.Priority, apiTask.Priority)
	}
	if len(apiTask.Repositories) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(apiTask.Repositories))
	}
	if apiTask.Repositories[0].RepositoryID != "repo-123" {
		t.Errorf("expected RepositoryID repo-123, got %s", apiTask.Repositories[0].RepositoryID)
	}
	if apiTask.Repositories[0].BaseBranch != "main" {
		t.Errorf("expected BaseBranch main, got %s", apiTask.Repositories[0].BaseBranch)
	}
	if apiTask.Metadata["key"] != "value" {
		t.Errorf("expected Metadata key 'value', got %v", apiTask.Metadata["key"])
	}
}

func TestTaskToAPIWithEmptyOptionalFields(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{
		ID:             "task-123",
		WorkspaceID:    "workspace-001",
		WorkflowID:     "workflow-456",
		WorkflowStepID: "step-789",
		Title:       "Test Task",
		Description: "A test task description",
		State:       v1.TaskStateTODO,
		Priority:    0,
		Position:    0,
		Metadata:    nil,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	apiTask := task.ToAPI()

	if len(apiTask.Repositories) != 0 {
		t.Errorf("expected no repositories, got %d", len(apiTask.Repositories))
	}
}
