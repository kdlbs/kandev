package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// TestGetTaskByExternalID covers the REST lookup route's service-level
// behavior: found (including unsettled), not found, and validation ordering
// (invalid external_id must not leak whether a workspace exists via a
// different status than "not found" -- validation happens after auth but
// this test focuses on the found/not-found/invalid trichotomy).
func TestGetTaskByExternalID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-1", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	found, err := svc.GetTaskByExternalID(ctx, "ws-1", "ext-1")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if found.ID != task.ID {
		t.Fatalf("found.ID = %s, want %s", found.ID, task.ID)
	}
	if found.ExternalIDSettledAt != nil {
		t.Fatal("unsettled task must still be returned by lookup")
	}

	if _, err := svc.GetTaskByExternalID(ctx, "ws-1", "ext-missing"); !errors.Is(err, taskrepo.ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}

	if _, err := svc.GetTaskByExternalID(ctx, "ws-1", "ext-1\n"); err == nil {
		t.Fatal("expected a validation error for a control character in external_id")
	}
}

// TestGetTaskByExternalIDReturnsArchivedTasks pins "including archived" from
// the spec's lookup table.
func TestGetTaskByExternalIDReturnsArchivedTasks(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-1", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.ArchiveTask(ctx, task.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	found, err := svc.GetTaskByExternalID(ctx, "ws-1", "ext-1")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if found.ArchivedAt == nil {
		t.Fatal("archived task should be returned with archived_at set, not auto-unarchived")
	}
}

// TestReleaseTaskExternalID covers the REST release route's service-level
// behavior: success frees the identity without deleting the task, and
// releasing an identity nothing holds reports false rather than erroring.
func TestReleaseTaskExternalID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-1", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	released, err := svc.ReleaseTaskExternalID(ctx, "ws-1", "ext-1")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if !released {
		t.Fatal("release should report the identity was held")
	}

	reloaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("task must still exist after release: %v", err)
	}
	if reloaded.ExternalID != "" {
		t.Fatalf("reloaded.ExternalID = %q, want empty", reloaded.ExternalID)
	}

	again, err := svc.ReleaseTaskExternalID(ctx, "ws-1", "ext-1")
	if err != nil {
		t.Fatalf("second release: %v", err)
	}
	if again {
		t.Fatal("releasing an identity nothing holds should report false, not error")
	}

	if _, err := svc.ReleaseTaskExternalID(ctx, "ws-1", "ext-1\n"); err == nil {
		t.Fatal("expected a validation error for a control character in external_id")
	}
}

// TestReleaseTaskExternalID_PublishesTaskUpdated covers the release-round
// human-review finding: releasing an identity is a task mutation like any
// other and must publish task.updated (apps/backend/AGENTS.md, "Task
// lifecycle events"), or WS-connected clients retain a stale copy of the
// task's external_id.
func TestReleaseTaskExternalID_PublishesTaskUpdated(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-1", Title: "Task", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	eventBus.ClearEvents()

	released, err := svc.ReleaseTaskExternalID(ctx, "ws-1", "ext-1")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if !released {
		t.Fatal("release should report the identity was held")
	}

	events := eventBus.GetPublishedEvents()
	var updated int
	for _, e := range events {
		if e.Type == "task.updated" {
			updated++
		}
	}
	if updated != 1 {
		t.Fatalf("task.updated events published = %d, want exactly 1; got %d total events of any type", updated, len(events))
	}
	data := singlePublishedEventData(t, eventBus)
	if got, ok := data["task_id"].(string); !ok || got != task.ID {
		t.Fatalf("published event task_id = %#v, want %q", data["task_id"], task.ID)
	}
	if _, present := data["external_id"]; present {
		t.Fatalf("published event external_id should be omitted after release, got %#v", data["external_id"])
	}
}

// TestGetTaskByExternalID_HydratesRepositoriesAndWorkspaceFolders covers the
// human-review finding that the lookup route returned an incomplete task
// DTO: GetTaskByExternalID must hydrate the same relations a normal GetTask
// call does, or a caller polling the lookup route for a task with
// repositories/workspace folders sees them silently disappear.
func TestGetTaskByExternalID_HydratesRepositoriesAndWorkspaceFolders(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-hydrate", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-hydrate", WorkspaceID: "ws-hydrate", Name: "hydrate-repo", SourceType: sourceTypeLocal, LocalPath: "/tmp/hydrate-repo",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-hydrate", Title: "Task", ExternalID: "ext-hydrate"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-hydrate", TaskID: task.ID, RepositoryID: "repo-hydrate",
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	if err := repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{
		TaskID:  task.ID,
		Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{LocalPath: "/tmp/hydrate-folder", DisplayName: "hydrate-folder"}}},
	}); err != nil {
		t.Fatalf("create workspace source batch: %v", err)
	}

	found, err := svc.GetTaskByExternalID(ctx, "ws-hydrate", "ext-hydrate")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(found.Repositories) != 1 || found.Repositories[0].RepositoryID != "repo-hydrate" {
		t.Fatalf("found.Repositories = %#v, want one repository linking repo-hydrate — the lookup route must hydrate the same relations GetTask does", found.Repositories)
	}
	if len(found.WorkspaceFolders) != 1 || found.WorkspaceFolders[0].DisplayName != "hydrate-folder" {
		t.Fatalf("found.WorkspaceFolders = %#v, want one folder named hydrate-folder", found.WorkspaceFolders)
	}
}

// TestCreateTaskFoundOutcome_HydratesRepositoriesAndWorkspaceFolders covers
// the same human-review finding on the retry path: a create that resolves
// to a Found outcome via the step-3 lookup must return the same hydrated
// relations a fresh GetTask would, not a bare repository-layer task struct.
func TestCreateTaskFoundOutcome_HydratesRepositoriesAndWorkspaceFolders(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-retry-hydrate", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-retry-hydrate", WorkspaceID: "ws-retry-hydrate", Name: "retry-hydrate-repo", SourceType: sourceTypeLocal, LocalPath: "/tmp/retry-hydrate-repo",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	task := &models.Task{WorkspaceID: "ws-retry-hydrate", Title: "Task", ExternalID: "ext-retry-hydrate"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.SettleTaskExternalID(ctx, task.ID, "ext-retry-hydrate", time.Now().UTC()); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-retry-hydrate", TaskID: task.ID, RepositoryID: "repo-retry-hydrate",
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	if err := repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{
		TaskID:  task.ID,
		Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{LocalPath: "/tmp/retry-hydrate-folder", DisplayName: "retry-hydrate-folder"}}},
	}); err != nil {
		t.Fatalf("create workspace source batch: %v", err)
	}

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-retry-hydrate", Title: "Retry", ExternalID: "ext-retry-hydrate",
	})
	if err != nil {
		t.Fatalf("retry create: %v", err)
	}
	if result.Outcome != CreateTaskOutcomeFoundSettled {
		t.Fatalf("Outcome = %v, want FoundSettled", result.Outcome)
	}
	if len(result.Task.Repositories) != 1 || result.Task.Repositories[0].RepositoryID != "repo-retry-hydrate" {
		t.Fatalf("result.Task.Repositories = %#v, want one repository linking repo-retry-hydrate — a Found outcome must hydrate the same relations GetTask does", result.Task.Repositories)
	}
	if len(result.Task.WorkspaceFolders) != 1 || result.Task.WorkspaceFolders[0].DisplayName != "retry-hydrate-folder" {
		t.Fatalf("result.Task.WorkspaceFolders = %#v, want one folder named retry-hydrate-folder", result.Task.WorkspaceFolders)
	}
}
