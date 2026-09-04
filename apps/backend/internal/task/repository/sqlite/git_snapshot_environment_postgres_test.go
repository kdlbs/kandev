package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresGitSnapshotEnvironmentMigrationIsTransactionalAndReplayable(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const (
		taskID      = "task-git-cutover-pg"
		environment = "env-git-cutover-pg"
		sessionID   = "session-git-cutover-pg"
	)
	seedPostgresTask(t, repo, taskID)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environment, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environment,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	replaceGitSnapshotTableWithLegacyPostgres(t, repo)
	base := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		id, files string
		createdAt time.Time
	}{
		{"legacy-pg-detailed", `{"old.go":{"status":"modified"}}`, base},
		{"legacy-pg-sparse", `{}`, base.Add(time.Hour)},
	} {
		if _, err := repo.db.Exec(repo.db.Rebind(`
			INSERT INTO task_session_git_snapshots (
				id, session_id, snapshot_type, branch, files, metadata, created_at
			) VALUES (?, ?, ?, ?, ?, '{}', ?)
		`), row.id, sessionID, string(models.SnapshotTypeStatusUpdate), "main", row.files, row.createdAt); err != nil {
			t.Fatalf("insert legacy row %s: %v", row.id, err)
		}
	}

	repo.failGitSnapshotCutoverAfter = "pre_swap"
	if err := repo.migrateGitSnapshotOwnership(); err == nil {
		t.Fatal("postgres git snapshot cutover unexpectedly succeeded with injected failure")
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_session_git_snapshots WHERE id = ?`, "legacy-pg-sparse"); got != 1 {
		t.Fatalf("legacy postgres row count after rollback = %d, want 1", got)
	}

	repo.failGitSnapshotCutoverAfter = ""
	if err := repo.migrateGitSnapshotOwnership(); err != nil {
		t.Fatalf("postgres git snapshot cutover: %v", err)
	}
	assertPostgresGitSnapshotEnvironmentWinner(t, repo, environment)
	if err := repo.migrateGitSnapshotOwnership(); err != nil {
		t.Fatalf("replay postgres git snapshot cutover: %v", err)
	}
	assertPostgresGitSnapshotEnvironmentWinner(t, repo, environment)
}

func replaceGitSnapshotTableWithLegacyPostgres(t *testing.T, repo *Repository) {
	t.Helper()
	for _, indexName := range []string{
		"idx_git_snapshots_environment",
		"idx_git_snapshots_environment_type",
		"idx_git_snapshots_session",
		"idx_git_snapshots_type",
	} {
		if _, err := repo.db.Exec("DROP INDEX IF EXISTS " + indexName); err != nil {
			t.Fatalf("drop postgres snapshot index %s: %v", indexName, err)
		}
	}
	if _, err := repo.db.Exec("DROP TABLE task_session_git_snapshots"); err != nil {
		t.Fatalf("drop postgres current snapshot table: %v", err)
	}
	if _, err := repo.db.Exec(`
		CREATE TABLE task_session_git_snapshots (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			snapshot_type TEXT NOT NULL,
			branch TEXT NOT NULL,
			remote_branch TEXT DEFAULT '',
			head_commit TEXT DEFAULT '',
			base_commit TEXT DEFAULT '',
			ahead INTEGER DEFAULT 0,
			behind INTEGER DEFAULT 0,
			files TEXT DEFAULT '{}',
			triggered_by TEXT DEFAULT '',
			metadata TEXT DEFAULT '{}',
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		)
	`); err != nil {
		t.Fatalf("create postgres legacy snapshot table: %v", err)
	}
}

func assertPostgresGitSnapshotEnvironmentWinner(t *testing.T, repo *Repository, environment string) {
	t.Helper()
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_session_git_snapshots`); got != 1 {
		t.Fatalf("postgres snapshot count = %d, want one winner", got)
	}
	current, err := repo.GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(context.Background(), []string{environment})
	if err != nil {
		t.Fatalf("read postgres current snapshot: %v", err)
	}
	if len(current) != 1 || current[0].ID != "legacy-pg-sparse" || current[0].Files != nil {
		t.Fatalf("postgres current snapshot = %+v, want sparse winner", current)
	}
}
