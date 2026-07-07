package repository

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	usermodels "github.com/kandev/kandev/internal/user/models"
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

func TestSQLiteRepository_UpdateTaskStateIfCurrentIn(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", State: v1.TaskStateInProgress}
	_ = repo.CreateTask(ctx, task)

	oldState, updated, err := repo.UpdateTaskStateIfCurrentIn(
		ctx, "task-123", v1.TaskStateWaitingForInput,
		[]v1.TaskState{v1.TaskStateInProgress, v1.TaskStateScheduling},
	)
	if err != nil {
		t.Fatalf("conditional update failed: %v", err)
	}
	if !updated {
		t.Fatal("expected update from IN_PROGRESS")
	}
	if oldState != v1.TaskStateInProgress {
		t.Fatalf("old state = %q, want IN_PROGRESS", oldState)
	}

	retrieved, _ := repo.GetTask(ctx, "task-123")
	if retrieved.State != v1.TaskStateWaitingForInput {
		t.Fatalf("expected WAITING_FOR_INPUT, got %s", retrieved.State)
	}

	_, updated, err = repo.UpdateTaskStateIfCurrentIn(
		ctx, "task-123", v1.TaskStateWaitingForInput,
		[]v1.TaskState{v1.TaskStateInProgress, v1.TaskStateScheduling},
	)
	if err != nil {
		t.Fatalf("second conditional update failed: %v", err)
	}
	if updated {
		t.Fatal("expected no update when current state is not allowed")
	}

	_, updated, err = repo.UpdateTaskStateIfCurrentIn(ctx, "missing", v1.TaskStateReview, []v1.TaskState{v1.TaskStateInProgress})
	if err == nil {
		t.Fatal("expected error for missing task")
	}
	if updated {
		t.Fatal("expected no update for missing task")
	}
}

func TestSQLiteRepository_ListExpiredQuickChatTasks(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-quick", Name: "Quick"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-quick", WorkspaceID: "ws-quick", Name: "Workflow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-7 * 24 * time.Hour)
	old := cutoff.Add(-time.Hour)
	older := cutoff.Add(-2 * time.Hour)
	recent := cutoff.Add(time.Hour)

	cases := []struct {
		id             string
		taskUpdated    time.Time
		sessionUpdated time.Time
		sessionState   models.TaskSessionState
		ephemeral      bool
		workflowID     string
		archived       bool
		origin         string
		metadata       map[string]interface{}
		wantExpired    bool
	}{
		{id: "expired-older", taskUpdated: older, sessionUpdated: older, sessionState: models.TaskSessionStateCompleted, ephemeral: true, wantExpired: true},
		{id: "expired-old", taskUpdated: old, sessionUpdated: old, sessionState: models.TaskSessionStateCompleted, ephemeral: true, wantExpired: true},
		{id: "expired-waiting-input", taskUpdated: old, sessionUpdated: old, sessionState: models.TaskSessionStateWaitingForInput, ephemeral: true, wantExpired: true},
		{id: "recent-task", taskUpdated: recent, sessionUpdated: old, sessionState: models.TaskSessionStateCompleted, ephemeral: true},
		{id: "recent-session", taskUpdated: old, sessionUpdated: recent, sessionState: models.TaskSessionStateCompleted, ephemeral: true},
		{id: "running-session", taskUpdated: older, sessionUpdated: older, sessionState: models.TaskSessionStateRunning, ephemeral: true},
		{id: "idle-session", taskUpdated: older, sessionUpdated: older, sessionState: models.TaskSessionStateIdle, ephemeral: true},
		{id: "config-mode", taskUpdated: older, sessionUpdated: older, sessionState: models.TaskSessionStateCompleted, ephemeral: true, metadata: map[string]interface{}{"config_mode": true}},
		{id: "automation-run", taskUpdated: older, sessionUpdated: older, sessionState: models.TaskSessionStateCompleted, ephemeral: true, origin: models.TaskOriginAutomationRun},
		{id: "kanban-task", taskUpdated: older, sessionUpdated: older, sessionState: models.TaskSessionStateCompleted, ephemeral: false, workflowID: "wf-quick"},
		{id: "ephemeral-workflow", taskUpdated: older, sessionUpdated: older, sessionState: models.TaskSessionStateCompleted, ephemeral: true, workflowID: "wf-quick"},
		{id: "archived", taskUpdated: older, sessionUpdated: older, sessionState: models.TaskSessionStateCompleted, ephemeral: true, archived: true},
	}

	for _, tc := range cases {
		createQuickChatExpiryFixture(t, repo, ctx, tc.id, tc.workflowID, tc.origin, tc.metadata, tc.ephemeral, tc.archived, tc.taskUpdated, tc.sessionUpdated, tc.sessionState)
	}

	tasks, err := repo.ListExpiredQuickChatTasks(ctx, cutoff)
	if err != nil {
		t.Fatalf("ListExpiredQuickChatTasks: %v", err)
	}

	var got []string
	for _, task := range tasks {
		got = append(got, task.ID)
	}
	var want []string
	for _, tc := range cases {
		if tc.wantExpired {
			want = append(want, tc.id)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expired task IDs = %v, want %v", got, want)
	}
}

func createQuickChatExpiryFixture(
	t *testing.T,
	repo *sqlite.Repository,
	ctx context.Context,
	id string,
	workflowID string,
	origin string,
	metadata map[string]interface{},
	ephemeral bool,
	archived bool,
	taskUpdated time.Time,
	sessionUpdated time.Time,
	sessionState models.TaskSessionState,
) {
	t.Helper()
	task := &models.Task{
		ID:          id,
		WorkspaceID: "ws-quick",
		WorkflowID:  workflowID,
		Title:       id,
		State:       v1.TaskStateTODO,
		Priority:    "medium",
		IsEphemeral: ephemeral,
		Origin:      origin,
		Metadata:    metadata,
		CreatedAt:   taskUpdated.Add(-time.Hour),
		UpdatedAt:   taskUpdated,
	}
	if archived {
		archivedAt := taskUpdated.Add(time.Minute)
		task.ArchivedAt = &archivedAt
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask(%s): %v", id, err)
	}
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE tasks SET created_at = ?, updated_at = ?, archived_at = ? WHERE id = ?`,
		task.CreatedAt, taskUpdated, task.ArchivedAt, id,
	); err != nil {
		t.Fatalf("backdate task(%s): %v", id, err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:             id + "-session",
		TaskID:         id,
		AgentProfileID: "agent-1",
		State:          sessionState,
		StartedAt:      sessionUpdated.Add(-time.Hour),
		UpdatedAt:      sessionUpdated,
		IsPrimary:      true,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", id, err)
	}
}

func TestSQLiteRepository_ListChildCompletionRows_ActiveChildren(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"})

	now := time.Now().UTC()
	parent := &models.Task{
		ID:             "parent-1",
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Parent",
		State:          v1.TaskStateInProgress,
		Priority:       "medium",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateTask(ctx, parent); err != nil {
		t.Fatalf("create parent: %v", err)
	}

	createChild := func(id string, state v1.TaskState, archived, ephemeral bool) {
		t.Helper()
		child := &models.Task{
			ID:             id,
			WorkspaceID:    "ws-1",
			WorkflowID:     "wf-1",
			WorkflowStepID: "step-1",
			Title:          id,
			State:          state,
			Priority:       "medium",
			ParentID:       parent.ID,
			IsEphemeral:    ephemeral,
			CreatedAt:      now.Add(time.Duration(len(id)) * time.Second),
			UpdatedAt:      now.Add(time.Duration(len(id)) * time.Minute),
		}
		if err := repo.CreateTask(ctx, child); err != nil {
			t.Fatalf("create child %s: %v", id, err)
		}
		if archived {
			if err := repo.ArchiveTask(ctx, id); err != nil {
				t.Fatalf("archive child %s: %v", id, err)
			}
		}
	}

	createChild("child-completed", v1.TaskStateCompleted, false, false)
	createChild("child-failed", v1.TaskStateFailed, false, false)
	createChild("child-cancelled", v1.TaskStateCancelled, false, false)
	createChild("child-open", v1.TaskStateInProgress, false, false)
	createChild("child-archived", v1.TaskStateTODO, true, false)
	createChild("child-ephemeral", v1.TaskStateTODO, false, true)

	rows, err := repo.ListChildCompletionRows(ctx, parent.ID)
	if err != nil {
		t.Fatalf("ListChildCompletionRows: %v", err)
	}
	gotIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		gotIDs = append(gotIDs, row.ID)
		if row.Title == "" {
			t.Errorf("row %s missing title", row.ID)
		}
		if row.UpdatedAt.IsZero() {
			t.Errorf("row %s missing updated_at", row.ID)
		}
	}
	wantIDs := []string{"child-cancelled", "child-completed", "child-failed", "child-open"}
	slices.Sort(gotIDs)
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("active child rows = %v, want %v", gotIDs, wantIDs)
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

func TestSQLiteRepository_GetTasksByIDs(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		_ = repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: id})
	}

	// Empty input returns no tasks and no error.
	none, err := repo.GetTasksByIDs(ctx, nil)
	if err != nil {
		t.Fatalf("GetTasksByIDs(nil): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 tasks for empty input, got %d", len(none))
	}

	// Existing + missing IDs: only the existing ones come back.
	got, err := repo.GetTasksByIDs(ctx, []string{"task-1", "task-3", "missing"})
	if err != nil {
		t.Fatalf("GetTasksByIDs: %v", err)
	}
	ids := map[string]bool{}
	for _, tk := range got {
		ids[tk.ID] = true
	}
	if len(got) != 2 || !ids["task-1"] || !ids["task-3"] {
		t.Errorf("expected [task-1 task-3], got %v", ids)
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
	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "", 1, 10, "", false, false, false, false)
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
	tasks, total, err = repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "", 1, 2, "", false, false, false, false)
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
	tasksPage2, _, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "", 2, 2, "", false, false, false, false)
	if err != nil {
		t.Fatalf("failed to list tasks page 2: %v", err)
	}
	if len(tasksPage2) != 1 {
		t.Errorf("expected 1 task on page 2, got %d", len(tasksPage2))
	}
}

func TestSQLiteRepository_ListTasksByWorkspace_SortBeforePagination(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-c", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Title: "Charlie"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-a", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Title: "Alpha"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "t-b", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Title: "Bravo"})

	page1, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "", 1, 2, usermodels.TasksListSortTitleAsc, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to list tasks by workspace: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total 3 tasks, got %d", total)
	}
	if len(page1) != 2 {
		t.Fatalf("page 1 length = %d, want 2", len(page1))
	}
	if got := []string{page1[0].ID, page1[1].ID}; !slices.Equal(got, []string{"t-a", "t-b"}) {
		t.Fatalf("page 1 IDs = %v, want [t-a t-b]", got)
	}

	page2, _, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "", 2, 2, usermodels.TasksListSortTitleAsc, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to list tasks page 2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page 2 length = %d, want 1", len(page2))
	}
	if got := []string{page2[0].ID}; !slices.Equal(got, []string{"t-c"}) {
		t.Fatalf("page 2 IDs = %v, want [t-c]", got)
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
	_, totalAuth, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "authentication", 1, 10, "", false, false, false, false)
	if err != nil {
		t.Fatalf("failed to search tasks by title: %v", err)
	}
	if totalAuth != 2 {
		t.Errorf("expected 2 tasks matching 'authentication', got %d", totalAuth)
	}

	// Test search by description
	tasksDarkMode, totalDarkMode, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "dark mode", 1, 10, "", false, false, false, false)
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
	tasksRepo, totalRepo, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "MyProject", 1, 10, "", false, false, false, false)
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
	_, totalPath, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "myproject", 1, 10, "", false, false, false, false)
	if err != nil {
		t.Fatalf("failed to search tasks by repository path: %v", err)
	}
	if totalPath != 1 {
		t.Errorf("expected 1 task matching repository path 'myproject', got %d", totalPath)
	}

	// Test search with no results
	tasksNone, totalNone, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "nonexistent", 1, 10, "", false, false, false, false)
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

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "wf-a", "", "", 1, 10, "", false, false, false, false)
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

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "repo-1", "", 1, 10, "", false, false, false, false)
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

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "wf-a", "repo-1", "", 1, 10, "", false, false, false, false)
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

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "wf-a", "", "login", 1, 10, "", false, false, false, false)
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

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "repo-1", "auth", 1, 10, "", false, false, false, false)
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

	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", "", "task", 1, 10, "", false, false, false, false)
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
	repo1, err := sqlite.NewWithDB(sqlxDB1, sqlxDB1, nil)
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
	repo2, err := sqlite.NewWithDB(sqlxDB2, sqlxDB2, nil)
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
