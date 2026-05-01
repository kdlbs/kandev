package sqlite

import (
	"context"
	"fmt"
	"math"
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
		workflow_id TEXT NOT NULL DEFAULT '',
		workflow_step_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL,
		state TEXT DEFAULT 'TODO',
		is_ephemeral INTEGER NOT NULL DEFAULT 0,
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

func assertDurationAlmostEqual(t *testing.T, got, want int64, label string) {
	t.Helper()
	const toleranceMs = int64(2)
	diff := int64(math.Abs(float64(got - want)))
	if diff > toleranceMs {
		t.Errorf("expected %s %dms (+/-%dms), got %dms", label, want, toleranceMs, got)
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

func TestGetGlobalStats_ExcludesEphemeralTasks(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	// Insert workspace + board + workflow step
	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at) VALUES ('step-todo', 'board-1', 'To Do', 0, ?, ?)`, nowStr, nowStr)

	// Insert a regular task
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-regular', 'ws-1', 'board-1', 'step-todo', 'Regular', 0, ?, ?)`, nowStr, nowStr)

	// Insert an ephemeral (quick chat) task
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-ephemeral', 'ws-1', 'board-1', '', 'Quick Chat', 1, ?, ?)`, nowStr, nowStr)

	// Add sessions for both tasks
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-regular', 'task-regular', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-ephemeral', 'task-ephemeral', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	// Add turns for both sessions
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-regular', 'sess-regular', 'task-regular', ?, ?, ?, ?)`, nowStr, nowStr, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-ephemeral', 'sess-ephemeral', 'task-ephemeral', ?, ?, ?, ?)`, nowStr, nowStr, nowStr, nowStr)

	// Global stats should only count the regular task
	stats, err := repo.GetGlobalStats(ctx, "ws-1", nil)
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}
	if stats.TotalTasks != 1 {
		t.Errorf("expected 1 total task (ephemeral excluded), got %d", stats.TotalTasks)
	}
	if stats.TotalSessions != 1 {
		t.Errorf("expected 1 total session (ephemeral excluded), got %d", stats.TotalSessions)
	}
	if stats.TotalTurns != 1 {
		t.Errorf("expected 1 total turn (ephemeral excluded), got %d", stats.TotalTurns)
	}
}

func TestGetTaskStats_ExcludesEphemeralTasks(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)

	// Insert regular and ephemeral tasks
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-regular', 'ws-1', 'board-1', '', 'Regular', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-ephemeral', 'ws-1', 'board-1', '', 'Quick Chat', 1, ?, ?)`, nowStr, nowStr)

	results, err := repo.GetTaskStats(ctx, "ws-1", nil, 100)
	if err != nil {
		t.Fatalf("GetTaskStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 task stat (ephemeral excluded), got %d", len(results))
	}
	if results[0].TaskID != "task-regular" {
		t.Errorf("expected task-regular, got %s", results[0].TaskID)
	}
}

func TestGetTaskStats_RespectsLimit(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)

	for i := 0; i < 5; i++ {
		taskID := fmt.Sprintf("task-%d", i)
		taskTitle := fmt.Sprintf("Task %d", i)
		execOrFatal(
			t,
			dbConn,
			`INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES (?, 'ws-1', 'board-1', '', ?, 0, ?, ?)`,
			taskID,
			taskTitle,
			nowStr,
			now.Add(time.Duration(i)*time.Second).Format(time.RFC3339),
		)
	}

	results, err := repo.GetTaskStats(ctx, "ws-1", nil, 3)
	if err != nil {
		t.Fatalf("GetTaskStats failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 task stats due to limit, got %d", len(results))
	}
}

func TestGetTaskStats_IncludesActiveAndElapsedDurations(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'Task 1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-1', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	// Two 10-minute turns with a 60-minute idle gap in between.
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-1', 'sess-1', 'task-1', '2026-01-01T10:00:00Z', '2026-01-01T10:10:00Z', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-2', 'sess-1', 'task-1', '2026-01-01T11:10:00Z', '2026-01-01T11:20:00Z', ?, ?)`, nowStr, nowStr)

	results, err := repo.GetTaskStats(ctx, "ws-1", nil, 100)
	if err != nil {
		t.Fatalf("GetTaskStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 task stat, got %d", len(results))
	}

	// Active duration = 20m, elapsed span = 80m.
	const (
		wantActiveMs  = int64(20 * 60 * 1000)
		wantElapsedMs = int64(80 * 60 * 1000)
	)
	assertDurationAlmostEqual(t, results[0].ActiveDurationMs, wantActiveMs, "active duration")
	assertDurationAlmostEqual(t, results[0].ElapsedSpanMs, wantElapsedMs, "elapsed span")
	assertDurationAlmostEqual(t, results[0].TotalDurationMs, wantActiveMs, "total duration")
}

func TestGetDailyActivity_ExcludesEphemeralTasks(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-regular', 'ws-1', 'board-1', '', 'Regular', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-ephemeral', 'ws-1', 'board-1', '', 'Quick Chat', 1, ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-regular', 'task-regular', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-ephemeral', 'task-ephemeral', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-regular', 'sess-regular', 'task-regular', ?, ?, ?, ?)`, nowStr, nowStr, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-ephemeral', 'sess-ephemeral', 'task-ephemeral', ?, ?, ?, ?)`, nowStr, nowStr, nowStr, nowStr)

	results, err := repo.GetDailyActivity(ctx, "ws-1", 7)
	if err != nil {
		t.Fatalf("GetDailyActivity failed: %v", err)
	}

	totalTurns := 0
	for _, r := range results {
		totalTurns += r.TurnCount
	}
	if totalTurns != 1 {
		t.Errorf("expected 1 total turn (ephemeral excluded), got %d", totalTurns)
	}
}

func TestGetCompletedTaskActivity_ExcludesEphemeralTasks(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at) VALUES ('step-done', 'board-1', 'Done', 1, ?, ?)`, nowStr, nowStr)

	// Completed task on last step with session completed_at (NOT archived — isolates session path)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-regular', 'ws-1', 'board-1', 'step-done', 'Regular', 0, ?, ?)`, nowStr, nowStr)
	// Archived task WITHOUT session completed_at — should still count using archived_at
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, archived_at, created_at, updated_at) VALUES ('task-archived-no-session', 'ws-1', 'board-1', 'step-done', 'Archived No Session', 0, ?, ?, ?)`, nowStr, nowStr, nowStr)
	// Task on last step with NO archived_at and NO session completed_at — should NOT count
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-last-step-no-dates', 'ws-1', 'board-1', 'step-done', 'No Dates', 0, ?, ?)`, nowStr, nowStr)
	// Ephemeral completed task
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, archived_at, created_at, updated_at) VALUES ('task-ephemeral', 'ws-1', 'board-1', '', 'Quick Chat', 1, ?, ?, ?)`, nowStr, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, completed_at, updated_at) VALUES ('sess-regular', 'task-regular', 'agent-1', 'COMPLETED', ?, ?, ?)`, nowStr, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, completed_at, updated_at) VALUES ('sess-ephemeral', 'task-ephemeral', 'agent-1', 'COMPLETED', ?, ?, ?)`, nowStr, nowStr, nowStr)
	// Session for archived task with NO completed_at
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-archived', 'task-archived-no-session', 'agent-1', 'CANCELLED', ?, ?)`, nowStr, nowStr)

	results, err := repo.GetCompletedTaskActivity(ctx, "ws-1", 7)
	if err != nil {
		t.Fatalf("GetCompletedTaskActivity failed: %v", err)
	}

	totalCompleted := 0
	for _, r := range results {
		totalCompleted += r.CompletedTasks
	}
	// Should count: task-regular (via session completed_at) + task-archived-no-session (via archived_at)
	// Should NOT count: task-last-step-no-dates (neither date), task-ephemeral (ephemeral)
	if totalCompleted != 2 {
		t.Errorf("expected 2 completed tasks (1 via session + 1 via archived_at), got %d", totalCompleted)
	}
}

func TestGetAgentUsage_ExcludesEphemeralTasks(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-regular', 'ws-1', 'board-1', '', 'Regular', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-ephemeral', 'ws-1', 'board-1', '', 'Quick Chat', 1, ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, agent_profile_snapshot, state, started_at, updated_at) VALUES ('sess-regular', 'task-regular', 'agent-1', '{"name":"Agent"}', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, agent_profile_snapshot, state, started_at, updated_at) VALUES ('sess-ephemeral', 'task-ephemeral', 'agent-1', '{"name":"Agent"}', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	results, err := repo.GetAgentUsage(ctx, "ws-1", 5, nil)
	if err != nil {
		t.Fatalf("GetAgentUsage failed: %v", err)
	}

	totalSessions := 0
	for _, r := range results {
		totalSessions += r.SessionCount
	}
	if totalSessions != 1 {
		t.Errorf("expected 1 session (ephemeral excluded), got %d", totalSessions)
	}
}

func TestGetGitStats_ExcludesEphemeralTasks(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-regular', 'ws-1', 'board-1', '', 'Regular', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-ephemeral', 'ws-1', 'board-1', '', 'Quick Chat', 1, ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-regular', 'task-regular', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-ephemeral', 'task-ephemeral', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, files_changed, insertions, deletions, created_at) VALUES ('c-regular', 'sess-regular', 'abc123', ?, 3, 10, 2, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, files_changed, insertions, deletions, created_at) VALUES ('c-ephemeral', 'sess-ephemeral', 'def456', ?, 5, 20, 8, ?)`, nowStr, nowStr)

	stats, err := repo.GetGitStats(ctx, "ws-1", nil)
	if err != nil {
		t.Fatalf("GetGitStats failed: %v", err)
	}
	if stats.TotalCommits != 1 {
		t.Errorf("expected 1 commit (ephemeral excluded), got %d", stats.TotalCommits)
	}
	if stats.TotalFilesChanged != 3 {
		t.Errorf("expected 3 files changed, got %d", stats.TotalFilesChanged)
	}
}

func TestGetRepositoryStats_ExcludesEphemeralTasks(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES ('repo-1', 'ws-1', 'my-repo', ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-regular', 'ws-1', 'board-1', '', 'Regular', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-ephemeral', 'ws-1', 'board-1', '', 'Quick Chat', 1, ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at) VALUES ('tr-1', 'task-regular', 'repo-1', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at) VALUES ('tr-2', 'task-ephemeral', 'repo-1', ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, repository_id, state, started_at, updated_at) VALUES ('sess-regular', 'task-regular', 'agent-1', 'repo-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, repository_id, state, started_at, updated_at) VALUES ('sess-ephemeral', 'task-ephemeral', 'agent-1', 'repo-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	results, err := repo.GetRepositoryStats(ctx, "ws-1", nil)
	if err != nil {
		t.Fatalf("GetRepositoryStats failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(results))
	}
	if results[0].TotalTasks != 1 {
		t.Errorf("expected 1 task (ephemeral excluded), got %d", results[0].TotalTasks)
	}
	if results[0].SessionCount != 1 {
		t.Errorf("expected 1 session (ephemeral excluded), got %d", results[0].SessionCount)
	}
}
