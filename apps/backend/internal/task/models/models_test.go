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
		{"TODO state", v1.TaskStateTODO, "TODO"},
		{"IN_PROGRESS state", v1.TaskStateInProgress, "IN_PROGRESS"},
		{"REVIEW state", v1.TaskStateReview, "REVIEW"},
		{"BLOCKED state", v1.TaskStateBlocked, "BLOCKED"},
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
	task := Task{
		ID:          "task-123",
		WorkspaceID: "workspace-001",
		BoardID:     "board-456",
		ColumnID:    "column-789",
		Title:       "Test Task",
		Description: "A test task description",
		State:       v1.TaskStateTODO,
		Priority:    5,
		AgentType:   "coding",
		AssignedTo:  "agent-001",
		Position:    1,
		Metadata:    map[string]interface{}{"key": "value"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if task.ID != "task-123" {
		t.Errorf("expected ID task-123, got %s", task.ID)
	}
	if task.WorkspaceID != "workspace-001" {
		t.Errorf("expected WorkspaceID workspace-001, got %s", task.WorkspaceID)
	}
	if task.BoardID != "board-456" {
		t.Errorf("expected BoardID board-456, got %s", task.BoardID)
	}
	if task.ColumnID != "column-789" {
		t.Errorf("expected ColumnID column-789, got %s", task.ColumnID)
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
	if task.AgentType != "coding" {
		t.Errorf("expected AgentType 'coding', got %s", task.AgentType)
	}
	if task.AssignedTo != "agent-001" {
		t.Errorf("expected AssignedTo 'agent-001', got %s", task.AssignedTo)
	}
	if task.Position != 1 {
		t.Errorf("expected Position 1, got %d", task.Position)
	}
	if task.Metadata["key"] != "value" {
		t.Errorf("expected Metadata key 'value', got %v", task.Metadata["key"])
	}
}

func TestBoardStructInitialization(t *testing.T) {
	now := time.Now().UTC()
	board := Board{
		ID:          "board-123",
		WorkspaceID: "workspace-001",
		Name:        "Test Board",
		Description: "A test board",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if board.ID != "board-123" {
		t.Errorf("expected ID board-123, got %s", board.ID)
	}
	if board.WorkspaceID != "workspace-001" {
		t.Errorf("expected WorkspaceID workspace-001, got %s", board.WorkspaceID)
	}
	if board.Name != "Test Board" {
		t.Errorf("expected Name 'Test Board', got %s", board.Name)
	}
	if board.Description != "A test board" {
		t.Errorf("expected Description 'A test board', got %s", board.Description)
	}
}

func TestColumnStructInitialization(t *testing.T) {
	now := time.Now().UTC()
	column := Column{
		ID:        "column-123",
		BoardID:   "board-456",
		Name:      "To Do",
		Position:  0,
		State:     v1.TaskStateTODO,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if column.ID != "column-123" {
		t.Errorf("expected ID column-123, got %s", column.ID)
	}
	if column.BoardID != "board-456" {
		t.Errorf("expected BoardID board-456, got %s", column.BoardID)
	}
	if column.Name != "To Do" {
		t.Errorf("expected Name 'To Do', got %s", column.Name)
	}
	if column.Position != 0 {
		t.Errorf("expected Position 0, got %d", column.Position)
	}
	if column.State != v1.TaskStateTODO {
		t.Errorf("expected State TODO, got %s", column.State)
	}
}

func TestTaskToAPI(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{
		ID:          "task-123",
		WorkspaceID: "workspace-001",
		BoardID:     "board-456",
		ColumnID:    "column-789",
		Title:       "Test Task",
		Description: "A test task description",
		State:       v1.TaskStateInProgress,
		Priority:    3,
		AgentType:   "coding",
		AssignedTo:  "agent-001",
		Position:    2,
		Metadata:    map[string]interface{}{"key": "value"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	apiTask := task.ToAPI()

	if apiTask.ID != task.ID {
		t.Errorf("expected ID %s, got %s", task.ID, apiTask.ID)
	}
	if apiTask.WorkspaceID != task.WorkspaceID {
		t.Errorf("expected WorkspaceID %s, got %s", task.WorkspaceID, apiTask.WorkspaceID)
	}
	if apiTask.BoardID != task.BoardID {
		t.Errorf("expected BoardID %s, got %s", task.BoardID, apiTask.BoardID)
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
	if apiTask.AgentType == nil || *apiTask.AgentType != task.AgentType {
		t.Errorf("expected AgentType %s, got %v", task.AgentType, apiTask.AgentType)
	}
	if apiTask.AssignedAgentID == nil || *apiTask.AssignedAgentID != task.AssignedTo {
		t.Errorf("expected AssignedAgentID %s, got %v", task.AssignedTo, apiTask.AssignedAgentID)
	}
	if apiTask.Metadata["key"] != "value" {
		t.Errorf("expected Metadata key 'value', got %v", apiTask.Metadata["key"])
	}
}

func TestTaskToAPIWithEmptyOptionalFields(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{
		ID:          "task-123",
		WorkspaceID: "workspace-001",
		BoardID:     "board-456",
		ColumnID:    "column-789",
		Title:       "Test Task",
		Description: "A test task description",
		State:       v1.TaskStateTODO,
		Priority:    0,
		AgentType:   "",
		AssignedTo:  "",
		Position:    0,
		Metadata:    nil,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	apiTask := task.ToAPI()

	if apiTask.AgentType != nil {
		t.Errorf("expected AgentType nil, got %v", apiTask.AgentType)
	}
	if apiTask.AssignedAgentID != nil {
		t.Errorf("expected AssignedAgentID nil, got %v", apiTask.AssignedAgentID)
	}
}
