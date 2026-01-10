package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// MockEventBus implements bus.EventBus for testing
type MockEventBus struct {
	mu             sync.Mutex
	publishedEvents []*bus.Event
	closed          bool
}

func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		publishedEvents: make([]*bus.Event, 0),
	}
}

func (m *MockEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedEvents = append(m.publishedEvents, event)
	return nil
}

func (m *MockEventBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *MockEventBus) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

func (m *MockEventBus) IsConnected() bool {
	return !m.closed
}

func (m *MockEventBus) GetPublishedEvents() []*bus.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishedEvents
}

func (m *MockEventBus) ClearEvents() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedEvents = make([]*bus.Event, 0)
}

func createTestService(t *testing.T) (*Service, *MockEventBus, *repository.MemoryRepository) {
	t.Helper()
	repo := repository.NewMemoryRepository()
	eventBus := NewMockEventBus()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewService(repo, eventBus, log)
	return svc, eventBus, repo
}

// Task tests

func TestService_CreateTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	// Create board and column first
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do", State: v1.TaskStateTODO}
	_ = repo.CreateColumn(ctx, column)

	req := &CreateTaskRequest{
		BoardID:     "board-123",
		ColumnID:    "col-123",
		Title:       "Test Task",
		Description: "A test task",
		Priority:    5,
	}

	task, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	if task.ID == "" {
		t.Error("expected task ID to be set")
	}
	if task.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", task.Title)
	}
	if task.State != v1.TaskStateTODO {
		t.Errorf("expected state TODO, got %s", task.State)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.created" {
		t.Errorf("expected event type 'task.created', got %s", events[0].Type)
	}
}

func TestService_GetTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	// Create a task directly in repo
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	retrieved, err := svc.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if retrieved.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", retrieved.Title)
	}
}

func TestService_GetTaskNotFound(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	_, err := svc.GetTask(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestService_UpdateTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Original"}
	_ = repo.CreateTask(ctx, task)
	eventBus.ClearEvents()

	newTitle := "Updated Title"
	req := &UpdateTaskRequest{Title: &newTitle}

	updated, err := svc.UpdateTask(ctx, "task-123", req)
	if err != nil {
		t.Fatalf("failed to update task: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %s", updated.Title)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.updated" {
		t.Errorf("expected event type 'task.updated', got %s", events[0].Type)
	}
}

func TestService_DeleteTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test"}
	_ = repo.CreateTask(ctx, task)
	eventBus.ClearEvents()

	err := svc.DeleteTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to delete task: %v", err)
	}

	// Verify task is deleted
	_, err = svc.GetTask(ctx, "task-123")
	if err == nil {
		t.Error("expected task to be deleted")
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestService_ListTasks(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", BoardID: "board-123", ColumnID: "col-123", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", BoardID: "board-123", ColumnID: "col-123", Title: "Task 2"})

	tasks, err := svc.ListTasks(ctx, "board-123")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestService_UpdateTaskState(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test", State: v1.TaskStateTODO}
	_ = repo.CreateTask(ctx, task)
	eventBus.ClearEvents()

	updated, err := svc.UpdateTaskState(ctx, "task-123", v1.TaskStateInProgress)
	if err != nil {
		t.Fatalf("failed to update task state: %v", err)
	}
	if updated.State != v1.TaskStateInProgress {
		t.Errorf("expected state IN_PROGRESS, got %s", updated.State)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.state_changed" {
		t.Errorf("expected event type 'task.state_changed', got %s", events[0].Type)
	}
}

func TestService_MoveTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	// Create column with different state
	column := &models.Column{ID: "col-done", BoardID: "board-123", Name: "Done", State: v1.TaskStateCompleted}
	_ = repo.CreateColumn(ctx, column)

	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test", State: v1.TaskStateTODO}
	_ = repo.CreateTask(ctx, task)
	eventBus.ClearEvents()

	moved, err := svc.MoveTask(ctx, "task-123", "col-done", 0)
	if err != nil {
		t.Fatalf("failed to move task: %v", err)
	}
	if moved.ColumnID != "col-done" {
		t.Errorf("expected column 'col-done', got %s", moved.ColumnID)
	}
	if moved.State != v1.TaskStateCompleted {
		t.Errorf("expected state COMPLETED, got %s", moved.State)
	}
}

// Board tests

func TestService_CreateBoard(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	req := &CreateBoardRequest{
		Name:        "Test Board",
		Description: "A test board",
		OwnerID:     "owner-123",
	}

	board, err := svc.CreateBoard(ctx, req)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}
	if board.ID == "" {
		t.Error("expected board ID to be set")
	}
	if board.Name != "Test Board" {
		t.Errorf("expected name 'Test Board', got %s", board.Name)
	}
}

func TestService_GetBoard(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	retrieved, err := svc.GetBoard(ctx, "board-123")
	if err != nil {
		t.Fatalf("failed to get board: %v", err)
	}
	if retrieved.Name != "Test Board" {
		t.Errorf("expected name 'Test Board', got %s", retrieved.Name)
	}
}

func TestService_UpdateBoard(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	board := &models.Board{ID: "board-123", Name: "Original"}
	_ = repo.CreateBoard(ctx, board)

	newName := "Updated"
	req := &UpdateBoardRequest{Name: &newName}

	updated, err := svc.UpdateBoard(ctx, "board-123", req)
	if err != nil {
		t.Fatalf("failed to update board: %v", err)
	}
	if updated.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %s", updated.Name)
	}
}


func TestService_DeleteBoard(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	err := svc.DeleteBoard(ctx, "board-123")
	if err != nil {
		t.Fatalf("failed to delete board: %v", err)
	}

	_, err = svc.GetBoard(ctx, "board-123")
	if err == nil {
		t.Error("expected board to be deleted")
	}
}

func TestService_ListBoards(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-1", Name: "Board 1"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-2", Name: "Board 2"})

	boards, err := svc.ListBoards(ctx)
	if err != nil {
		t.Fatalf("failed to list boards: %v", err)
	}
	if len(boards) != 2 {
		t.Errorf("expected 2 boards, got %d", len(boards))
	}
}

// Column tests

func TestService_CreateColumn(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	req := &CreateColumnRequest{
		BoardID:  "board-123",
		Name:     "To Do",
		Position: 0,
		State:    v1.TaskStateTODO,
	}

	column, err := svc.CreateColumn(ctx, req)
	if err != nil {
		t.Fatalf("failed to create column: %v", err)
	}
	if column.ID == "" {
		t.Error("expected column ID to be set")
	}
	if column.Name != "To Do" {
		t.Errorf("expected name 'To Do', got %s", column.Name)
	}
}

func TestService_GetColumn(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do"}
	_ = repo.CreateColumn(ctx, column)

	retrieved, err := svc.GetColumn(ctx, "col-123")
	if err != nil {
		t.Fatalf("failed to get column: %v", err)
	}
	if retrieved.Name != "To Do" {
		t.Errorf("expected name 'To Do', got %s", retrieved.Name)
	}
}

func TestService_ListColumns(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-1", BoardID: "board-123", Name: "To Do"})
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-2", BoardID: "board-123", Name: "Done"})

	columns, err := svc.ListColumns(ctx, "board-123")
	if err != nil {
		t.Fatalf("failed to list columns: %v", err)
	}
	if len(columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(columns))
	}
}

