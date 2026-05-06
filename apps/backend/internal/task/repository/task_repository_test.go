package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

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
		WorkflowID:     "wf-123",
		WorkflowStepID: "step-123",
		Title:          "Test Task",
		Description:    "A test task",
		State:          v1.TaskStateTODO,
		Priority:       "high",
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
	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "", 1, 10, false, false, false, false)
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
	tasks, total, err = repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "", 1, 2, false, false, false, false)
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
	tasksPage2, _, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "", 2, 2, false, false, false, false)
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
	_, totalAuth, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "authentication", 1, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to search tasks by title: %v", err)
	}
	if totalAuth != 2 {
		t.Errorf("expected 2 tasks matching 'authentication', got %d", totalAuth)
	}

	// Test search by description
	tasksDarkMode, totalDarkMode, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "dark mode", 1, 10, false, false, false, false)
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
	tasksRepo, totalRepo, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "MyProject", 1, 10, false, false, false, false)
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
	_, totalPath, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "myproject", 1, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to search tasks by repository path: %v", err)
	}
	if totalPath != 1 {
		t.Errorf("expected 1 task matching repository path 'myproject', got %d", totalPath)
	}

	// Test search with no results
	tasksNone, totalNone, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "nonexistent", 1, 10, false, false, false, false)
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

func TestSQLiteRepository_ListTasksByWorkspace_WorkflowFilter(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-a", WorkspaceID: "ws-1", Name: "Workflow A"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-b", WorkspaceID: "ws-1", Name: "Workflow B"})

	_ = repo.CreateTask(ctx, &models.Task{ID: "t-1", WorkspaceID: "ws-1", WorkflowID: "wf-a", WorkflowStepID: "s-1", Title: "Alpha task"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-2", WorkspaceID: "ws-1", WorkflowID: "wf-a", WorkflowStepID: "s-1", Title: "Beta task"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-3", WorkspaceID: "ws-1", WorkflowID: "wf-b", WorkflowStepID: "s-1", Title: "Gamma task"})

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "wf-a", "", "", 1, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2 for wf-a, got %d", total)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.WorkflowID != "wf-a" {
			t.Errorf("expected workflow wf-a, got %s", task.WorkflowID)
		}
	}
}

func TestSQLiteRepository_ListTasksByWorkspace_RepositoryFilter(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "Repo One", LocalPath: "/repo/one"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-2", WorkspaceID: "ws-1", Name: "Repo Two", LocalPath: "/repo/two"})

	_ = repo.CreateTask(ctx, &models.Task{ID: "t-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Title: "Task One"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-2", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Title: "Task Two"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-3", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Title: "Task Three"})
	_ = repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-1", TaskID: "t-1", RepositoryID: "repo-1"})
	_ = repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-2", TaskID: "t-2", RepositoryID: "repo-2"})

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "repo-1", "", 1, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1 for repo-1, got %d", total)
	}
	if len(tasks) != 1 || tasks[0].ID != "t-1" {
		t.Errorf("expected only t-1, got %v", tasks)
	}
}

func TestSQLiteRepository_ListTasksByWorkspace_WorkflowAndRepositoryFilter(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-a", WorkspaceID: "ws-1", Name: "Workflow A"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-b", WorkspaceID: "ws-1", Name: "Workflow B"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "Repo One", LocalPath: "/repo/one"})

	_ = repo.CreateTask(ctx, &models.Task{ID: "t-1", WorkspaceID: "ws-1", WorkflowID: "wf-a", WorkflowStepID: "s-1", Title: "Match task"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-2", WorkspaceID: "ws-1", WorkflowID: "wf-a", WorkflowStepID: "s-1", Title: "No repo task"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-3", WorkspaceID: "ws-1", WorkflowID: "wf-b", WorkflowStepID: "s-1", Title: "Wrong workflow"})
	_ = repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-1", TaskID: "t-1", RepositoryID: "repo-1"})
	_ = repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-3", TaskID: "t-3", RepositoryID: "repo-1"})

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "wf-a", "repo-1", "", 1, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1 (wf-a + repo-1), got %d", total)
	}
	if len(tasks) != 1 || tasks[0].ID != "t-1" {
		t.Errorf("expected only t-1, got %v", tasks)
	}
}

func TestSQLiteRepository_ListTasksByWorkspace_WorkflowFilterWithQuery(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-a", WorkspaceID: "ws-1", Name: "Workflow A"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-b", WorkspaceID: "ws-1", Name: "Workflow B"})

	_ = repo.CreateTask(ctx, &models.Task{ID: "t-1", WorkspaceID: "ws-1", WorkflowID: "wf-a", WorkflowStepID: "s-1", Title: "Fix login bug"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-2", WorkspaceID: "ws-1", WorkflowID: "wf-a", WorkflowStepID: "s-1", Title: "Fix payment bug"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-3", WorkspaceID: "ws-1", WorkflowID: "wf-b", WorkflowStepID: "s-1", Title: "Fix login bug"})

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "wf-a", "", "login", 1, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 result (wf-a + 'login'), got %d", total)
	}
	if len(tasks) != 1 || tasks[0].ID != "t-1" {
		t.Errorf("expected only t-1, got %v", tasks)
	}
}

func TestSQLiteRepository_ListTasksByWorkspace_RepositoryFilterWithQuery(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "Repo One", LocalPath: "/repo/one"})

	_ = repo.CreateTask(ctx, &models.Task{ID: "t-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Title: "Fix auth bug"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-2", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Title: "Fix auth elsewhere"})
	_ = repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-1", TaskID: "t-1", RepositoryID: "repo-1"})

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "repo-1", "auth", 1, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 result (repo-1 + 'auth'), got %d", total)
	}
	if len(tasks) != 1 || tasks[0].ID != "t-1" {
		t.Errorf("expected only t-1, got %v", tasks)
	}
}

func TestSQLiteRepository_ListTasksByWorkspace_DistinctWithMultipleRepos(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "Repo One", LocalPath: "/repo/one"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-2", WorkspaceID: "ws-1", Name: "Repo Two", LocalPath: "/repo/two"})

	// t-1 is linked to two repositories — must not appear twice in results
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Title: "Multi-repo task"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-2", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Title: "Single-repo task"})
	_ = repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-1", TaskID: "t-1", RepositoryID: "repo-1"})
	_ = repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-2", TaskID: "t-1", RepositoryID: "repo-2"})
	_ = repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-3", TaskID: "t-2", RepositoryID: "repo-1"})

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "task", 1, 10, false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2 distinct tasks, got %d", total)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 rows (no duplicates), got %d", len(tasks))
	}
	seen := make(map[string]int)
	for _, task := range tasks {
		seen[task.ID]++
	}
	if seen["t-1"] != 1 {
		t.Errorf("expected t-1 exactly once, appeared %d times", seen["t-1"])
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
	sqlxDB1 := sqlx.NewDb(dbConn1, "sqlite3")
	repo1, err := sqlite.NewWithDB(sqlxDB1, sqlxDB1)
	if err != nil {
		t.Fatalf("failed to create first repository: %v", err)
	}

	_ = repo1.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "persist-wf", WorkspaceID: "ws-1", Name: "Persistent Workflow"}
	_ = repo1.CreateWorkflow(ctx, workflow)
	if err := repo1.Close(); err != nil {
		t.Fatalf("failed to close repo: %v", err)
	}
	if err := sqlxDB1.Close(); err != nil {
		t.Fatalf("failed to close sqlite db: %v", err)
	}

	// Reopen repository and verify data persisted
	dbConn2, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("failed to open SQLite database: %v", err)
	}
	sqlxDB2 := sqlx.NewDb(dbConn2, "sqlite3")
	repo2, err := sqlite.NewWithDB(sqlxDB2, sqlxDB2)
	if err != nil {
		t.Fatalf("failed to create second repository: %v", err)
	}
	defer func() {
		if err := sqlxDB2.Close(); err != nil {
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
