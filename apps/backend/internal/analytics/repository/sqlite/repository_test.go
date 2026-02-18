package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
)

// createTestDB sets up a SQLite database with all required schemas for analytics queries.
func createTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	// Create tables that analytics queries depend on
	schema := `
	CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS boards (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS workflow_steps (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL,
		name TEXT NOT NULL,
		position INTEGER NOT NULL,
		color TEXT,
		prompt TEXT,
		events TEXT,
		allow_manual_move INTEGER DEFAULT 1,
		auto_archive_after_hours INTEGER DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		board_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL,
		state TEXT DEFAULT 'TODO',
		archived_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS repositories (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		source_type TEXT NOT NULL DEFAULT 'local',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS task_repositories (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		UNIQUE(task_id, repository_id)
	);
	CREATE TABLE IF NOT EXISTS task_sessions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		agent_profile_id TEXT NOT NULL,
		agent_profile_snapshot TEXT DEFAULT '{}',
		repository_id TEXT DEFAULT '',
		state TEXT NOT NULL DEFAULT 'CREATED',
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS task_session_turns (
		id TEXT PRIMARY KEY,
		task_session_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS task_session_messages (
		id TEXT PRIMARY KEY,
		task_session_id TEXT NOT NULL,
		turn_id TEXT NOT NULL,
		author_type TEXT NOT NULL DEFAULT 'user',
		type TEXT NOT NULL DEFAULT 'message',
		content TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS task_session_commits (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		commit_sha TEXT NOT NULL,
		committed_at TIMESTAMP NOT NULL,
		files_changed INTEGER DEFAULT 0,
		insertions INTEGER DEFAULT 0,
		deletions INTEGER DEFAULT 0,
		created_at TIMESTAMP NOT NULL
	);
	`
	if _, err := sqlxDB.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	return sqlxDB
}

func execOrFatal(t *testing.T, dbConn *sqlx.DB, query string, args ...any) {
	t.Helper()
	if _, err := dbConn.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %v", err)
	}
}

func TestEnsureStatsIndexes_CreatesIndexes(t *testing.T) {
	dbConn := createTestDB(t)

	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	// Verify indexes were created
	var count int
	err = dbConn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count indexes: %v", err)
	}
	if count == 0 {
		t.Error("expected indexes to be created, got 0")
	}

	_ = repo // keep reference
}

func TestEnsureStatsIndexes_Idempotent(t *testing.T) {
	dbConn := createTestDB(t)

	repo := &Repository{db: dbConn}
	if err := repo.ensureStatsIndexes(); err != nil {
		t.Fatalf("first ensureStatsIndexes failed: %v", err)
	}

	// Calling again should not error (CREATE INDEX IF NOT EXISTS is idempotent)
	if err := repo.ensureStatsIndexes(); err != nil {
		t.Fatalf("second ensureStatsIndexes failed: %v", err)
	}
}

func TestGetRepositoryStats_ExcludesSoftDeletedRepos(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// Insert workspace
	if _, err := dbConn.Exec(
		`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"ws-1", "Test Workspace", now, now,
	); err != nil {
		t.Fatalf("failed to insert workspace: %v", err)
	}

	// Insert an active repo
	if _, err := dbConn.Exec(
		`INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"repo-active", "ws-1", "active-repo", now, now,
	); err != nil {
		t.Fatalf("failed to insert active repo: %v", err)
	}

	// Insert a soft-deleted repo
	if _, err := dbConn.Exec(
		`INSERT INTO repositories (id, workspace_id, name, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"repo-deleted", "ws-1", "deleted-repo", now, now, now,
	); err != nil {
		t.Fatalf("failed to insert deleted repo: %v", err)
	}

	results, err := repo.GetRepositoryStats(ctx, "ws-1", nil)
	if err != nil {
		t.Fatalf("GetRepositoryStats failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(results))
	}
	if results[0].RepositoryID != "repo-active" {
		t.Errorf("expected repo-active, got %s", results[0].RepositoryID)
	}
}

func TestGetGlobalStats_EmptyWorkspace(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	stats, err := repo.GetGlobalStats(ctx, "nonexistent", nil)
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}

	if stats.TotalTasks != 0 {
		t.Errorf("expected 0 total tasks, got %d", stats.TotalTasks)
	}
	if stats.TotalSessions != 0 {
		t.Errorf("expected 0 total sessions, got %d", stats.TotalSessions)
	}
}

func TestGetGlobalStats_WithTimeFilter(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	oldStr := now.AddDate(0, 0, -60).Format(time.RFC3339)

	// Insert workspace + board + workflow step
	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at) VALUES ('step-todo', 'board-1', 'To Do', 0, ?, ?)`, nowStr, nowStr)

	// Insert a recent task and an old task
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, created_at, updated_at) VALUES ('task-recent', 'ws-1', 'board-1', 'step-todo', 'Recent', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, created_at, updated_at) VALUES ('task-old', 'ws-1', 'board-1', 'step-todo', 'Old', ?, ?)`, oldStr, oldStr)

	// With no time filter — should see both
	stats, err := repo.GetGlobalStats(ctx, "ws-1", nil)
	if err != nil {
		t.Fatalf("GetGlobalStats (no filter) failed: %v", err)
	}
	if stats.TotalTasks != 2 {
		t.Errorf("expected 2 total tasks (no filter), got %d", stats.TotalTasks)
	}

	// With time filter for last 7 days — should see only the recent one
	weekAgo := now.AddDate(0, 0, -7)
	stats, err = repo.GetGlobalStats(ctx, "ws-1", &weekAgo)
	if err != nil {
		t.Fatalf("GetGlobalStats (week filter) failed: %v", err)
	}
	if stats.TotalTasks != 1 {
		t.Errorf("expected 1 total task (week filter), got %d", stats.TotalTasks)
	}
}

func TestGetGitStats_EmptyWorkspace(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	stats, err := repo.GetGitStats(ctx, "nonexistent", nil)
	if err != nil {
		t.Fatalf("GetGitStats failed: %v", err)
	}

	if stats.TotalCommits != 0 {
		t.Errorf("expected 0 commits, got %d", stats.TotalCommits)
	}
}
