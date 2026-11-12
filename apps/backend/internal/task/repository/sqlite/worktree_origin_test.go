package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func openWorktreeOriginTestRepository(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "worktree-origin.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo := NewWithInitializedDB(sqlxDB, sqlxDB, nil)
	if err := repo.initSchema(); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	seedWorktreeOriginTask(t, repo)
	return repo
}

func seedWorktreeOriginTask(t *testing.T, repo *Repository) {
	t.Helper()
	statements := []string{
		`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('ws-origin','Origin',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO tasks(id,workspace_id,title,state,created_at,updated_at) VALUES('task-origin','ws-origin','Origin','IN_PROGRESS',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO repositories(id,workspace_id,name,source_type,created_at,updated_at) VALUES('repo-origin','ws-origin','origin','local',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		`INSERT INTO task_environments(id,task_id,executor_type,status,ownership_generation,workspace_path,created_at,updated_at) VALUES('env-origin','task-origin','worktree','ready',1,'/tmp/task-origin',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
	}
	for _, statement := range statements {
		if _, err := repo.db.Exec(repo.db.Rebind(statement)); err != nil {
			t.Fatalf("seed worktree origin: %v", err)
		}
	}
}

func TestWorktreeOriginPersistsAndDefaults(t *testing.T) {
	repo := openWorktreeOriginTestRepository(t)
	ctx := context.Background()
	row := &models.TaskEnvironmentRepo{
		ID: "env-repo-adopted", TaskEnvironmentID: "env-origin", RepositoryID: "repo-origin",
		WorktreeID: "wt-adopted", WorktreePath: "/tmp/task-origin/.claude/worktrees/fix",
		WorktreeBranch: "fix", BranchSlug: "fix", WorktreeOrigin: models.WorktreeOriginAdopted,
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, row); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
	}
	rows, err := repo.ListTaskEnvironmentRepos(ctx, "env-origin")
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListTaskEnvironmentRepos: rows=%d err=%v", len(rows), err)
	}
	if rows[0].WorktreeOrigin != models.WorktreeOriginAdopted {
		t.Fatalf("worktree origin = %q, want adopted", rows[0].WorktreeOrigin)
	}
	defaultRow := &models.TaskEnvironmentRepo{
		ID: "env-repo-default", TaskEnvironmentID: "env-origin", RepositoryID: "repo-origin",
		WorktreeID: "wt-default", WorktreePath: "/tmp/task-origin/primary",
		WorktreeBranch: "main", BranchSlug: "main",
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, defaultRow); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo default: %v", err)
	}
	rows, _ = repo.ListTaskEnvironmentRepos(ctx, "env-origin")
	for _, candidate := range rows {
		if candidate.ID == "env-repo-default" && candidate.WorktreeOrigin != models.WorktreeOriginKandev {
			t.Fatalf("default worktree origin = %q, want kandev", candidate.WorktreeOrigin)
		}
	}
}

func TestWorktreeOriginMigrationReplay(t *testing.T) {
	repo := openWorktreeOriginTestRepository(t)
	ctx := context.Background()
	if _, err := repo.db.Exec(`ALTER TABLE task_environment_repos DROP COLUMN worktree_origin`); err != nil {
		t.Skipf("sqlite does not support dropping the column: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
	row := &models.TaskEnvironmentRepo{
		ID: "env-repo-replay", TaskEnvironmentID: "env-origin", RepositoryID: "repo-origin",
		WorktreeID: "wt-replay", WorktreePath: "/tmp/task-origin/primary",
		WorktreeBranch: "main", BranchSlug: "main",
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, row); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo after replay: %v", err)
	}
	rows, err := repo.ListTaskEnvironmentRepos(ctx, "env-origin")
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListTaskEnvironmentRepos: rows=%d err=%v", len(rows), err)
	}
	if rows[0].WorktreeOrigin != models.WorktreeOriginKandev {
		t.Fatalf("worktree origin = %q, want kandev default", rows[0].WorktreeOrigin)
	}
}

func TestDetachedWorktreeStoreLifecycle(t *testing.T) {
	repo := openWorktreeOriginTestRepository(t)
	ctx := context.Background()
	path := "/tmp/task-origin/.claude/worktrees/fix"
	if err := repo.MarkTaskWorktreeDetached(ctx, "task-origin", path, time.Now().UTC()); err != nil {
		t.Fatalf("MarkTaskWorktreeDetached: %v", err)
	}
	detached, err := repo.IsTaskWorktreeDetached(ctx, "task-origin", path)
	if err != nil || !detached {
		t.Fatalf("IsTaskWorktreeDetached = %v, %v", detached, err)
	}
	if err := repo.MarkTaskWorktreeDetached(ctx, "task-origin", path, time.Now().UTC()); err != nil {
		t.Fatalf("idempotent MarkTaskWorktreeDetached: %v", err)
	}
	if err := repo.ClearTaskWorktreeDetached(ctx, "task-origin", path); err != nil {
		t.Fatalf("ClearTaskWorktreeDetached: %v", err)
	}
	detached, err = repo.IsTaskWorktreeDetached(ctx, "task-origin", path)
	if err != nil || detached {
		t.Fatalf("after clear IsTaskWorktreeDetached = %v, %v", detached, err)
	}
	if err := repo.MarkTaskWorktreeDetached(ctx, "task-origin", path, time.Now().UTC()); err != nil {
		t.Fatalf("MarkTaskWorktreeDetached again: %v", err)
	}
	if err := repo.DeleteTaskDetachedWorktreesByTask(ctx, "task-origin"); err != nil {
		t.Fatalf("DeleteTaskDetachedWorktreesByTask: %v", err)
	}
	detached, _ = repo.IsTaskWorktreeDetached(ctx, "task-origin", path)
	if detached {
		t.Fatal("task-scoped cleanup left a detachment row")
	}
}

func TestDetachTaskRepositoryAttachmentIsAtomic(t *testing.T) {
	repo := openWorktreeOriginTestRepository(t)
	ctx := context.Background()
	seed, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_repositories(id,task_id,repository_id,checkout_branch,workspace_relative_path,position,created_at,updated_at)
		VALUES('tr-detach','task-origin','repo-origin','fix-b','.claude/worktrees/fix-b',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`))
	if err != nil {
		t.Fatalf("seed task repository: %v", err)
	}
	if rows, _ := seed.RowsAffected(); rows != 1 {
		t.Fatalf("seeded task repository rows = %d", rows)
	}
	row := &models.TaskEnvironmentRepo{
		ID: "env-repo-detach", TaskEnvironmentID: "env-origin", RepositoryID: "repo-origin",
		WorktreeID: "wt-detach", WorktreePath: "/tmp/task-origin/.claude/worktrees/fix-b",
		WorktreeBranch: "fix-b", BranchSlug: "fix-b", WorkspaceRelativePath: ".claude/worktrees/fix-b",
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, row); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
	}
	path := "/tmp/task-origin/.claude/worktrees/fix-b"
	if err := repo.DetachTaskRepositoryAttachment(ctx, "task-origin", "tr-detach", row.ID, path, true); err != nil {
		t.Fatalf("DetachTaskRepositoryAttachment: %v", err)
	}
	detached, err := repo.IsTaskWorktreeDetached(ctx, "task-origin", path)
	if err != nil || !detached {
		t.Fatalf("suppression row = %v, %v", detached, err)
	}
	rows, _ := repo.ListTaskEnvironmentRepos(ctx, "env-origin")
	if len(rows) != 1 || rows[0].Status != models.TaskEnvironmentRepoStatusDetached {
		t.Fatalf("retained inventory = %+v", rows)
	}
	var count int
	if err := repo.db.Get(&count, repo.db.Rebind(`SELECT COUNT(*) FROM task_repositories WHERE id='tr-detach'`)); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("task repository row survived detach")
	}
}

func TestDetachTaskRepositoryAttachmentRollsBackOnFailure(t *testing.T) {
	repo := openWorktreeOriginTestRepository(t)
	ctx := context.Background()
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_repositories(id,task_id,repository_id,checkout_branch,workspace_relative_path,position,created_at,updated_at)
		VALUES('tr-rollback','task-origin','repo-origin','fix-b','.claude/worktrees/fix-b',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)); err != nil {
		t.Fatalf("seed task repository: %v", err)
	}
	row := &models.TaskEnvironmentRepo{
		ID: "env-repo-rollback", TaskEnvironmentID: "env-origin", RepositoryID: "repo-origin",
		WorktreeID: "wt-rollback", WorktreePath: "/tmp/task-origin/.claude/worktrees/fix-b",
		WorktreeBranch: "fix-b", BranchSlug: "fix-b",
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, row); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
	}
	path := "/tmp/task-origin/.claude/worktrees/fix-b"
	if err := repo.DetachTaskRepositoryAttachment(ctx, "task-origin", "missing", row.ID, path, true); err == nil {
		t.Fatal("expected a missing task repository failure")
	}
	detached, err := repo.IsTaskWorktreeDetached(ctx, "task-origin", path)
	if err != nil || detached {
		t.Fatalf("suppression row after rollback = %v, %v", detached, err)
	}
	rows, _ := repo.ListTaskEnvironmentRepos(ctx, "env-origin")
	if len(rows) != 1 || rows[0].Status != models.TaskEnvironmentRepoStatusActive {
		t.Fatalf("inventory after rollback = %+v", rows)
	}
}
