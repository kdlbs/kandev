package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskWorkspaceFoldersSchemaPersistsCanonicalFolders(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "task-folders", "workspace-folders", "Folders", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO task_workspace_folders
			(id, task_id, local_path, display_name, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "folder-1", "task-folders", "/canonical/support", "support", 0, now, now); err != nil {
		t.Fatalf("insert workspace folder: %v", err)
	}

	var path, name string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT local_path, display_name FROM task_workspace_folders WHERE id = ?
	`, "folder-1").Scan(&path, &name); err != nil {
		t.Fatalf("read persisted workspace folder: %v", err)
	}
	if path != "/canonical/support" || name != "support" {
		t.Fatalf("persisted folder = (%q, %q), want canonical path and display name", path, name)
	}
}

func TestListTaskWorkspaceFoldersByTaskIDsGroupsFolders(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, taskID := range []string{"task-folders-a", "task-folders-b"} {
		if _, err := repo.DB().ExecContext(ctx, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, taskID, "workspace-folders", taskID, now, now); err != nil {
			t.Fatalf("seed task %s: %v", taskID, err)
		}
	}
	for _, folder := range []struct {
		id, taskID, name string
		position         int
	}{{"folder-a", "task-folders-a", "docs", 1}, {"folder-b", "task-folders-b", "support", 0}} {
		if _, err := repo.DB().ExecContext(ctx, `INSERT INTO task_workspace_folders (id, task_id, local_path, display_name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, folder.id, folder.taskID, "/canonical/"+folder.name, folder.name, folder.position, now, now); err != nil {
			t.Fatalf("seed folder %s: %v", folder.id, err)
		}
	}
	folders, err := repo.ListTaskWorkspaceFoldersByTaskIDs(ctx, []string{"task-folders-a", "task-folders-b"})
	if err != nil {
		t.Fatalf("ListTaskWorkspaceFoldersByTaskIDs: %v", err)
	}
	if len(folders["task-folders-a"]) != 1 || folders["task-folders-a"][0].DisplayName != "docs" || len(folders["task-folders-b"]) != 1 || folders["task-folders-b"][0].DisplayName != "support" {
		t.Fatalf("grouped folders = %#v", folders)
	}
}

func TestCreateWorkspaceSourceBatchAllocatesMixedPositionsAndCompensates(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-sources")
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repository-existing", WorkspaceID: "workspace-sources", Name: "existing",
	}); err != nil {
		t.Fatalf("seed existing repository: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repository-new", WorkspaceID: "workspace-sources", Name: "new",
	}); err != nil {
		t.Fatalf("seed new repository: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-sources", WorkspaceID: "workspace-sources", Title: "Sources", Priority: "medium",
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repository-existing", TaskID: "task-sources", RepositoryID: "repository-existing", BaseBranch: "main", Position: 3,
	}); err != nil {
		t.Fatalf("seed task repository: %v", err)
	}

	batch := &models.WorkspaceSourceBatch{
		TaskID: "task-sources",
		Sources: []models.WorkspaceSource{
			{Repository: &models.TaskRepository{RepositoryID: "repository-new", BaseBranch: "main"}},
			{Folder: &models.TaskWorkspaceFolder{LocalPath: "/canonical/docs", DisplayName: "docs"}},
		},
	}
	if err := repo.CreateWorkspaceSourceBatch(ctx, batch); err != nil {
		t.Fatalf("create workspace source batch: %v", err)
	}
	if batch.Sources[0].Repository.Position != 4 || batch.Sources[1].Folder.Position != 5 {
		t.Fatalf("allocated positions = repository %d, folder %d; want 4, 5", batch.Sources[0].Repository.Position, batch.Sources[1].Folder.Position)
	}
	if err := repo.CompensateWorkspaceSourceBatch(ctx, batch); err != nil {
		t.Fatalf("compensate workspace source batch: %v", err)
	}
	folders, err := repo.ListTaskWorkspaceFolders(ctx, "task-sources")
	if err != nil {
		t.Fatalf("list folders after compensation: %v", err)
	}
	if len(folders) != 0 {
		t.Fatalf("folders after compensation = %#v, want none", folders)
	}
	repos, err := repo.ListTaskRepositories(ctx, "task-sources")
	if err != nil {
		t.Fatalf("list repositories after compensation: %v", err)
	}
	if len(repos) != 1 || repos[0].ID != "task-repository-existing" {
		t.Fatalf("repositories after compensation = %#v, want only existing source", repos)
	}
}

func TestTaskWorkspaceFolderRejectsDuplicatePathOrDisplayNameAndCascades(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-folder-constraints")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-folder-constraints", WorkspaceID: "workspace-folder-constraints", Title: "Folders", Priority: "medium",
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{TaskID: "task-folder-constraints", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{
		LocalPath: "/canonical/docs", DisplayName: "docs",
	}}}}); err != nil {
		t.Fatalf("create first folder: %v", err)
	}
	for _, folder := range []*models.TaskWorkspaceFolder{
		{TaskID: "task-folder-constraints", LocalPath: "/canonical/docs", DisplayName: "other"},
		{TaskID: "task-folder-constraints", LocalPath: "/canonical/other", DisplayName: "docs"},
	} {
		if err := repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{TaskID: folder.TaskID, Sources: []models.WorkspaceSource{{Folder: folder}}}); err == nil {
			t.Fatalf("CreateWorkspaceSourceBatch(%q, %q) succeeded, want uniqueness error", folder.LocalPath, folder.DisplayName)
		}
	}
	if err := repo.DeleteTask(ctx, "task-folder-constraints"); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var count int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM task_workspace_folders WHERE task_id = ?`, "task-folder-constraints").Scan(&count); err != nil {
		t.Fatalf("count cascaded folders: %v", err)
	}
	if count != 0 {
		t.Fatalf("folders after task delete = %d, want cascade deletion", count)
	}
}

func TestTaskWorkspaceFoldersMigrationReplaysAfterLegacyTableIsMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	if _, err := repo.DB().Exec(`DROP TABLE task_workspace_folders`); err != nil {
		t.Fatalf("simulate legacy schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay workspace-folder migration: %v", err)
	}
	var tableName string
	if err := repo.DB().QueryRow(`
		SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'task_workspace_folders'
	`).Scan(&tableName); err != nil {
		t.Fatalf("workspace-folder table after migration: %v", err)
	}
}
