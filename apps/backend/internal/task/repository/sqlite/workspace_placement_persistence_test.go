package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkspacePlacementColumnsExistOnFreshSchema(t *testing.T) {
	repo := newRepoForEntityTests(t)

	columns := []struct {
		table  string
		column string
	}{
		{table: "tasks", column: "initial_workspace_layout"},
		{table: "task_repositories", column: "workspace_relative_path"},
		{table: "task_environments", column: "workspace_layout"},
		{table: "task_environment_repos", column: "workspace_relative_path"},
	}
	for _, column := range columns {
		rows, err := repo.db.QueryxContext(context.Background(), "SELECT "+column.column+" FROM "+column.table+" LIMIT 0")
		if err != nil {
			t.Fatalf("select %s.%s: %v", column.table, column.column, err)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close %s.%s rows: %v", column.table, column.column, err)
		}
	}
}

func TestWorkspacePlacementPersistenceRoundTripsThroughCRUD(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	workspaceID := "ws-workspace-placement"
	seedWorkspace(t, repo, workspaceID)

	task := &models.Task{
		ID:                     "task-workspace-placement",
		WorkspaceID:            workspaceID,
		Title:                  "Workspace placement",
		InitialWorkspaceLayout: "task_root",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	persistedTask, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if persistedTask.InitialWorkspaceLayout != "task_root" {
		t.Fatalf("task initial workspace layout = %q, want task_root", persistedTask.InitialWorkspaceLayout)
	}
	persistedTask.Title = "Workspace placement updated"
	persistedTask.InitialWorkspaceLayout = ""
	if err := repo.UpdateTask(ctx, persistedTask); err != nil {
		t.Fatalf("update task: %v", err)
	}
	persistedTask, err = repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get updated task: %v", err)
	}
	if persistedTask.InitialWorkspaceLayout != "task_root" {
		t.Fatalf("task initial workspace layout after update = %q, want task_root", persistedTask.InitialWorkspaceLayout)
	}

	if err := repo.CreateRepository(ctx, &models.Repository{
		ID:          "repo-workspace-placement",
		WorkspaceID: workspaceID,
		Name:        "placement-repo",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	taskRepo := &models.TaskRepository{
		ID:                    "task-repo-workspace-placement",
		TaskID:                task.ID,
		RepositoryID:          "repo-workspace-placement",
		WorkspaceRelativePath: "services/api",
		BaseBranch:            "main",
	}
	if err := repo.CreateTaskRepository(ctx, taskRepo); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	persistedTaskRepo, err := repo.GetTaskRepository(ctx, taskRepo.ID)
	if err != nil {
		t.Fatalf("get task repository: %v", err)
	}
	if persistedTaskRepo.WorkspaceRelativePath != taskRepo.WorkspaceRelativePath {
		t.Fatalf("task repository workspace relative path = %q, want %q", persistedTaskRepo.WorkspaceRelativePath, taskRepo.WorkspaceRelativePath)
	}
	listedTaskRepos, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatalf("list task repositories: %v", err)
	}
	if len(listedTaskRepos) != 1 || listedTaskRepos[0].WorkspaceRelativePath != taskRepo.WorkspaceRelativePath {
		t.Fatalf("listed task repositories = %#v, want one row with path %q", listedTaskRepos, taskRepo.WorkspaceRelativePath)
	}
	listedByTask, err := repo.ListTaskRepositoriesByTaskIDs(ctx, []string{task.ID})
	if err != nil {
		t.Fatalf("list task repositories by task IDs: %v", err)
	}
	if len(listedByTask[task.ID]) != 1 || listedByTask[task.ID][0].WorkspaceRelativePath != taskRepo.WorkspaceRelativePath {
		t.Fatalf("batched task repositories = %#v, want one row with path %q", listedByTask[task.ID], taskRepo.WorkspaceRelativePath)
	}
	taskRepo.WorkspaceRelativePath = "apps/api"
	if err := repo.UpdateTaskRepository(ctx, taskRepo); err != nil {
		t.Fatalf("update task repository: %v", err)
	}
	persistedTaskRepo, err = repo.GetTaskRepository(ctx, taskRepo.ID)
	if err != nil {
		t.Fatalf("get updated task repository: %v", err)
	}
	if persistedTaskRepo.WorkspaceRelativePath != "apps/api" {
		t.Fatalf("updated task repository workspace relative path = %q, want apps/api", persistedTaskRepo.WorkspaceRelativePath)
	}

	env := &models.TaskEnvironment{
		ID:              "env-workspace-placement",
		TaskID:          task.ID,
		ExecutorType:    string(models.ExecutorTypeLocal),
		Status:          models.TaskEnvironmentStatusCreating,
		WorkspacePath:   "/workspace/task",
		WorkspaceLayout: "task_root",
		Repos: []*models.TaskEnvironmentRepo{{
			ID:                    "env-repo-workspace-placement",
			RepositoryID:          taskRepo.RepositoryID,
			WorkspaceRelativePath: "apps/api",
		}},
	}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-workspace-placement",
		TaskID:            task.ID,
		TaskEnvironmentID: env.ID,
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	sessionRepos, err := repo.ListTaskSessionWorktrees(ctx, "session-workspace-placement")
	if err != nil {
		t.Fatalf("list task session worktrees: %v", err)
	}
	if len(sessionRepos) != 1 || sessionRepos[0].WorkspaceRelativePath != "apps/api" {
		t.Fatalf("task session worktrees = %#v, want one row with path apps/api", sessionRepos)
	}
	sessionReposByID, err := repo.ListWorktreesBySessionIDs(ctx, []string{"session-workspace-placement"})
	if err != nil {
		t.Fatalf("list worktrees by session IDs: %v", err)
	}
	if len(sessionReposByID["session-workspace-placement"]) != 1 ||
		sessionReposByID["session-workspace-placement"][0].WorkspaceRelativePath != "apps/api" {
		t.Fatalf("batched task session worktrees = %#v, want one row with path apps/api", sessionReposByID["session-workspace-placement"])
	}
	persistedEnv, err := repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if persistedEnv.WorkspaceLayout != "task_root" || len(persistedEnv.Repos) != 1 ||
		persistedEnv.Repos[0].WorkspaceRelativePath != "apps/api" {
		t.Fatalf("persisted task environment = %#v, want task_root and repo path apps/api", persistedEnv)
	}
	persistedEnv.WorkspaceLayout = ""
	if err := repo.UpdateTaskEnvironment(ctx, persistedEnv); err != nil {
		t.Fatalf("update task environment with omitted layout: %v", err)
	}
	persistedEnv, err = repo.GetTaskEnvironmentByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task environment after omitted layout: %v", err)
	}
	if persistedEnv.WorkspaceLayout != "task_root" {
		t.Fatalf("omitted workspace layout cleared persisted value: %q", persistedEnv.WorkspaceLayout)
	}
	persistedEnv.WorkspaceLayout = "repository"
	if err := repo.UpdateTaskEnvironment(ctx, persistedEnv); err != nil {
		t.Fatalf("update task environment: %v", err)
	}
	persistedEnv, err = repo.GetTaskEnvironmentByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task environment by task: %v", err)
	}
	if persistedEnv.WorkspaceLayout != "repository" {
		t.Fatalf("updated task environment workspace layout = %q, want repository", persistedEnv.WorkspaceLayout)
	}
	persistedEnv.Repos[0].WorkspaceRelativePath = "services/api"
	if err := repo.UpdateTaskEnvironmentRepo(ctx, persistedEnv.Repos[0]); err != nil {
		t.Fatalf("update task environment repository: %v", err)
	}
	persistedEnv, err = repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("get updated task environment: %v", err)
	}
	if persistedEnv.Repos[0].WorkspaceRelativePath != "services/api" {
		t.Fatalf("updated task environment repository workspace relative path = %q, want services/api", persistedEnv.Repos[0].WorkspaceRelativePath)
	}
}

func TestWorkspacePlacementLegacyRowsDefaultEmptyAfterMigrationReplay(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	workspaceID := "ws-workspace-placement-legacy"
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID:          "repo-workspace-placement-legacy",
		WorkspaceID: workspaceID,
		Name:        "legacy-repo",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	now := time.Now().UTC()
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "task-workspace-placement-legacy", workspaceID, "Legacy task", now, now); err != nil {
		t.Fatalf("insert legacy task: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "task-repo-workspace-placement-legacy", "task-workspace-placement-legacy", "repo-workspace-placement-legacy", now, now); err != nil {
		t.Fatalf("insert legacy task repository: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_environments (id, task_id, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, "env-workspace-placement-legacy", "task-workspace-placement-legacy", now, now); err != nil {
		t.Fatalf("insert legacy task environment: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_environment_repos (id, task_environment_id, repository_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "env-repo-workspace-placement-legacy", "env-workspace-placement-legacy", "repo-workspace-placement-legacy", now, now); err != nil {
		t.Fatalf("insert legacy task environment repository: %v", err)
	}

	assertWorkspacePlacementLegacyDefaults(t, repo)
	for _, statement := range []string{
		`ALTER TABLE tasks DROP COLUMN initial_workspace_layout`,
		`ALTER TABLE task_repositories DROP COLUMN workspace_relative_path`,
		`ALTER TABLE task_environments DROP COLUMN workspace_layout`,
		`ALTER TABLE task_environment_repos DROP COLUMN workspace_relative_path`,
	} {
		if _, err := repo.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("drop workspace placement column with %q: %v", statement, err)
		}
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay workspace placement migrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay workspace placement migrations second time: %v", err)
	}
	assertWorkspacePlacementLegacyDefaults(t, repo)
}

func assertWorkspacePlacementLegacyDefaults(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	task, err := repo.GetTask(ctx, "task-workspace-placement-legacy")
	if err != nil {
		t.Fatalf("get legacy task: %v", err)
	}
	if task.InitialWorkspaceLayout != "" {
		t.Fatalf("legacy task initial workspace layout = %q, want empty", task.InitialWorkspaceLayout)
	}
	taskRepo, err := repo.GetTaskRepository(ctx, "task-repo-workspace-placement-legacy")
	if err != nil {
		t.Fatalf("get legacy task repository: %v", err)
	}
	if taskRepo.WorkspaceRelativePath != "" {
		t.Fatalf("legacy task repository workspace relative path = %q, want empty", taskRepo.WorkspaceRelativePath)
	}
	env, err := repo.GetTaskEnvironment(ctx, "env-workspace-placement-legacy")
	if err != nil {
		t.Fatalf("get legacy task environment: %v", err)
	}
	if env.WorkspaceLayout != "" || len(env.Repos) != 1 || env.Repos[0].WorkspaceRelativePath != "" {
		t.Fatalf("legacy task environment = %#v, want empty placement fields", env)
	}
}

func TestMigrateTasksRebuildAddsInitialWorkspaceLayout(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := repo.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `DROP TABLE tasks`); err != nil {
		t.Fatalf("drop tasks: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			state TEXT DEFAULT 'TODO',
			priority INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0,
			wip_admitted INTEGER NOT NULL DEFAULT 1,
			queued_for_step_id TEXT NOT NULL DEFAULT '',
			queued_at TIMESTAMP,
			metadata TEXT DEFAULT '{}',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			parent_id TEXT DEFAULT '',
			autopilot_enabled INTEGER NOT NULL DEFAULT 0,
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id)
		)
	`); err != nil {
		t.Fatalf("create legacy tasks table: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO tasks (id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, "task-workspace-placement-rebuild", "Rebuilt task", now, now); err != nil {
		t.Fatalf("insert legacy task row: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("restore foreign keys: %v", err)
	}
	if err := repo.migrateTasksRemoveWorkflowFK(); err != nil {
		t.Fatalf("rebuild legacy tasks table: %v", err)
	}
	var layout string
	if err := repo.db.GetContext(ctx, &layout, `SELECT initial_workspace_layout FROM tasks WHERE id = ?`, "task-workspace-placement-rebuild"); err != nil {
		t.Fatalf("read rebuilt workspace layout: %v", err)
	}
	if layout != "" {
		t.Fatalf("rebuilt task initial workspace layout = %q, want empty", layout)
	}
}

func TestMigrateTaskRepositoriesRebuildAddsWorkspaceRelativePath(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	workspaceID := "ws-workspace-placement-repo-rebuild"
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-workspace-placement-repo-rebuild", WorkspaceID: workspaceID, Name: "legacy-repo"}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-workspace-placement-repo-rebuild", WorkspaceID: workspaceID, Title: "Legacy task"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	now := time.Now().UTC()
	if _, err := repo.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `DROP TABLE task_repositories`); err != nil {
		t.Fatalf("drop task repositories: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		CREATE TABLE task_repositories (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL,
			base_branch TEXT DEFAULT '',
			checkout_branch TEXT DEFAULT '',
			branch_policy_id TEXT DEFAULT '',
			branch_policy_name TEXT DEFAULT '',
			branch_policy_base_branch TEXT DEFAULT '',
			branch_policy_branch_template TEXT DEFAULT '',
			branch_policy_pull_request_target TEXT DEFAULT '',
			position INTEGER DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
			UNIQUE(task_id, repository_id)
		)
	`); err != nil {
		t.Fatalf("create legacy task repositories table: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "task-repo-workspace-placement-rebuild", "task-workspace-placement-repo-rebuild", "repo-workspace-placement-repo-rebuild", now, now); err != nil {
		t.Fatalf("insert legacy task repository row: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("restore foreign keys: %v", err)
	}
	if err := repo.migrateTaskRepositoriesAllowMultiBranch(); err != nil {
		t.Fatalf("rebuild legacy task repositories table: %v", err)
	}
	var path string
	if err := repo.db.GetContext(ctx, &path, `SELECT workspace_relative_path FROM task_repositories WHERE id = ?`, "task-repo-workspace-placement-rebuild"); err != nil {
		t.Fatalf("read rebuilt workspace relative path: %v", err)
	}
	if path != "" {
		t.Fatalf("rebuilt task repository workspace relative path = %q, want empty", path)
	}
}
