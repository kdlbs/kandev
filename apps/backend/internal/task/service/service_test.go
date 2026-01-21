package service

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/worktree"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// MockEventBus implements bus.EventBus for testing
type MockEventBus struct {
	mu              sync.Mutex
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

func createTestService(t *testing.T) (*Service, *MockEventBus, repository.Repository) {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	repoImpl, cleanup, err := repository.Provide(dbConn)
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	repo := repository.Repository(repoImpl)
	if _, err := worktree.NewSQLiteStore(dbConn); err != nil {
		t.Fatalf("failed to init worktree store: %v", err)
	}
	t.Cleanup(func() {
		if err := dbConn.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if err := cleanup(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	})
	eventBus := NewMockEventBus()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewService(repo, eventBus, log, RepositoryDiscoveryConfig{})
	return svc, eventBus, repo
}

// Task tests

func TestService_CreateTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	// Create board and column first
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do", State: v1.TaskStateTODO}
	_ = repo.CreateColumn(ctx, column)
	repository := &models.Repository{ID: "repo-123", WorkspaceID: "ws-1", Name: "Test Repo"}
	_ = repo.CreateRepository(ctx, repository)

	req := &CreateTaskRequest{
		WorkspaceID: "ws-1",
		BoardID:     "board-123",
		ColumnID:    "col-123",
		Title:       "Test Task",
		Description: "A test task",
		Priority:    5,
		Repositories: []TaskRepositoryInput{
			{
				RepositoryID: "repo-123",
				BaseBranch:   "main",
			},
		},
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
	if task.State != v1.TaskStateCreated {
		t.Errorf("expected state CREATED, got %s", task.State)
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

func TestService_CreateRepository_DefaultWorktreeBranchPrefix(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})

	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "Test Repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository failed: %v", err)
	}
	if created.WorktreeBranchPrefix != worktree.DefaultBranchPrefix {
		t.Fatalf("expected default prefix %q, got %q", worktree.DefaultBranchPrefix, created.WorktreeBranchPrefix)
	}

	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository failed: %v", err)
	}
	if stored.WorktreeBranchPrefix != worktree.DefaultBranchPrefix {
		t.Fatalf("expected stored prefix %q, got %q", worktree.DefaultBranchPrefix, stored.WorktreeBranchPrefix)
	}
}

func TestService_GetTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	// Create required entities
	setupTestTask(t, repo)

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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Board"})
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-123", BoardID: "board-123", Name: "Column", State: v1.TaskStateTODO})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-123", Title: "Original"})
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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Board"})
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-123", BoardID: "board-123", Name: "Column", State: v1.TaskStateTODO})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-123", Title: "Test"})
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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Board"})
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-123", BoardID: "board-123", Name: "Column", State: v1.TaskStateTODO})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-123", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-123", Title: "Task 2"})

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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Board"})
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-123", BoardID: "board-123", Name: "Column", State: v1.TaskStateTODO})
	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-123", Title: "Test", State: v1.TaskStateTODO}
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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Board"})

	// Create source column
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-123", BoardID: "board-123", Name: "Todo", State: v1.TaskStateTODO})
	// Create destination column with different state
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-done", BoardID: "board-123", Name: "Done", State: v1.TaskStateCompleted})

	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-123", Title: "Test", State: v1.TaskStateTODO}
	_ = repo.CreateTask(ctx, task)
	eventBus.ClearEvents()

	moved, err := svc.MoveTask(ctx, "task-123", "board-123", "col-done", 0)
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
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	req := &CreateBoardRequest{
		WorkspaceID: "ws-1",
		Name:        "Test Board",
		Description: "A test board",
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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Original"}
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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-1", WorkspaceID: "ws-1", Name: "Board 1"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-2", WorkspaceID: "ws-1", Name: "Board 2"})

	boards, err := svc.ListBoards(ctx, "ws-1")
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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Board"})
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

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Board"})
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

// Message tests

func setupTestTask(t *testing.T, repo repository.Repository) {
	t.Helper()
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Board"})
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-123", BoardID: "board-123", Name: "Column", State: v1.TaskStateTODO})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"})
}

func setupTestSession(t *testing.T, repo repository.Repository) string {
	t.Helper()
	ctx := context.Background()
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateStarting,
	}
	_ = repo.CreateTaskSession(ctx, session)
	return session.ID
}

func setupTestTurn(t *testing.T, repo repository.Repository, sessionID, taskID, turnID string) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	turn := &models.Turn{
		ID:            turnID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("failed to create test turn: %v", err)
	}
	return turn.ID
}

func TestService_CreateMessage(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	// Create a task first
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")
	eventBus.ClearEvents()

	req := &CreateMessageRequest{
		TaskSessionID: sessionID,
		TurnID:        turnID,
		Content:       "This is a test comment",
		AuthorType:    "user",
		AuthorID:      "user-123",
	}

	comment, err := svc.CreateMessage(ctx, req)
	if err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	if comment.ID == "" {
		t.Error("expected comment ID to be set")
	}
	if comment.Content != "This is a test comment" {
		t.Errorf("expected content 'This is a test comment', got %s", comment.Content)
	}
	if comment.AuthorType != models.MessageAuthorUser {
		t.Errorf("expected author type 'user', got %s", comment.AuthorType)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "message.added" {
		t.Errorf("expected event type 'message.added', got %s", events[0].Type)
	}
}

func TestService_CreateAgentMessage(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	req := &CreateMessageRequest{
		TaskSessionID: sessionID,
		TurnID:        turnID,
		Content:       "What should I do next?",
		AuthorType:    "agent",
		AuthorID:      "agent-123",
		RequestsInput: true,
	}

	comment, err := svc.CreateMessage(ctx, req)
	if err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	if comment.AuthorType != models.MessageAuthorAgent {
		t.Errorf("expected author type 'agent', got %s", comment.AuthorType)
	}
	if !comment.RequestsInput {
		t.Error("expected RequestsInput to be true")
	}
}

func TestService_CreateMessageSessionNotFound(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	req := &CreateMessageRequest{
		TaskSessionID: "nonexistent",
		Content:       "Test comment",
	}

	_, err := svc.CreateMessage(ctx, req)
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestService_GetMessage(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	comment := &models.Message{ID: "comment-123", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Test"}
	_ = repo.CreateMessage(ctx, comment)

	retrieved, err := svc.GetMessage(ctx, "comment-123")
	if err != nil {
		t.Fatalf("failed to get comment: %v", err)
	}
	if retrieved.Content != "Test" {
		t.Errorf("expected content 'Test', got %s", retrieved.Content)
	}
}

func TestService_ListMessages(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-1", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Comment 1"})
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-2", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorAgent, Content: "Comment 2"})

	comments, err := svc.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}
}

func TestService_DeleteMessage(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	comment := &models.Message{ID: "comment-123", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Test"}
	_ = repo.CreateMessage(ctx, comment)

	err := svc.DeleteMessage(ctx, "comment-123")
	if err != nil {
		t.Fatalf("failed to delete comment: %v", err)
	}

	_, err = svc.GetMessage(ctx, "comment-123")
	if err == nil {
		t.Error("expected comment to be deleted")
	}
}
