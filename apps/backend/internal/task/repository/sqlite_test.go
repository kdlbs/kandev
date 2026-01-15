package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func createTestSQLiteRepo(t *testing.T) (*SQLiteRepository, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create SQLite repository: %v", err)
	}

	cleanup := func() {
		repo.Close()
		os.Remove(dbPath)
	}

	return repo, cleanup
}

func TestNewSQLiteRepository(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
	if repo.db == nil {
		t.Error("expected db to be initialized")
	}
}

func TestSQLiteRepository_Close(t *testing.T) {
	repo, _ := createTestSQLiteRepo(t)
	err := repo.Close()
	if err != nil {
		t.Errorf("expected no error on close, got %v", err)
	}
}

func TestSQLiteRepository_SeedsDefaultWorkspace(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	workspaces, err := repo.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("failed to list workspaces: %v", err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(workspaces))
	}
	if workspaces[0].Name != "Default Workspace" {
		t.Errorf("expected Default Workspace, got %s", workspaces[0].Name)
	}

	boards, err := repo.ListBoards(ctx, workspaces[0].ID)
	if err != nil {
		t.Fatalf("failed to list boards: %v", err)
	}
	if len(boards) != 1 {
		t.Fatalf("expected 1 board, got %d", len(boards))
	}
	if boards[0].Name != "Dev" {
		t.Errorf("expected Dev board, got %s", boards[0].Name)
	}

	columns, err := repo.ListColumns(ctx, boards[0].ID)
	if err != nil {
		t.Fatalf("failed to list columns: %v", err)
	}
	if len(columns) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(columns))
	}
	expected := map[string]v1.TaskState{
		"Todo":        v1.TaskStateTODO,
		"In Progress": v1.TaskStateInProgress,
		"Review":      v1.TaskStateReview,
		"Done":        v1.TaskStateCompleted,
	}
	for _, column := range columns {
		expectedState, ok := expected[column.Name]
		if !ok {
			t.Errorf("unexpected column name %s", column.Name)
			continue
		}
		if column.State != expectedState {
			t.Errorf("expected %s state %s, got %s", column.Name, expectedState, column.State)
		}
	}
}

// Board CRUD tests

func TestSQLiteRepository_BoardCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
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

func TestSQLiteRepository_BoardNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
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

func TestSQLiteRepository_ListBoards(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
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

func TestSQLiteRepository_ColumnCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// First create a workspace and board for foreign key
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	// Create column
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

func TestSQLiteRepository_ColumnNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
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

func TestSQLiteRepository_ListColumns(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-1", BoardID: "board-123", Name: "To Do"})
	_ = repo.CreateColumn(ctx, &models.Column{ID: "col-2", BoardID: "board-123", Name: "In Progress"})

	columns, err := repo.ListColumns(ctx, "board-123")
	if err != nil {
		t.Fatalf("failed to list columns: %v", err)
	}
	if len(columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(columns))
	}
}

// Task CRUD tests

func TestSQLiteRepository_TaskCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workspace, board, and column for foreign keys
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do"}
	_ = repo.CreateColumn(ctx, column)

	// Create task
	task := &models.Task{
		WorkspaceID: "ws-1",
		BoardID:     "board-123",
		ColumnID:    "col-123",
		Title:       "Test Task",
		Description: "A test task",
		State:       v1.TaskStateTODO,
		Priority:    5,
		Metadata:    map[string]interface{}{"key": "value"},
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	if task.ID == "" {
		t.Error("expected task ID to be set")
	}

	// Get
	retrieved, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if retrieved.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", retrieved.Title)
	}
	if retrieved.Metadata["key"] != "value" {
		t.Errorf("expected metadata key 'value', got %v", retrieved.Metadata["key"])
	}

	// Update
	task.Title = "Updated Task"
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

func TestSQLiteRepository_TaskNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
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

func TestSQLiteRepository_UpdateTaskState(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workspace, board, column, and task
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do"}
	_ = repo.CreateColumn(ctx, column)
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

func TestSQLiteRepository_UpdateTaskStateNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	err := repo.UpdateTaskState(ctx, "nonexistent", v1.TaskStateInProgress)
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestSQLiteRepository_ListTasks(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workspace, board, and column
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do"}
	_ = repo.CreateColumn(ctx, column)

	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-123", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", BoardID: "board-123", ColumnID: "col-123", Title: "Task 2"})

	tasks, err := repo.ListTasks(ctx, "board-123")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestSQLiteRepository_ListTasksByColumn(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workspace, board, and columns
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	col1 := &models.Column{ID: "col-1", BoardID: "board-123", Name: "To Do"}
	col2 := &models.Column{ID: "col-2", BoardID: "board-123", Name: "Done"}
	_ = repo.CreateColumn(ctx, col1)
	_ = repo.CreateColumn(ctx, col2)

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

func TestSQLiteRepository_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persistence_test.db")
	ctx := context.Background()

	// Create repository and add data
	repo1, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create first repository: %v", err)
	}

	_ = repo1.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "persist-board", WorkspaceID: "ws-1", Name: "Persistent Board"}
	_ = repo1.CreateBoard(ctx, board)
	repo1.Close()

	// Reopen repository and verify data persisted
	repo2, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create second repository: %v", err)
	}
	defer repo2.Close()

	retrieved, err := repo2.GetBoard(ctx, "persist-board")
	if err != nil {
		t.Fatalf("failed to get board after reopen: %v", err)
	}
	if retrieved.Name != "Persistent Board" {
		t.Errorf("expected name 'Persistent Board', got %s", retrieved.Name)
	}
}

// Comment CRUD tests

func TestSQLiteRepository_CommentCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board and column first (required for foreign key constraints)
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "Test Column"}
	_ = repo.CreateColumn(ctx, column)

	// Create a task first
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create comment
	comment := &models.Comment{
		TaskID:        "task-123",
		AuthorType:    models.CommentAuthorUser,
		AuthorID:      "user-123",
		Content:       "This is a test comment",
		RequestsInput: false,
	}
	if err := repo.CreateComment(ctx, comment); err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}
	if comment.ID == "" {
		t.Error("expected comment ID to be set")
	}
	if comment.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Get comment
	retrieved, err := repo.GetComment(ctx, comment.ID)
	if err != nil {
		t.Fatalf("failed to get comment: %v", err)
	}
	if retrieved.Content != "This is a test comment" {
		t.Errorf("expected content 'This is a test comment', got %s", retrieved.Content)
	}
	if retrieved.AuthorType != models.CommentAuthorUser {
		t.Errorf("expected author type 'user', got %s", retrieved.AuthorType)
	}

	// Delete comment
	if err := repo.DeleteComment(ctx, comment.ID); err != nil {
		t.Fatalf("failed to delete comment: %v", err)
	}
	_, err = repo.GetComment(ctx, comment.ID)
	if err == nil {
		t.Error("expected comment to be deleted")
	}
}

func TestSQLiteRepository_CommentNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_, err := repo.GetComment(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent comment")
	}

	err = repo.DeleteComment(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent comment")
	}
}

func TestSQLiteRepository_ListComments(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board and column first (required for foreign key constraints)
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "Test Column"}
	_ = repo.CreateColumn(ctx, column)

	// Create tasks
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	task2 := &models.Task{ID: "task-456", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task 2"}
	_ = repo.CreateTask(ctx, task2)

	// Create multiple comments
	_ = repo.CreateComment(ctx, &models.Comment{ID: "comment-1", TaskID: "task-123", AuthorType: models.CommentAuthorUser, Content: "Comment 1"})
	_ = repo.CreateComment(ctx, &models.Comment{ID: "comment-2", TaskID: "task-123", AuthorType: models.CommentAuthorAgent, Content: "Comment 2"})
	_ = repo.CreateComment(ctx, &models.Comment{ID: "comment-3", TaskID: "task-456", AuthorType: models.CommentAuthorUser, Content: "Comment 3"})

	comments, err := repo.ListComments(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments for task-123, got %d", len(comments))
	}
}

func TestSQLiteRepository_ListCommentsPagination(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "Test Column"}
	_ = repo.CreateColumn(ctx, column)
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_ = repo.CreateComment(ctx, &models.Comment{
		ID:        "comment-1",
		TaskID:    "task-123",
		AuthorType: models.CommentAuthorUser,
		Content:   "Comment 1",
		CreatedAt: baseTime.Add(-2 * time.Minute),
	})
	_ = repo.CreateComment(ctx, &models.Comment{
		ID:        "comment-2",
		TaskID:    "task-123",
		AuthorType: models.CommentAuthorUser,
		Content:   "Comment 2",
		CreatedAt: baseTime.Add(-1 * time.Minute),
	})
	_ = repo.CreateComment(ctx, &models.Comment{
		ID:        "comment-3",
		TaskID:    "task-123",
		AuthorType: models.CommentAuthorUser,
		Content:   "Comment 3",
		CreatedAt: baseTime,
	})

	comments, hasMore, err := repo.ListCommentsPaginated(ctx, "task-123", ListCommentsOptions{
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

func TestSQLiteRepository_CommentWithRequestsInput(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board and column first (required for foreign key constraints)
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "Test Column"}
	_ = repo.CreateColumn(ctx, column)

	// Create a task
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create agent comment requesting input
	comment := &models.Comment{
		TaskID:        "task-123",
		AuthorType:    models.CommentAuthorAgent,
		AuthorID:      "agent-123",
		Content:       "What should I do next?",
		RequestsInput: true,
		ACPSessionID:  "session-abc",
	}
	if err := repo.CreateComment(ctx, comment); err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	retrieved, _ := repo.GetComment(ctx, comment.ID)
	if !retrieved.RequestsInput {
		t.Error("expected RequestsInput to be true")
	}
	if retrieved.ACPSessionID != "session-abc" {
		t.Errorf("expected ACPSessionID 'session-abc', got %s", retrieved.ACPSessionID)
	}
}

// AgentSession CRUD tests

func TestSQLiteRepository_AgentSessionCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board, column, and task first (required for foreign key constraints)
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "Test Column"}
	_ = repo.CreateColumn(ctx, column)
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create agent session
	session := &models.AgentSession{
		TaskID:          "task-123",
		AgentInstanceID: "instance-abc",
		ContainerID:     "container-xyz",
		AgentProfileID:  "profile-123",
		ACPSessionID:    "acp-session-123",
		ExecutorID:      "executor-1",
		EnvironmentID:   "env-1",
		Status:          models.AgentSessionStatusPending,
		Progress:        0,
		Metadata:        map[string]interface{}{"key": "value"},
	}
	if err := repo.CreateAgentSession(ctx, session); err != nil {
		t.Fatalf("failed to create agent session: %v", err)
	}
	if session.ID == "" {
		t.Error("expected session ID to be set")
	}
	if session.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
	if session.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Get agent session
	retrieved, err := repo.GetAgentSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get agent session: %v", err)
	}
	if retrieved.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123', got %s", retrieved.TaskID)
	}
	if retrieved.AgentProfileID != "profile-123" {
		t.Errorf("expected AgentProfileID 'profile-123', got %s", retrieved.AgentProfileID)
	}
	if retrieved.Status != models.AgentSessionStatusPending {
		t.Errorf("expected status 'pending', got %s", retrieved.Status)
	}
	if retrieved.Metadata["key"] != "value" {
		t.Errorf("expected metadata key 'value', got %v", retrieved.Metadata["key"])
	}

	// Update agent session
	session.Status = models.AgentSessionStatusRunning
	session.Progress = 50
	if err := repo.UpdateAgentSession(ctx, session); err != nil {
		t.Fatalf("failed to update agent session: %v", err)
	}
	retrieved, _ = repo.GetAgentSession(ctx, session.ID)
	if retrieved.Status != models.AgentSessionStatusRunning {
		t.Errorf("expected status 'running', got %s", retrieved.Status)
	}
	if retrieved.Progress != 50 {
		t.Errorf("expected progress 50, got %d", retrieved.Progress)
	}

	// Delete agent session
	if err := repo.DeleteAgentSession(ctx, session.ID); err != nil {
		t.Fatalf("failed to delete agent session: %v", err)
	}
	_, err = repo.GetAgentSession(ctx, session.ID)
	if err == nil {
		t.Error("expected agent session to be deleted")
	}
}

func TestSQLiteRepository_AgentSessionNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_, err := repo.GetAgentSession(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent session")
	}

	err = repo.UpdateAgentSession(ctx, &models.AgentSession{ID: "nonexistent", TaskID: "task-123"})
	if err == nil {
		t.Error("expected error for updating nonexistent agent session")
	}

	err = repo.DeleteAgentSession(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent agent session")
	}
}

func TestSQLiteRepository_AgentSessionByTaskID(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board, column, and task
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "Test Column"}
	_ = repo.CreateColumn(ctx, column)
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create multiple sessions for the same task (simulating session history)
	session1 := &models.AgentSession{
		ID:             "session-1",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		Status:         models.AgentSessionStatusCompleted,
	}
	_ = repo.CreateAgentSession(ctx, session1)

	session2 := &models.AgentSession{
		ID:             "session-2",
		TaskID:         "task-123",
		AgentProfileID: "profile-2",
		Status:         models.AgentSessionStatusRunning,
	}
	_ = repo.CreateAgentSession(ctx, session2)

	// GetAgentSessionByTaskID should return the most recent session
	retrieved, err := repo.GetAgentSessionByTaskID(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get agent session by task ID: %v", err)
	}
	if retrieved.ID != "session-2" {
		t.Errorf("expected session-2 (most recent), got %s", retrieved.ID)
	}

	// GetActiveAgentSessionByTaskID should return the active session
	active, err := repo.GetActiveAgentSessionByTaskID(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get active agent session by task ID: %v", err)
	}
	if active.ID != "session-2" {
		t.Errorf("expected session-2 (active), got %s", active.ID)
	}
	if active.Status != models.AgentSessionStatusRunning {
		t.Errorf("expected status 'running', got %s", active.Status)
	}

	// Test when no active session exists
	session2.Status = models.AgentSessionStatusCompleted
	_ = repo.UpdateAgentSession(ctx, session2)

	_, err = repo.GetActiveAgentSessionByTaskID(ctx, "task-123")
	if err == nil {
		t.Error("expected error when no active session exists")
	}

	// Test for nonexistent task
	_, err = repo.GetAgentSessionByTaskID(ctx, "nonexistent-task")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestSQLiteRepository_ListAgentSessions(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board, column, and tasks
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "Test Column"}
	_ = repo.CreateColumn(ctx, column)
	task1 := &models.Task{ID: "task-1", BoardID: "board-123", ColumnID: "col-123", Title: "Task 1"}
	_ = repo.CreateTask(ctx, task1)
	task2 := &models.Task{ID: "task-2", BoardID: "board-123", ColumnID: "col-123", Title: "Task 2"}
	_ = repo.CreateTask(ctx, task2)

	// Create sessions for different tasks
	_ = repo.CreateAgentSession(ctx, &models.AgentSession{ID: "session-1", TaskID: "task-1", AgentProfileID: "profile-1", Status: models.AgentSessionStatusCompleted})
	_ = repo.CreateAgentSession(ctx, &models.AgentSession{ID: "session-2", TaskID: "task-1", AgentProfileID: "profile-1", Status: models.AgentSessionStatusRunning})
	_ = repo.CreateAgentSession(ctx, &models.AgentSession{ID: "session-3", TaskID: "task-2", AgentProfileID: "profile-2", Status: models.AgentSessionStatusPending})

	// List sessions for task-1
	sessions, err := repo.ListAgentSessions(ctx, "task-1")
	if err != nil {
		t.Fatalf("failed to list agent sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions for task-1, got %d", len(sessions))
	}

	// List all active sessions
	activeSessions, err := repo.ListActiveAgentSessions(ctx)
	if err != nil {
		t.Fatalf("failed to list active agent sessions: %v", err)
	}
	if len(activeSessions) != 2 {
		t.Errorf("expected 2 active sessions, got %d", len(activeSessions))
	}

	// Verify only active statuses are returned
	for _, s := range activeSessions {
		if s.Status != models.AgentSessionStatusPending && s.Status != models.AgentSessionStatusRunning && s.Status != models.AgentSessionStatusWaiting {
			t.Errorf("expected active status, got %s", s.Status)
		}
	}
}

func TestSQLiteRepository_UpdateAgentSessionStatus(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board, column, and task
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "Test Column"}
	_ = repo.CreateColumn(ctx, column)
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create an agent session
	session := &models.AgentSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		Status:         models.AgentSessionStatusPending,
	}
	_ = repo.CreateAgentSession(ctx, session)

	// Update to running status
	err := repo.UpdateAgentSessionStatus(ctx, "session-123", models.AgentSessionStatusRunning, "")
	if err != nil {
		t.Fatalf("failed to update agent session status: %v", err)
	}
	retrieved, _ := repo.GetAgentSession(ctx, "session-123")
	if retrieved.Status != models.AgentSessionStatusRunning {
		t.Errorf("expected status 'running', got %s", retrieved.Status)
	}
	if retrieved.CompletedAt != nil {
		t.Error("expected CompletedAt to be nil for running status")
	}

	// Update to completed status (should set CompletedAt)
	err = repo.UpdateAgentSessionStatus(ctx, "session-123", models.AgentSessionStatusCompleted, "")
	if err != nil {
		t.Fatalf("failed to update agent session status to completed: %v", err)
	}
	retrieved, _ = repo.GetAgentSession(ctx, "session-123")
	if retrieved.Status != models.AgentSessionStatusCompleted {
		t.Errorf("expected status 'completed', got %s", retrieved.Status)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for completed status")
	}

	// Test failed status with error message
	session2 := &models.AgentSession{
		ID:             "session-456",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		Status:         models.AgentSessionStatusRunning,
	}
	_ = repo.CreateAgentSession(ctx, session2)

	err = repo.UpdateAgentSessionStatus(ctx, "session-456", models.AgentSessionStatusFailed, "connection timeout")
	if err != nil {
		t.Fatalf("failed to update agent session status to failed: %v", err)
	}
	retrieved, _ = repo.GetAgentSession(ctx, "session-456")
	if retrieved.Status != models.AgentSessionStatusFailed {
		t.Errorf("expected status 'failed', got %s", retrieved.Status)
	}
	if retrieved.ErrorMessage != "connection timeout" {
		t.Errorf("expected error message 'connection timeout', got %s", retrieved.ErrorMessage)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for failed status")
	}

	// Test stopped status
	session3 := &models.AgentSession{
		ID:             "session-789",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		Status:         models.AgentSessionStatusRunning,
	}
	_ = repo.CreateAgentSession(ctx, session3)

	err = repo.UpdateAgentSessionStatus(ctx, "session-789", models.AgentSessionStatusStopped, "")
	if err != nil {
		t.Fatalf("failed to update agent session status to stopped: %v", err)
	}
	retrieved, _ = repo.GetAgentSession(ctx, "session-789")
	if retrieved.Status != models.AgentSessionStatusStopped {
		t.Errorf("expected status 'stopped', got %s", retrieved.Status)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for stopped status")
	}

	// Test nonexistent session
	err = repo.UpdateAgentSessionStatus(ctx, "nonexistent", models.AgentSessionStatusRunning, "")
	if err == nil {
		t.Error("expected error for updating nonexistent session status")
	}
}
