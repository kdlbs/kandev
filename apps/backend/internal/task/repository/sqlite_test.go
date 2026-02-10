package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/worktree"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func createTestSQLiteRepo(t *testing.T) (*sqlite.Repository, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("failed to open SQLite database: %v", err)
	}
	repo, err := sqlite.NewWithDB(dbConn)
	if err != nil {
		t.Fatalf("failed to create SQLite repository: %v", err)
	}
	if _, err := worktree.NewSQLiteStore(repo.DB()); err != nil {
		t.Fatalf("failed to init worktree store: %v", err)
	}

	cleanup := func() {
		if err := dbConn.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if err := repo.Close(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	}

	return repo, cleanup
}

func TestNewSQLiteRepositoryWithDB(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
	if repo.DB() == nil {
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
	if boards[0].Name != "Development" {
		t.Errorf("expected Development board, got %s", boards[0].Name)
	}
	// Note: workflow steps are now managed by the workflow repository, not the task repository
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

// Task CRUD tests

func TestSQLiteRepository_TaskCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workspace and board for foreign keys
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	// Create task (workflow steps are managed by workflow repository)
	task := &models.Task{
		WorkspaceID:    "ws-1",
		BoardID:        "board-123",
		WorkflowStepID: "step-123",
		Title:          "Test Task",
		Description:    "A test task",
		State:          v1.TaskStateTODO,
		Priority:       5,
		Metadata:       map[string]interface{}{"key": "value"},
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

	// Create workspace, board, and task
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Test", State: v1.TaskStateTODO}
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

	// Create workspace and board
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Task 2"})

	tasks, err := repo.ListTasks(ctx, "board-123")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestSQLiteRepository_ListTasksByWorkflowStep(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workspace and board
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	// Tasks with different workflow steps
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-1", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-1", Title: "Task 2"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-3", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-2", Title: "Task 3"})

	tasks, err := repo.ListTasksByWorkflowStep(ctx, "step-1")
	if err != nil {
		t.Fatalf("failed to list tasks by workflow step: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for step-1, got %d", len(tasks))
	}
}

func TestSQLiteRepository_ListTasksByWorkspace(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workspaces and board
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	// Create tasks in workspace 1
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-1", Title: "Task One"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-1", Title: "Task Two"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-3", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-1", Title: "Task Three"})
	// Create task in workspace 2
	board2 := &models.Board{ID: "board-456", WorkspaceID: "ws-2", Name: "Test Board 2"}
	_ = repo.CreateBoard(ctx, board2)
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-4", WorkspaceID: "ws-2", BoardID: "board-456", WorkflowStepID: "step-2", Title: "Task Four"})

	// Test basic listing without search
	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", 1, 10)
	if err != nil {
		t.Fatalf("failed to list tasks by workspace: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3 tasks for ws-1, got %d", total)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks returned, got %d", len(tasks))
	}

	// Test pagination
	tasks, total, err = repo.ListTasksByWorkspace(ctx, "ws-1", "", 1, 2)
	if err != nil {
		t.Fatalf("failed to list tasks with pagination: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3 tasks, got %d", total)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks per page, got %d", len(tasks))
	}

	// Test page 2
	tasksPage2, _, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", 2, 2)
	if err != nil {
		t.Fatalf("failed to list tasks page 2: %v", err)
	}
	if len(tasksPage2) != 1 {
		t.Errorf("expected 1 task on page 2, got %d", len(tasksPage2))
	}
}

func TestSQLiteRepository_ListTasksByWorkspaceWithSearch(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workspace, board, and repository
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	board := &models.Board{ID: "board-123", WorkspaceID: "ws-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	repository := &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "MyProject", LocalPath: "/home/user/projects/myproject"}
	_ = repo.CreateRepository(ctx, repository)

	// Create tasks with different titles and descriptions
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-1", Title: "Fix authentication bug", Description: "Users cannot login"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-1", Title: "Add new feature", Description: "Implement dark mode"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-3", WorkspaceID: "ws-1", BoardID: "board-123", WorkflowStepID: "step-1", Title: "Refactor codebase", Description: "Clean up authentication module"})

	// Link task-1 to the repository
	_ = repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main"})

	// Test search by title
	_, totalAuth, err := repo.ListTasksByWorkspace(ctx, "ws-1", "authentication", 1, 10)
	if err != nil {
		t.Fatalf("failed to search tasks by title: %v", err)
	}
	if totalAuth != 2 {
		t.Errorf("expected 2 tasks matching 'authentication', got %d", totalAuth)
	}

	// Test search by description
	tasksDarkMode, totalDarkMode, err := repo.ListTasksByWorkspace(ctx, "ws-1", "dark mode", 1, 10)
	if err != nil {
		t.Fatalf("failed to search tasks by description: %v", err)
	}
	if totalDarkMode != 1 {
		t.Errorf("expected 1 task matching 'dark mode', got %d", totalDarkMode)
	}
	if len(tasksDarkMode) != 1 || tasksDarkMode[0].ID != "task-2" {
		t.Errorf("expected task-2 to be returned")
	}

	// Test search by repository name
	tasksRepo, totalRepo, err := repo.ListTasksByWorkspace(ctx, "ws-1", "MyProject", 1, 10)
	if err != nil {
		t.Fatalf("failed to search tasks by repository name: %v", err)
	}
	if totalRepo != 1 {
		t.Errorf("expected 1 task matching repository 'MyProject', got %d", totalRepo)
	}
	if len(tasksRepo) != 1 || tasksRepo[0].ID != "task-1" {
		t.Errorf("expected task-1 to be returned")
	}

	// Test search by repository local_path
	_, totalPath, err := repo.ListTasksByWorkspace(ctx, "ws-1", "myproject", 1, 10)
	if err != nil {
		t.Fatalf("failed to search tasks by repository path: %v", err)
	}
	if totalPath != 1 {
		t.Errorf("expected 1 task matching repository path 'myproject', got %d", totalPath)
	}

	// Test search with no results
	tasksNone, totalNone, err := repo.ListTasksByWorkspace(ctx, "ws-1", "nonexistent", 1, 10)
	if err != nil {
		t.Fatalf("failed to search tasks with no results: %v", err)
	}
	if totalNone != 0 {
		t.Errorf("expected 0 tasks matching 'nonexistent', got %d", totalNone)
	}
	if len(tasksNone) != 0 {
		t.Errorf("expected empty tasks slice, got %d tasks", len(tasksNone))
	}
}

func TestSQLiteRepository_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persistence_test.db")
	ctx := context.Background()

	// Create repository and add data
	dbConn1, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("failed to open SQLite database: %v", err)
	}
	repo1, err := sqlite.NewWithDB(dbConn1)
	if err != nil {
		t.Fatalf("failed to create first repository: %v", err)
	}

	_ = repo1.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	board := &models.Board{ID: "persist-board", WorkspaceID: "ws-1", Name: "Persistent Board"}
	_ = repo1.CreateBoard(ctx, board)
	if err := repo1.Close(); err != nil {
		t.Fatalf("failed to close repo: %v", err)
	}
	if err := dbConn1.Close(); err != nil {
		t.Fatalf("failed to close sqlite db: %v", err)
	}

	// Reopen repository and verify data persisted
	dbConn2, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("failed to open SQLite database: %v", err)
	}
	repo2, err := sqlite.NewWithDB(dbConn2)
	if err != nil {
		t.Fatalf("failed to create second repository: %v", err)
	}
	defer func() {
		if err := dbConn2.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if err := repo2.Close(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	}()

	retrieved, err := repo2.GetBoard(ctx, "persist-board")
	if err != nil {
		t.Fatalf("failed to get board after reopen: %v", err)
	}
	if retrieved.Name != "Persistent Board" {
		t.Errorf("expected name 'Persistent Board', got %s", retrieved.Name)
	}
}

// Message CRUD tests

func setupSQLiteTestSession(ctx context.Context, repo *sqlite.Repository, taskID, sessionID string) string {
	session := &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateStarting,
	}
	_ = repo.CreateTaskSession(ctx, session)
	return session.ID
}

func setupSQLiteTestTurn(ctx context.Context, repo *sqlite.Repository, sessionID, taskID, turnID string) string {
	now := time.Now()
	turn := &models.Turn{
		ID:            turnID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_ = repo.CreateTurn(ctx, turn)
	return turn.ID
}

func TestSQLiteRepository_MessageCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board first (required for foreign key constraints)
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	// Create a task first
	task := &models.Task{ID: "task-123", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupSQLiteTestSession(ctx, repo, task.ID, "session-123")
	turnID := setupSQLiteTestTurn(ctx, repo, sessionID, task.ID, "turn-123")

	// Create comment
	comment := &models.Message{
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorUser,
		AuthorID:      "user-123",
		Content:       "This is a test comment",
		RequestsInput: false,
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

func TestSQLiteRepository_MessageNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
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

func TestSQLiteRepository_ListMessages(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board first (required for foreign key constraints)
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	// Create tasks
	task := &models.Task{ID: "task-123", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	task2 := &models.Task{ID: "task-456", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Test Task 2"}
	_ = repo.CreateTask(ctx, task2)
	sessionID := setupSQLiteTestSession(ctx, repo, task.ID, "session-123")
	session2ID := setupSQLiteTestSession(ctx, repo, task2.ID, "session-456")
	turnID := setupSQLiteTestTurn(ctx, repo, sessionID, task.ID, "turn-123")
	turn2ID := setupSQLiteTestTurn(ctx, repo, session2ID, task2.ID, "turn-456")

	// Create multiple comments
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-1", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Comment 1"})
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-2", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorAgent, Content: "Comment 2"})
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-3", TaskSessionID: session2ID, TaskID: "task-456", TurnID: turn2ID, AuthorType: models.MessageAuthorUser, Content: "Comment 3"})

	comments, err := repo.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments for task-123, got %d", len(comments))
	}
}

func TestSQLiteRepository_ListMessagesPagination(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	task := &models.Task{ID: "task-123", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupSQLiteTestSession(ctx, repo, task.ID, "session-123")
	turnID := setupSQLiteTestTurn(ctx, repo, sessionID, task.ID, "turn-123")

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_ = repo.CreateMessage(ctx, &models.Message{
		ID:            "comment-1",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorUser,
		Content:       "Comment 1",
		CreatedAt:     baseTime.Add(-2 * time.Minute),
	})
	_ = repo.CreateMessage(ctx, &models.Message{
		ID:            "comment-2",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorUser,
		Content:       "Comment 2",
		CreatedAt:     baseTime.Add(-1 * time.Minute),
	})
	_ = repo.CreateMessage(ctx, &models.Message{
		ID:            "comment-3",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorUser,
		Content:       "Comment 3",
		CreatedAt:     baseTime,
	})

	comments, hasMore, err := repo.ListMessagesPaginated(ctx, sessionID, models.ListMessagesOptions{
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

func TestSQLiteRepository_MessageWithRequestsInput(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board first (required for foreign key constraints)
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)

	// Create a task
	task := &models.Task{ID: "task-123", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupSQLiteTestSession(ctx, repo, task.ID, "session-123")
	turnID := setupSQLiteTestTurn(ctx, repo, sessionID, task.ID, "turn-123")

	// Create agent comment requesting input
	comment := &models.Message{
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		AuthorID:      "agent-123",
		Content:       "What should I do next?",
		RequestsInput: true,
	}
	if err := repo.CreateMessage(ctx, comment); err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	retrieved, _ := repo.GetMessage(ctx, comment.ID)
	if !retrieved.RequestsInput {
		t.Error("expected RequestsInput to be true")
	}
}

// TaskSession CRUD tests

func TestSQLiteRepository_TaskSessionCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board and task first (required for foreign key constraints)
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	task := &models.Task{ID: "task-123", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create agent session
	session := &models.TaskSession{
		TaskID:           "task-123",
		AgentExecutionID: "execution-abc",
		ContainerID:      "container-xyz",
		AgentProfileID:   "profile-123",
		ExecutorID:       "executor-1",
		EnvironmentID:    "env-1",
		State:            models.TaskSessionStateStarting,
		Metadata:         map[string]interface{}{"key": "value"},
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
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
	retrieved, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to get agent session: %v", err)
	}
	if retrieved.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123', got %s", retrieved.TaskID)
	}
	if retrieved.AgentProfileID != "profile-123" {
		t.Errorf("expected AgentProfileID 'profile-123', got %s", retrieved.AgentProfileID)
	}
	if retrieved.State != models.TaskSessionStateStarting {
		t.Errorf("expected state 'starting', got %s", retrieved.State)
	}
	if retrieved.Metadata["key"] != "value" {
		t.Errorf("expected metadata key 'value', got %v", retrieved.Metadata["key"])
	}

	// Update agent session
	session.State = models.TaskSessionStateRunning
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update agent session: %v", err)
	}
	retrieved, _ = repo.GetTaskSession(ctx, session.ID)
	if retrieved.State != models.TaskSessionStateRunning {
		t.Errorf("expected state 'running', got %s", retrieved.State)
	}

	// Delete agent session
	if err := repo.DeleteTaskSession(ctx, session.ID); err != nil {
		t.Fatalf("failed to delete agent session: %v", err)
	}
	_, err = repo.GetTaskSession(ctx, session.ID)
	if err == nil {
		t.Error("expected agent session to be deleted")
	}
}

func TestSQLiteRepository_TaskSessionNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_, err := repo.GetTaskSession(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent session")
	}

	err = repo.UpdateTaskSession(ctx, &models.TaskSession{ID: "nonexistent", TaskID: "task-123"})
	if err == nil {
		t.Error("expected error for updating nonexistent agent session")
	}

	err = repo.DeleteTaskSession(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent agent session")
	}
}

func TestSQLiteRepository_TaskSessionByTaskID(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board and task
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	task := &models.Task{ID: "task-123", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create multiple sessions for the same task (simulating session history)
	session1 := &models.TaskSession{
		ID:             "session-1",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateCompleted,
	}
	_ = repo.CreateTaskSession(ctx, session1)

	session2 := &models.TaskSession{
		ID:             "session-2",
		TaskID:         "task-123",
		AgentProfileID: "profile-2",
		State:          models.TaskSessionStateRunning,
	}
	_ = repo.CreateTaskSession(ctx, session2)

	// GetTaskSessionByTaskID should return the most recent session
	retrieved, err := repo.GetTaskSessionByTaskID(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get agent session by task ID: %v", err)
	}
	if retrieved.ID != "session-2" {
		t.Errorf("expected session-2 (most recent), got %s", retrieved.ID)
	}

	// GetActiveTaskSessionByTaskID should return the active session
	active, err := repo.GetActiveTaskSessionByTaskID(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get active agent session by task ID: %v", err)
	}
	if active.ID != "session-2" {
		t.Errorf("expected session-2 (active), got %s", active.ID)
	}
	if active.State != models.TaskSessionStateRunning {
		t.Errorf("expected state 'running', got %s", active.State)
	}

	// Test when no active session exists
	session2.State = models.TaskSessionStateCompleted
	_ = repo.UpdateTaskSession(ctx, session2)

	_, err = repo.GetActiveTaskSessionByTaskID(ctx, "task-123")
	if err == nil {
		t.Error("expected error when no active session exists")
	}

	// Test for nonexistent task
	_, err = repo.GetTaskSessionByTaskID(ctx, "nonexistent-task")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestSQLiteRepository_ListTaskSessions(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board and tasks
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	task1 := &models.Task{ID: "task-1", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Task 1"}
	_ = repo.CreateTask(ctx, task1)
	task2 := &models.Task{ID: "task-2", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Task 2"}
	_ = repo.CreateTask(ctx, task2)

	// Create sessions for different tasks
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-1", TaskID: "task-1", AgentProfileID: "profile-1", State: models.TaskSessionStateCompleted})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-2", TaskID: "task-1", AgentProfileID: "profile-1", State: models.TaskSessionStateRunning})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-3", TaskID: "task-2", AgentProfileID: "profile-2", State: models.TaskSessionStateStarting})

	// List sessions for task-1
	sessions, err := repo.ListTaskSessions(ctx, "task-1")
	if err != nil {
		t.Fatalf("failed to list agent sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions for task-1, got %d", len(sessions))
	}

	// List all active sessions
	activeSessions, err := repo.ListActiveTaskSessions(ctx)
	if err != nil {
		t.Fatalf("failed to list active agent sessions: %v", err)
	}
	if len(activeSessions) != 2 {
		t.Errorf("expected 2 active sessions, got %d", len(activeSessions))
	}

	// Verify only active statuses are returned
	for _, s := range activeSessions {
		if s.State != models.TaskSessionStateStarting && s.State != models.TaskSessionStateRunning && s.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected active state, got %s", s.State)
		}
	}
}

func TestSQLiteRepository_UpdateTaskSessionState(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create board and task
	board := &models.Board{ID: "board-123", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	task := &models.Task{ID: "task-123", BoardID: "board-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)

	// Create an agent session
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateStarting,
	}
	_ = repo.CreateTaskSession(ctx, session)

	// Update to running status
	err := repo.UpdateTaskSessionState(ctx, "session-123", models.TaskSessionStateRunning, "")
	if err != nil {
		t.Fatalf("failed to update agent session status: %v", err)
	}
	retrieved, _ := repo.GetTaskSession(ctx, "session-123")
	if retrieved.State != models.TaskSessionStateRunning {
		t.Errorf("expected state 'running', got %s", retrieved.State)
	}
	if retrieved.CompletedAt != nil {
		t.Error("expected CompletedAt to be nil for running status")
	}

	// Update to completed status (should set CompletedAt)
	err = repo.UpdateTaskSessionState(ctx, "session-123", models.TaskSessionStateCompleted, "")
	if err != nil {
		t.Fatalf("failed to update agent session status to completed: %v", err)
	}
	retrieved, _ = repo.GetTaskSession(ctx, "session-123")
	if retrieved.State != models.TaskSessionStateCompleted {
		t.Errorf("expected state 'completed', got %s", retrieved.State)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for completed status")
	}

	// Test failed status with error message
	session2 := &models.TaskSession{
		ID:             "session-456",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateRunning,
	}
	_ = repo.CreateTaskSession(ctx, session2)

	err = repo.UpdateTaskSessionState(ctx, "session-456", models.TaskSessionStateFailed, "connection timeout")
	if err != nil {
		t.Fatalf("failed to update agent session status to failed: %v", err)
	}
	retrieved, _ = repo.GetTaskSession(ctx, "session-456")
	if retrieved.State != models.TaskSessionStateFailed {
		t.Errorf("expected state 'failed', got %s", retrieved.State)
	}
	if retrieved.ErrorMessage != "connection timeout" {
		t.Errorf("expected error message 'connection timeout', got %s", retrieved.ErrorMessage)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for failed status")
	}

	// Test stopped status
	session3 := &models.TaskSession{
		ID:             "session-789",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateRunning,
	}
	_ = repo.CreateTaskSession(ctx, session3)

	err = repo.UpdateTaskSessionState(ctx, "session-789", models.TaskSessionStateCancelled, "")
	if err != nil {
		t.Fatalf("failed to update agent session status to stopped: %v", err)
	}
	retrieved, _ = repo.GetTaskSession(ctx, "session-789")
	if retrieved.State != models.TaskSessionStateCancelled {
		t.Errorf("expected state 'cancelled', got %s", retrieved.State)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for stopped status")
	}

	// Test nonexistent session
	err = repo.UpdateTaskSessionState(ctx, "nonexistent", models.TaskSessionStateRunning, "")
	if err == nil {
		t.Error("expected error for updating nonexistent session status")
	}
}

func TestSQLiteRepository_CompleteRunningToolCallsForTurn(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Setup
	board := &models.Board{ID: "board-1", Name: "Test Board"}
	_ = repo.CreateBoard(ctx, board)
	task := &models.Task{ID: "task-1", BoardID: "board-1", WorkflowStepID: "step-1", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	sessionID := setupSQLiteTestSession(ctx, repo, task.ID, "session-1")
	turnID := setupSQLiteTestTurn(ctx, repo, sessionID, task.ID, "turn-1")

	// Create a tool call message with status "running"
	runningTool := &models.Message{
		ID: "msg-running-1", TaskSessionID: sessionID, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Content: "Running tool",
		Type:     models.MessageTypeToolCall,
		Metadata: map[string]interface{}{"tool_call_id": "tc-1", "status": "running"},
	}
	// Create a tool call message already "complete"
	completeTool := &models.Message{
		ID: "msg-complete-1", TaskSessionID: sessionID, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Content: "Complete tool",
		Type:     models.MessageTypeToolCall,
		Metadata: map[string]interface{}{"tool_call_id": "tc-2", "status": "complete"},
	}
	// Create a regular message (no tool_call_id) with status "running" — should NOT be affected
	regularMsg := &models.Message{
		ID: "msg-regular-1", TaskSessionID: sessionID, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Content: "Regular message",
		Type:     models.MessageTypeMessage,
		Metadata: map[string]interface{}{"status": "running"},
	}
	// Create a second running tool call
	runningTool2 := &models.Message{
		ID: "msg-running-2", TaskSessionID: sessionID, TaskID: task.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Content: "Running tool 2",
		Type:     models.MessageTypeToolCall,
		Metadata: map[string]interface{}{"tool_call_id": "tc-3", "status": "running"},
	}

	for _, msg := range []*models.Message{runningTool, completeTool, regularMsg, runningTool2} {
		if err := repo.CreateMessage(ctx, msg); err != nil {
			t.Fatalf("failed to create message %s: %v", msg.ID, err)
		}
	}

	// Execute
	affected, err := repo.CompleteRunningToolCallsForTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("CompleteRunningToolCallsForTurn failed: %v", err)
	}

	// Should have updated exactly 2 running tool call messages
	if affected != 2 {
		t.Errorf("expected 2 affected rows, got %d", affected)
	}

	// Verify running tool calls are now "complete"
	msg1, _ := repo.GetMessage(ctx, "msg-running-1")
	if msg1.Metadata["status"] != "complete" {
		t.Errorf("expected msg-running-1 status 'complete', got %v", msg1.Metadata["status"])
	}
	msg2, _ := repo.GetMessage(ctx, "msg-running-2")
	if msg2.Metadata["status"] != "complete" {
		t.Errorf("expected msg-running-2 status 'complete', got %v", msg2.Metadata["status"])
	}

	// Verify already-complete tool call is unchanged
	msg3, _ := repo.GetMessage(ctx, "msg-complete-1")
	if msg3.Metadata["status"] != "complete" {
		t.Errorf("expected msg-complete-1 status 'complete', got %v", msg3.Metadata["status"])
	}

	// Verify regular message (no tool_call_id) was NOT affected
	msg4, _ := repo.GetMessage(ctx, "msg-regular-1")
	if msg4.Metadata["status"] != "running" {
		t.Errorf("expected msg-regular-1 status 'running' (unchanged), got %v", msg4.Metadata["status"])
	}

	// Running again should affect 0 rows
	affected2, err := repo.CompleteRunningToolCallsForTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("second CompleteRunningToolCallsForTurn failed: %v", err)
	}
	if affected2 != 0 {
		t.Errorf("expected 0 affected rows on second call, got %d", affected2)
	}
}
