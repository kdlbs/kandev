package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestNewMemoryRepository(t *testing.T) {
	repo := NewMemoryRepository()
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
	if repo.tasks == nil {
		t.Error("expected tasks map to be initialized")
	}
	if repo.workspaces == nil {
		t.Error("expected workspaces map to be initialized")
	}
	if repo.boards == nil {
		t.Error("expected boards map to be initialized")
	}
	if repo.columns == nil {
		t.Error("expected columns map to be initialized")
	}
}

func TestMemoryRepository_Close(t *testing.T) {
	repo := NewMemoryRepository()
	err := repo.Close()
	if err != nil {
		t.Errorf("expected no error on close, got %v", err)
	}
}

// Board CRUD tests

func TestMemoryRepository_BoardCRUD(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	workspace := &models.Workspace{ID: "ws-1", Name: "Workspace"}
	if err := repo.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Create
	board := &models.Board{WorkspaceID: workspace.ID, Name: "Test Board", Description: "A test board"}
	if err := repo.CreateBoard(ctx, board); err != nil {
		t.Fatalf("failed to create board: %v", err)
	}
	if board.ID == "" {
		t.Error("expected board ID to be set")
	}
	if board.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Get
	retrieved, err := repo.GetBoard(ctx, board.ID)
	if err != nil {
		t.Fatalf("failed to get board: %v", err)
	}
	if retrieved.Name != "Test Board" {
		t.Errorf("expected name 'Test Board', got %s", retrieved.Name)
	}

	// Update
	board.Name = "Updated Name"
	if err := repo.UpdateBoard(ctx, board); err != nil {
		t.Fatalf("failed to update board: %v", err)
	}
	retrieved, _ = repo.GetBoard(ctx, board.ID)
	if retrieved.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %s", retrieved.Name)
	}

	// Delete
	if err := repo.DeleteBoard(ctx, board.ID); err != nil {
		t.Fatalf("failed to delete board: %v", err)
	}
	_, err = repo.GetBoard(ctx, board.ID)
	if err == nil {
		t.Error("expected board to be deleted")
	}
}

func TestMemoryRepository_BoardNotFound(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	_, err := repo.GetBoard(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent board")
	}

	err = repo.UpdateBoard(ctx, &models.Board{ID: "nonexistent", Name: "Test"})
	if err == nil {
		t.Error("expected error for updating nonexistent board")
	}

	err = repo.DeleteBoard(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent board")
	}
}

func TestMemoryRepository_ListBoards(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"})

	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-1", WorkspaceID: "ws-1", Name: "Board 1"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-2", WorkspaceID: "ws-1", Name: "Board 2"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-3", WorkspaceID: "ws-2", Name: "Board 3"})

	boards, err := repo.ListBoards(ctx, "ws-1")
	if err != nil {
		t.Fatalf("failed to list boards: %v", err)
	}
	if len(boards) != 2 {
		t.Errorf("expected 2 boards, got %d", len(boards))
	}
}

// Column CRUD tests

func TestMemoryRepository_ColumnCRUD(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	// Create
	column := &models.Column{BoardID: "board-123", Name: "To Do", Position: 0, State: v1.TaskStateTODO}
	if err := repo.CreateColumn(ctx, column); err != nil {
		t.Fatalf("failed to create column: %v", err)
	}
	if column.ID == "" {
		t.Error("expected column ID to be set")
	}

	// Get
	retrieved, err := repo.GetColumn(ctx, column.ID)
	if err != nil {
		t.Fatalf("failed to get column: %v", err)
	}
	if retrieved.Name != "To Do" {
		t.Errorf("expected name 'To Do', got %s", retrieved.Name)
	}

	// Update
	column.Name = "Done"
	column.State = v1.TaskStateCompleted
	if err := repo.UpdateColumn(ctx, column); err != nil {
		t.Fatalf("failed to update column: %v", err)
	}
	retrieved, _ = repo.GetColumn(ctx, column.ID)
	if retrieved.Name != "Done" {
		t.Errorf("expected name 'Done', got %s", retrieved.Name)
	}

	// Delete
	if err := repo.DeleteColumn(ctx, column.ID); err != nil {
		t.Fatalf("failed to delete column: %v", err)
	}
	_, err = repo.GetColumn(ctx, column.ID)
	if err == nil {
		t.Error("expected column to be deleted")
	}
}

func TestMemoryRepository_ColumnNotFound(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	_, err := repo.GetColumn(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent column")
	}

	err = repo.UpdateColumn(ctx, &models.Column{ID: "nonexistent", Name: "Test"})
	if err == nil {
		t.Error("expected error for updating nonexistent column")
	}

	err = repo.DeleteColumn(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent column")
	}
}

func TestMemoryRepository_ListColumns(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-1", BoardID: "board-123", Name: "To Do"})
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-2", BoardID: "board-123", Name: "In Progress"})
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-3", BoardID: "board-456", Name: "Done"})

	columns, err := repo.ListColumns(ctx, "board-123")
	if err != nil {
		t.Fatalf("failed to list columns: %v", err)
	}
	if len(columns) != 2 {
		t.Errorf("expected 2 columns for board-123, got %d", len(columns))
	}
}

// Task CRUD tests

func TestMemoryRepository_TaskCRUD(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	// Create
	task := &models.Task{
		WorkspaceID: "ws-1",
		BoardID:     "board-123",
		ColumnID:    "col-123",
		Title:       "Test Task",
		Description: "A test task",
		State:       v1.TaskStateTODO,
		Priority:    5,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	if task.ID == "" {
		t.Error("expected task ID to be set")
	}
	if task.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Get
	retrieved, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if retrieved.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", retrieved.Title)
	}

	// Update
	task.Title = "Updated Task"
	task.Description = "Updated description"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("failed to update task: %v", err)
	}
	retrieved, _ = repo.GetTask(ctx, task.ID)
	if retrieved.Title != "Updated Task" {
		t.Errorf("expected title 'Updated Task', got %s", retrieved.Title)
	}

	// Delete
	if err := repo.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("failed to delete task: %v", err)
	}
	_, err = repo.GetTask(ctx, task.ID)
	if err == nil {
		t.Error("expected task to be deleted")
	}
}

func TestMemoryRepository_TaskNotFound(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	_, err := repo.GetTask(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}

	err = repo.UpdateTask(ctx, &models.Task{ID: "nonexistent", Title: "Test"})
	if err == nil {
		t.Error("expected error for updating nonexistent task")
	}

	err = repo.DeleteTask(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent task")
	}
}

func TestMemoryRepository_UpdateTaskState(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-123", Title: "Test", State: v1.TaskStateTODO}
	_ = repo.CreateTask(ctx, task)

	err := repo.UpdateTaskState(ctx, "task-123", v1.TaskStateInProgress)
	if err != nil {
		t.Fatalf("failed to update task state: %v", err)
	}

	retrieved, _ := repo.GetTask(ctx, "task-123")
	if retrieved.State != v1.TaskStateInProgress {
		t.Errorf("expected state IN_PROGRESS, got %s", retrieved.State)
	}
}

func TestMemoryRepository_UpdateTaskStateNotFound(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	err := repo.UpdateTaskState(ctx, "nonexistent", v1.TaskStateInProgress)
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestMemoryRepository_ListTasks(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-1", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-1", Title: "Task 2"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-3", WorkspaceID: "ws-1", BoardID: "board-456", ColumnID: "col-2", Title: "Task 3"})

	tasks, err := repo.ListTasks(ctx, "board-123")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for board-123, got %d", len(tasks))
	}
}

func TestMemoryRepository_ListTasksByColumn(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-1", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-1", Title: "Task 2"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-3", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-2", Title: "Task 3"})

	tasks, err := repo.ListTasksByColumn(ctx, "col-1")
	if err != nil {
		t.Fatalf("failed to list tasks by column: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for col-1, got %d", len(tasks))
	}
}

// Message CRUD tests

func setupMemoryTestSession(ctx context.Context, repo *MemoryRepository, taskID string) string {
	session := &models.AgentSession{
		ID:             "session-123",
		TaskID:         taskID,
		AgentProfileID: "profile-123",
		State:          models.AgentSessionStateStarting,
	}
	_ = repo.CreateAgentSession(ctx, session)
	return session.ID
}

func TestMemoryRepository_MessageCRUD(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	// Create a task first
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupMemoryTestSession(ctx, repo, task.ID)

	// Create comment
	comment := &models.Message{
		AgentSessionID: sessionID,
		TaskID:         "task-123",
		AuthorType:     models.MessageAuthorUser,
		AuthorID:       "user-123",
		Content:        "This is a test comment",
		RequestsInput:  false,
	}
	if err := repo.CreateMessage(ctx, comment); err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}
	if comment.ID == "" {
		t.Error("expected comment ID to be set")
	}
	if comment.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Get comment
	retrieved, err := repo.GetMessage(ctx, comment.ID)
	if err != nil {
		t.Fatalf("failed to get comment: %v", err)
	}
	if retrieved.Content != "This is a test comment" {
		t.Errorf("expected content 'This is a test comment', got %s", retrieved.Content)
	}
	if retrieved.AuthorType != models.MessageAuthorUser {
		t.Errorf("expected author type 'user', got %s", retrieved.AuthorType)
	}

	// Delete comment
	if err := repo.DeleteMessage(ctx, comment.ID); err != nil {
		t.Fatalf("failed to delete comment: %v", err)
	}
	_, err = repo.GetMessage(ctx, comment.ID)
	if err == nil {
		t.Error("expected comment to be deleted")
	}
}

func TestMemoryRepository_MessageNotFound(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	_, err := repo.GetMessage(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent comment")
	}

	err = repo.DeleteMessage(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent comment")
	}
}

func TestMemoryRepository_ListMessages(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	// Create a task
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupMemoryTestSession(ctx, repo, task.ID)

	// Create multiple comments
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-1", AgentSessionID: sessionID, TaskID: "task-123", AuthorType: models.MessageAuthorUser, Content: "Comment 1"})
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-2", AgentSessionID: sessionID, TaskID: "task-123", AuthorType: models.MessageAuthorAgent, Content: "Comment 2"})
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-3", AgentSessionID: "session-456", TaskID: "task-456", AuthorType: models.MessageAuthorUser, Content: "Comment 3"})

	comments, err := repo.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments for task-123, got %d", len(comments))
	}
}

func TestMemoryRepository_ListMessagesPagination(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupMemoryTestSession(ctx, repo, task.ID)

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_ = repo.CreateMessage(ctx, &models.Message{
		ID:             "comment-1",
		AgentSessionID: sessionID,
		TaskID:         "task-123",
		AuthorType:     models.MessageAuthorUser,
		Content:        "Comment 1",
		CreatedAt:      baseTime.Add(-2 * time.Minute),
	})
	_ = repo.CreateMessage(ctx, &models.Message{
		ID:             "comment-2",
		AgentSessionID: sessionID,
		TaskID:         "task-123",
		AuthorType:     models.MessageAuthorUser,
		Content:        "Comment 2",
		CreatedAt:      baseTime.Add(-1 * time.Minute),
	})
	_ = repo.CreateMessage(ctx, &models.Message{
		ID:             "comment-3",
		AgentSessionID: sessionID,
		TaskID:         "task-123",
		AuthorType:     models.MessageAuthorUser,
		Content:        "Comment 3",
		CreatedAt:      baseTime,
	})

	comments, hasMore, err := repo.ListMessagesPaginated(ctx, sessionID, ListMessagesOptions{
		Limit: 2,
		Sort:  "desc",
	})
	if err != nil {
		t.Fatalf("failed to list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if !hasMore {
		t.Error("expected hasMore to be true")
	}
	if comments[0].ID != "comment-3" {
		t.Errorf("expected newest comment first, got %s", comments[0].ID)
	}
}

func TestMemoryRepository_MessageWithRequestsInput(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	// Create a task
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupMemoryTestSession(ctx, repo, task.ID)

	// Create agent comment requesting input
	comment := &models.Message{
		AgentSessionID: sessionID,
		TaskID:         "task-123",
		AuthorType:     models.MessageAuthorAgent,
		AuthorID:       "agent-123",
		Content:        "What should I do next?",
		RequestsInput:  true,
	}
	if err := repo.CreateMessage(ctx, comment); err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	retrieved, _ := repo.GetMessage(ctx, comment.ID)
	if !retrieved.RequestsInput {
		t.Error("expected RequestsInput to be true")
	}
}
