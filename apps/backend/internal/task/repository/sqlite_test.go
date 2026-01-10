package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

// Board CRUD tests

func TestSQLiteRepository_BoardCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create
	board := &models.Board{Name: "Test Board", Description: "A test board", OwnerID: "owner-123"}
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

	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-1", Name: "Board 1"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-2", Name: "Board 2"})
	_ = repo.CreateBoard(ctx, &models.Board{ID: "board-3", Name: "Board 3"})

	boards, err := repo.ListBoards(ctx)
	if err != nil {
		t.Fatalf("failed to list boards: %v", err)
	}
	if len(boards) != 3 {
		t.Errorf("expected 3 boards, got %d", len(boards))
	}
}

// Column CRUD tests

func TestSQLiteRepository_ColumnCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// First create a board for foreign key
	board := &models.Board{ID: "board-123", Name: "Test Board"}
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

	board := &models.Board{ID: "board-123", Name: "Test Board"}
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

	// Create board and column for foreign keys
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do"}
	_ = repo.CreateColumn(ctx, column)

	// Create task
	task := &models.Task{
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

	// Create board, column, and task
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do"}
	_ = repo.CreateColumn(ctx, column)
	task := &models.Task{ID: "task-123", BoardID: "board-123", ColumnID: "col-123", Title: "Test", State: v1.TaskStateTODO}
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

	// Create board and column
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	column := &models.Column{ID: "col-123", BoardID: "board-123", Name: "To Do"}
	_ = repo.CreateColumn(ctx, column)

	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", BoardID: "board-123", ColumnID: "col-123", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", BoardID: "board-123", ColumnID: "col-123", Title: "Task 2"})

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

	// Create board and columns
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	col1 := &models.Column{ID: "col-1", BoardID: "board-123", Name: "To Do"}
	col2 := &models.Column{ID: "col-2", BoardID: "board-123", Name: "Done"}
	_ = repo.CreateColumn(ctx, col1)
	_ = repo.CreateColumn(ctx, col2)

	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", BoardID: "board-123", ColumnID: "col-1", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", BoardID: "board-123", ColumnID: "col-1", Title: "Task 2"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-3", BoardID: "board-123", ColumnID: "col-2", Title: "Task 3"})

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

	board := &models.Board{ID: "persist-board", Name: "Persistent Board"}
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

