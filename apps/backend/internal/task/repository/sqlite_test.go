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

	workflows, err := repo.ListWorkflows(ctx, workspaces[0].ID)
	if err != nil {
		t.Fatalf("failed to list workflows: %v", err)
	}
	if len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(workflows))
	}
	if workflows[0].Name != "Development" {
		t.Errorf("expected Development workflow, got %s", workflows[0].Name)
	}
	// Note: workflow steps are now managed by the workflow repository, not the task repository
}

// Workflow CRUD tests

func TestSQLiteRepository_WorkflowCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	workspace := &models.Workspace{ID: "ws-1", Name: "Workspace"}
	if err := repo.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Create
	workflow := &models.Workflow{WorkspaceID: workspace.ID, Name: "Test Workflow", Description: "A test workflow"}
	if err := repo.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	if workflow.ID == "" {
		t.Error("expected workflow ID to be set")
	}
	if workflow.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Get
	retrieved, err := repo.GetWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("failed to get workflow: %v", err)
	}
	if retrieved.Name != "Test Workflow" {
		t.Errorf("expected name 'Test Workflow', got %s", retrieved.Name)
	}

	// Update
	workflow.Name = "Updated Name"
	if err := repo.UpdateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("failed to update workflow: %v", err)
	}
	retrieved, _ = repo.GetWorkflow(ctx, workflow.ID)
	if retrieved.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %s", retrieved.Name)
	}

	// Delete
	if err := repo.DeleteWorkflow(ctx, workflow.ID); err != nil {
		t.Fatalf("failed to delete workflow: %v", err)
	}
	_, err = repo.GetWorkflow(ctx, workflow.ID)
	if err == nil {
		t.Error("expected workflow to be deleted")
	}
}

func TestSQLiteRepository_WorkflowNotFound(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_, err := repo.GetWorkflow(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent workflow")
	}

	err = repo.UpdateWorkflow(ctx, &models.Workflow{ID: "nonexistent", Name: "Test"})
	if err == nil {
		t.Error("expected error for updating nonexistent workflow")
	}

	err = repo.DeleteWorkflow(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent workflow")
	}
}

func TestSQLiteRepository_ListWorkflows(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"})

	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow 1"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-2", WorkspaceID: "ws-1", Name: "Workflow 2"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-3", WorkspaceID: "ws-2", Name: "Workflow 3"})

	workflows, err := repo.ListWorkflows(ctx, "ws-1")
	if err != nil {
		t.Fatalf("failed to list workflows: %v", err)
	}
	if len(workflows) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(workflows))
	}
}

// Task CRUD tests

func TestSQLiteRepository_TaskCRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create workspace and workflow for foreign keys
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	// Create task (workflow steps are managed by workflow repository)
	task := &models.Task{
		WorkspaceID:    "ws-1",
		WorkflowID:        "wf-123",
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

	// Create workspace, workflow, and task
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", State: v1.TaskStateTODO}
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

	// Create workspace and workflow
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Task 2"})

	tasks, err := repo.ListTasks(ctx, "wf-123")
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

	// Create workspace and workflow
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	// Tasks with different workflow steps
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-1", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-1", Title: "Task 2"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-3", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-2", Title: "Task 3"})

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

	// Create workspaces and workflow
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	// Create tasks in workspace 1
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-1", Title: "Task One"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-1", Title: "Task Two"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-3", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-1", Title: "Task Three"})
	// Create task in workspace 2
	workflow2 := &models.Workflow{ID: "wf-456", WorkspaceID: "ws-2", Name: "Test Workflow 2"}
	_ = repo.CreateWorkflow(ctx, workflow2)
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-4", WorkspaceID: "ws-2", WorkflowID: "wf-456", WorkflowStepID: "step-2", Title: "Task Four"})

	// Test basic listing without search
	tasks, total, err := repo.ListTasksByWorkspace(ctx,"ws-1", "", 1, 10, false)
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
	tasks, total, err = repo.ListTasksByWorkspace(ctx,"ws-1", "", 1, 2, false)
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
	tasksPage2, _, err := repo.ListTasksByWorkspace(ctx,"ws-1", "", 2, 2, false)
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

	// Create workspace, workflow, and repository
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	repository := &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "MyProject", LocalPath: "/home/user/projects/myproject"}
	_ = repo.CreateRepository(ctx, repository)

	// Create tasks with different titles and descriptions
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-1", Title: "Fix authentication bug", Description: "Users cannot login"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-1", Title: "Add new feature", Description: "Implement dark mode"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-3", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-1", Title: "Refactor codebase", Description: "Clean up authentication module"})

	// Link task-1 to the repository
	_ = repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main"})

	// Test search by title
	_, totalAuth, err := repo.ListTasksByWorkspace(ctx,"ws-1", "authentication", 1, 10, false)
	if err != nil {
		t.Fatalf("failed to search tasks by title: %v", err)
	}
	if totalAuth != 2 {
		t.Errorf("expected 2 tasks matching 'authentication', got %d", totalAuth)
	}

	// Test search by description
	tasksDarkMode, totalDarkMode, err := repo.ListTasksByWorkspace(ctx,"ws-1", "dark mode", 1, 10, false)
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
	tasksRepo, totalRepo, err := repo.ListTasksByWorkspace(ctx,"ws-1", "MyProject", 1, 10, false)
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
	_, totalPath, err := repo.ListTasksByWorkspace(ctx,"ws-1", "myproject", 1, 10, false)
	if err != nil {
		t.Fatalf("failed to search tasks by repository path: %v", err)
	}
	if totalPath != 1 {
		t.Errorf("expected 1 task matching repository path 'myproject', got %d", totalPath)
	}

	// Test search with no results
	tasksNone, totalNone, err := repo.ListTasksByWorkspace(ctx,"ws-1", "nonexistent", 1, 10, false)
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
	workflow := &models.Workflow{ID: "persist-wf", WorkspaceID: "ws-1", Name: "Persistent Workflow"}
	_ = repo1.CreateWorkflow(ctx, workflow)
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

	retrieved, err := repo2.GetWorkflow(ctx, "persist-wf")
	if err != nil {
		t.Fatalf("failed to get workflow after reopen: %v", err)
	}
	if retrieved.Name != "Persistent Workflow" {
		t.Errorf("expected name 'Persistent Workflow', got %s", retrieved.Name)
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

	// Create workflow first (required for foreign key constraints)
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	// Create a task first
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
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

	// Create workflow first (required for foreign key constraints)
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	// Create tasks
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
	_ = repo.CreateTask(ctx, task)
	task2 := &models.Task{ID: "task-456", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task 2"}
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

	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
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

	// Create workflow first (required for foreign key constraints)
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	// Create a task
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
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

	// Create workflow and task first (required for foreign key constraints)
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
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

	// Create workflow and task
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
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

	// Create workflow and tasks
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task1 := &models.Task{ID: "task-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Task 1"}
	_ = repo.CreateTask(ctx, task1)
	task2 := &models.Task{ID: "task-2", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Task 2"}
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

	// Create workflow and task
	workflow := &models.Workflow{ID: "wf-123", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-123", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"}
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
	workflow := &models.Workflow{ID: "wf-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Test Task"}
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

func TestSQLiteRepository_ArchiveTask(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	workflow := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Task One"})

	// Archive the task
	err := repo.ArchiveTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("failed to archive task: %v", err)
	}

	// Verify archived_at is set
	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if task.ArchivedAt == nil {
		t.Fatal("expected archived_at to be set")
	}

	// Archive again should fail
	err = repo.ArchiveTask(ctx, "task-1")
	// The repo method doesn't check for already archived, but it still succeeds (updates the timestamp)
	if err != nil {
		t.Fatalf("unexpected error archiving already-archived task: %v", err)
	}

	// Archive non-existent task should fail
	err = repo.ArchiveTask(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error when archiving nonexistent task")
	}
}

func TestSQLiteRepository_ArchiveTask_ExcludesFromListTasks(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	workflow := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Task One"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Task Two"})

	// Archive task-1
	_ = repo.ArchiveTask(ctx, "task-1")

	// ListTasks should exclude archived tasks
	tasks, err := repo.ListTasks(ctx, "wf-1")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task (excluding archived), got %d", len(tasks))
	}
	if tasks[0].ID != "task-2" {
		t.Errorf("expected task-2, got %s", tasks[0].ID)
	}

	// ListTasksByWorkflowStep should exclude archived tasks
	tasksByStep, err := repo.ListTasksByWorkflowStep(ctx, "step-1")
	if err != nil {
		t.Fatalf("failed to list tasks by step: %v", err)
	}
	if len(tasksByStep) != 1 {
		t.Errorf("expected 1 task by step (excluding archived), got %d", len(tasksByStep))
	}

	// CountTasksByWorkflow should exclude archived tasks
	count, err := repo.CountTasksByWorkflow(ctx, "wf-1")
	if err != nil {
		t.Fatalf("failed to count tasks: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// CountTasksByWorkflowStep should exclude archived tasks
	countByStep, err := repo.CountTasksByWorkflowStep(ctx, "step-1")
	if err != nil {
		t.Fatalf("failed to count tasks by step: %v", err)
	}
	if countByStep != 1 {
		t.Errorf("expected count 1, got %d", countByStep)
	}
}

func TestSQLiteRepository_ListTasksByWorkspace_IncludeArchived(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	workflow := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Active Task"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Archived Task"})
	_ = repo.ArchiveTask(ctx, "task-2")

	// Without includeArchived: should return only active task
	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", 1, 10, false)
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 total without archived, got %d", total)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Errorf("expected only task-1, got %v", tasks)
	}

	// With includeArchived: should return both tasks
	tasks, total, err = repo.ListTasksByWorkspace(ctx, "ws-1", "", 1, 10, true)
	if err != nil {
		t.Fatalf("failed to list tasks with archived: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 total with archived, got %d", total)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}

	// Search with includeArchived=false should filter archived
	_, total, err = repo.ListTasksByWorkspace(ctx, "ws-1", "Task", 1, 10, false)
	if err != nil {
		t.Fatalf("failed to search tasks: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 search result without archived, got %d", total)
	}

	// Search with includeArchived=true should include archived
	_, total, err = repo.ListTasksByWorkspace(ctx, "ws-1", "Task", 1, 10, true)
	if err != nil {
		t.Fatalf("failed to search tasks with archived: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 search results with archived, got %d", total)
	}
}

func TestSQLiteRepository_ListTasksForAutoArchive(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create the workflow_steps table (normally created by workflow repo)
	_, err := repo.DB().Exec(`
		CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			name TEXT NOT NULL,
			position INTEGER NOT NULL,
			color TEXT,
			prompt TEXT,
			events TEXT,
			allow_manual_move INTEGER DEFAULT 1,
			is_start_step INTEGER DEFAULT 0,
			auto_archive_after_hours INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create workflow_steps table: %v", err)
	}

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	workflow := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	now := time.Now().UTC()

	// Create workflow step with auto-archive after 1 hour
	_, err = repo.DB().Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position, auto_archive_after_hours, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "step-done", "wf-1", "Done", 2, 1, now, now)
	if err != nil {
		t.Fatalf("failed to create workflow step: %v", err)
	}

	// Create workflow step without auto-archive
	_, err = repo.DB().Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position, auto_archive_after_hours, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "step-todo", "wf-1", "Todo", 1, 0, now, now)
	if err != nil {
		t.Fatalf("failed to create workflow step: %v", err)
	}

	oldTime := now.Add(-2 * time.Hour)

	// Task in auto-archive step, updated 2 hours ago — should be eligible
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-old", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-done", Title: "Old Task",
	})
	// Backdate updated_at (CreateTask sets it to now)
	_, _ = repo.DB().Exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`, oldTime, "task-old")

	// Task in auto-archive step, updated just now — should NOT be eligible
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-recent", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-done", Title: "Recent Task",
	})

	// Task in non-auto-archive step — should NOT be eligible
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-todo", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-todo", Title: "Todo Task",
	})
	_, _ = repo.DB().Exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`, oldTime, "task-todo")

	// Already archived task — should NOT be eligible
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-archived", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-done", Title: "Already Archived",
	})
	_, _ = repo.DB().Exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`, oldTime, "task-archived")
	_ = repo.ArchiveTask(ctx, "task-archived")

	// List candidates
	candidates, err := repo.ListTasksForAutoArchive(ctx)
	if err != nil {
		t.Fatalf("failed to list auto-archive candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].ID != "task-old" {
		t.Errorf("expected task-old, got %s", candidates[0].ID)
	}
}
