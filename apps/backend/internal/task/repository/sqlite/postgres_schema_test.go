package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresSchemaReinitializes(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("first postgres schema init: %v", err)
	}
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second postgres schema init: %v", err)
	}
}

func TestPostgresSkipsLegacyTaskEnvironmentBackfill(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), "task-orphaned", "Orphaned task", now, now); err != nil {
		t.Fatalf("insert orphaned task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "session-orphaned", "task-orphaned", "CREATED", now, now); err != nil {
		t.Fatalf("insert orphaned session: %v", err)
	}

	if err := repo.backfillTaskEnvironments(); err != nil {
		t.Fatalf("backfill task environments: %v", err)
	}

	var count int
	if err := db.Get(&count, db.Rebind(`
		SELECT COUNT(*) FROM task_environments WHERE task_id = ?
	`), "task-orphaned"); err != nil {
		t.Fatalf("count task environments: %v", err)
	}
	if count != 0 {
		t.Fatalf("task environment count = %d, want 0", count)
	}
}

func TestPostgresTaskEnvironmentReposMultiBranchMigration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := db.Exec(`
		CREATE TABLE task_environment_repos (
			id TEXT PRIMARY KEY,
			task_environment_id TEXT NOT NULL,
			repository_id TEXT NOT NULL,
			worktree_id TEXT DEFAULT '',
			worktree_path TEXT DEFAULT '',
			worktree_branch TEXT DEFAULT '',
			position INTEGER DEFAULT 0,
			error_message TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			UNIQUE(task_environment_id, repository_id)
		)
	`); err != nil {
		t.Fatalf("create legacy task_environment_repos: %v", err)
	}

	repo := &Repository{db: db}
	if err := repo.migrateTaskEnvironmentReposAllowMultiBranch(); err != nil {
		t.Fatalf("migrate task_environment_repos: %v", err)
	}
	if err := repo.migrateTaskEnvironmentReposAllowMultiBranch(); err != nil {
		t.Fatalf("rerun migration: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, created_at, updated_at
		) VALUES
			('ter-main', 'env-1', 'repo-1', '', 'wt-main', $1, $1),
			('ter-branch', 'env-1', 'repo-1', 'branch-5hn', 'wt-branch', $1, $1)
	`, now); err != nil {
		t.Fatalf("insert same repo multi-branch rows: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, created_at, updated_at
		) VALUES ('ter-dupe', 'env-1', 'repo-1', '', 'wt-dupe', $1, $1)
	`, now); err == nil {
		t.Fatal("expected duplicate env/repo/branch insert to fail")
	}
}
