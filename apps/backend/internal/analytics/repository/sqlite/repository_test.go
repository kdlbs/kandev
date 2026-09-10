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
		origin TEXT NOT NULL DEFAULT '',
		metadata TEXT DEFAULT '{}',
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
	CREATE TABLE IF NOT EXISTS task_session_git_snapshots (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		snapshot_type TEXT NOT NULL DEFAULT '',
		files TEXT DEFAULT '{}',
		created_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS kandev_meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	);
	`
	if _, err := sqlxDB.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	// Seed a commit-capture activation marker far in the past, matching
	// migrateSessionCommitsDedupeAndActivation's production kandev_meta row,
	// so every test session (started_at ~ time.Now()) is post-activation and
	// gets a real 0 for committed lines rather than the legacy-NULL case -
	// tests that want the legacy path override this row explicitly.
	if _, err := sqlxDB.Exec(
		`INSERT INTO kandev_meta (key, value) VALUES (?, ?)`,
		commitCaptureActivatedAtMetaKey, "2000-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("failed to seed commit_capture_activated_at: %v", err)
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

// TestGetGlobalStats_AggregatesAcrossTables exercises the CTE-based query path
// for every aggregated field — total/user/tool messages, turn count, and the
// computed total duration — to guard against regressions when the query is
// further consolidated.
func TestGetGlobalStats_AggregatesAcrossTables(t *testing.T) {
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
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-1', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	// Two turns: 5m + 10m of active duration (= 900_000 ms). All messages live
	// on turn-1; turn-2 is intentionally empty so it's excluded from
	// clean_turn_agg via the msg_count >= cleanTurnMinMessages filter.
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-1', 'sess-1', 'task-1', '2026-01-01T10:00:00Z', '2026-01-01T10:05:00Z', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-2', 'sess-1', 'task-1', '2026-01-01T10:10:00Z', '2026-01-01T10:20:00Z', ?, ?)`, nowStr, nowStr)
	// Messages: 2 user, 1 tool_call, 1 tool_edit, 1 agent text.
	execOrFatal(t, dbConn, `INSERT INTO task_session_messages (id, task_session_id, turn_id, author_type, type, content, created_at) VALUES ('m-1', 'sess-1', 'turn-1', 'user', 'message', 'hi', ?)`, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_messages (id, task_session_id, turn_id, author_type, type, content, created_at) VALUES ('m-2', 'sess-1', 'turn-1', 'user', 'message', 'hello', ?)`, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_messages (id, task_session_id, turn_id, author_type, type, content, created_at) VALUES ('m-3', 'sess-1', 'turn-1', 'agent', 'tool_call', 'bash', ?)`, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_messages (id, task_session_id, turn_id, author_type, type, content, created_at) VALUES ('m-4', 'sess-1', 'turn-1', 'agent', 'tool_edit', 'edit', ?)`, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_session_messages (id, task_session_id, turn_id, author_type, type, content, created_at) VALUES ('m-5', 'sess-1', 'turn-1', 'agent', 'message', 'reply', ?)`, nowStr)

	stats, err := repo.GetGlobalStats(ctx, "ws-1", nil)
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}
	if stats.TotalTasks != 1 {
		t.Errorf("TotalTasks: want 1, got %d", stats.TotalTasks)
	}
	if stats.TotalSessions != 1 {
		t.Errorf("TotalSessions: want 1, got %d", stats.TotalSessions)
	}
	if stats.TotalTurns != 2 {
		t.Errorf("TotalTurns: want 2, got %d", stats.TotalTurns)
	}
	if stats.TotalMessages != 5 {
		t.Errorf("TotalMessages: want 5, got %d", stats.TotalMessages)
	}
	if stats.TotalUserMessages != 2 {
		t.Errorf("TotalUserMessages: want 2, got %d", stats.TotalUserMessages)
	}
	if stats.TotalToolCalls != 2 {
		t.Errorf("TotalToolCalls: want 2 (tool_call + tool_edit), got %d", stats.TotalToolCalls)
	}
	assertDurationAlmostEqual(t, stats.TotalDurationMs, int64(15*60*1000), "TotalDurationMs")
	// Clean-turn averages: only turn-1 (5m, 5 msgs) qualifies; turn-2 has 0 messages so it's excluded.
	assertDurationAlmostEqual(t, stats.AvgTurnDurationMs, int64(5*60*1000), "AvgTurnDurationMs")
	if math.Abs(stats.AvgMessagesPerTurn-5.0) > 0.01 {
		t.Errorf("AvgMessagesPerTurn: want 5.0, got %f", stats.AvgMessagesPerTurn)
	}
}

// TestGetGlobalStats_CleanTurnExcludesOutliers verifies that the clean-turn
// metrics exclude turns shorter than 1s, longer than 1h, with no messages, or
// without a completed_at timestamp.
func TestGetGlobalStats_CleanTurnExcludesOutliers(t *testing.T) {
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
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-1', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	// turn-good: 2m, 4 messages — included.
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-good', 'sess-1', 'task-1', '2026-01-01T10:00:00Z', '2026-01-01T10:02:00Z', ?, ?)`, nowStr, nowStr)
	// turn-short: 0.5s — excluded (below 1s floor).
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-short', 'sess-1', 'task-1', '2026-01-01T11:00:00.000Z', '2026-01-01T11:00:00.500Z', ?, ?)`, nowStr, nowStr)
	// turn-zombie: 2h — excluded (above 1h ceiling).
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-zombie', 'sess-1', 'task-1', '2026-01-01T12:00:00Z', '2026-01-01T14:00:00Z', ?, ?)`, nowStr, nowStr)
	// turn-boundary: exactly 1h — excluded (cleanTurnMaxDurationMs is exclusive).
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-boundary', 'sess-1', 'task-1', '2026-01-01T13:00:00Z', '2026-01-01T14:00:00Z', ?, ?)`, nowStr, nowStr)
	// turn-empty: 10m, 0 messages — excluded (no messages).
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-empty', 'sess-1', 'task-1', '2026-01-01T15:00:00Z', '2026-01-01T15:10:00Z', ?, ?)`, nowStr, nowStr)
	// turn-incomplete: still running — excluded (NULL completed_at).
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at) VALUES ('turn-incomplete', 'sess-1', 'task-1', '2026-01-01T16:00:00Z', ?, ?)`, nowStr, nowStr)

	// Messages: 4 on turn-good, 1 each on the otherwise-excluded turns so they aren't excluded by msg filter alone.
	for i, turnID := range []string{"turn-good", "turn-good", "turn-good", "turn-good", "turn-short", "turn-zombie", "turn-boundary", "turn-incomplete"} {
		mid := fmt.Sprintf("m-%d", i)
		execOrFatal(t, dbConn, `INSERT INTO task_session_messages (id, task_session_id, turn_id, author_type, type, content, created_at) VALUES (?, 'sess-1', ?, 'user', 'message', 'x', ?)`, mid, turnID, nowStr)
	}

	stats, err := repo.GetGlobalStats(ctx, "ws-1", nil)
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}
	// Only turn-good (120s, 4 msgs) contributes.
	assertDurationAlmostEqual(t, stats.AvgTurnDurationMs, int64(2*60*1000), "AvgTurnDurationMs")
	if math.Abs(stats.AvgMessagesPerTurn-4.0) > 0.01 {
		t.Errorf("AvgMessagesPerTurn: want 4.0, got %f", stats.AvgMessagesPerTurn)
	}
}

// TestGetGlobalStats_CleanTurnZeroWhenNoQualifyingTurns verifies that the
// clean-turn averages are zero (not NaN/NULL) when every turn is filtered out.
func TestGetGlobalStats_CleanTurnZeroWhenNoQualifyingTurns(t *testing.T) {
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
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'T1', 0, ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-1', 'task-1', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	// Single short turn (excluded) — avg should be 0, not NaN.
	execOrFatal(t, dbConn, `INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, completed_at, created_at, updated_at) VALUES ('turn-short', 'sess-1', 'task-1', '2026-01-01T10:00:00.000Z', '2026-01-01T10:00:00.100Z', ?, ?)`, nowStr, nowStr)

	stats, err := repo.GetGlobalStats(ctx, "ws-1", nil)
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}
	if stats.AvgTurnDurationMs != 0 {
		t.Errorf("AvgTurnDurationMs: want 0, got %d", stats.AvgTurnDurationMs)
	}
	if stats.AvgMessagesPerTurn != 0 {
		t.Errorf("AvgMessagesPerTurn: want 0, got %f", stats.AvgMessagesPerTurn)
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

func TestGetModelUsage_ExcludesEphemeralTasks(t *testing.T) {
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

	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, agent_profile_snapshot, state, started_at, updated_at) VALUES ('sess-regular', 'task-regular', 'agent-1', '{"name":"Agent","model":"opus"}', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, agent_profile_snapshot, state, started_at, updated_at) VALUES ('sess-ephemeral', 'task-ephemeral', 'agent-1', '{"name":"Agent","model":"opus"}', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	results, err := repo.GetModelUsage(ctx, "ws-1", 5, nil)
	if err != nil {
		t.Fatalf("GetModelUsage failed: %v", err)
	}

	totalSessions := 0
	for _, r := range results {
		totalSessions += r.SessionCount
	}
	if totalSessions != 1 {
		t.Errorf("expected 1 session (ephemeral excluded), got %d", totalSessions)
	}
}

// The reported bug: a shorter range advertised a model that the longer range
// containing it never mentioned, because usage was keyed by the agent profile
// and each range labelled the group from whichever session row it happened to
// read. Keyed by model, a range's rows are always a subset of the wider range's
// rows and every count only grows.
func TestGetModelUsage_CountsNestAcrossRanges(t *testing.T) {
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
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'Task', 0, ?, ?)`, nowStr, nowStr)

	// One profile, repointed at a different model twice over the period.
	sessions := []struct {
		id       string
		snapshot string
		age      time.Duration
	}{
		{"sess-recent-a", `{"name":"Default (recommended)","model":"opus[1m]"}`, 24 * time.Hour},
		{"sess-recent-b", `{"name":"Default (recommended)","model":"opus[1m]"}`, 3 * 24 * time.Hour},
		{"sess-mid", `{"name":"Default (recommended)","model":"claude-fable-5[1m]"}`, 12 * 24 * time.Hour},
		{"sess-old", `{"name":"Default","model":"sonnet"}`, 40 * 24 * time.Hour},
	}
	for _, s := range sessions {
		startedAt := now.Add(-s.age).Format(time.RFC3339)
		execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, agent_profile_snapshot, state, started_at, updated_at) VALUES (?, 'task-1', 'agent-1', ?, 'COMPLETED', ?, ?)`,
			s.id, s.snapshot, startedAt, startedAt)
	}

	weekStart := now.Add(-7 * 24 * time.Hour)
	monthStart := now.Add(-30 * 24 * time.Hour)

	cases := []struct {
		name  string
		start *time.Time
		want  map[string]int
	}{
		{"last week", &weekStart, map[string]int{"opus[1m]": 2}},
		{"last month", &monthStart, map[string]int{"opus[1m]": 2, "claude-fable-5[1m]": 1}},
		{"all time", nil, map[string]int{"opus[1m]": 2, "claude-fable-5[1m]": 1, "sonnet": 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results, err := repo.GetModelUsage(ctx, "ws-1", 5, tc.start)
			if err != nil {
				t.Fatalf("GetModelUsage failed: %v", err)
			}
			got := map[string]int{}
			for _, r := range results {
				got[r.Model] = r.SessionCount
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

// Two profiles pointed at the same model are one line of usage, not two rows
// disagreeing about what ran.
func TestGetModelUsage_MergesProfilesSharingAModel(t *testing.T) {
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
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'Task', 0, ?, ?)`, nowStr, nowStr)

	sessions := []struct{ id, profile, snapshot string }{
		{"sess-a", "agent-default", `{"name":"Default (recommended)","model":"opus[1m]"}`},
		{"sess-b", "agent-sonnet", `{"name":"Sonnet","model":"opus[1m]"}`},
		// model_name and llm are the fallback snapshot keys for the same field.
		{"sess-c", "agent-alt", `{"name":"Alt","model_name":"opus[1m]"}`},
		{"sess-d", "agent-legacy", `{"name":"Legacy","llm":"opus[1m]"}`},
		// No model recorded at all: counted nowhere rather than under a blank label.
		{"sess-e", "agent-unknown", `{"name":"Unknown"}`},
	}
	for _, s := range sessions {
		execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, agent_profile_snapshot, state, started_at, updated_at) VALUES (?, 'task-1', ?, ?, 'COMPLETED', ?, ?)`,
			s.id, s.profile, s.snapshot, nowStr, nowStr)
	}

	results, err := repo.GetModelUsage(ctx, "ws-1", 5, nil)
	if err != nil {
		t.Fatalf("GetModelUsage failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 model row, got %d: %+v", len(results), results)
	}
	if results[0].Model != "opus[1m]" || results[0].SessionCount != 4 {
		t.Errorf("expected opus[1m] with 4 sessions, got %q with %d", results[0].Model, results[0].SessionCount)
	}
}

// Rows tied on session count must come back in a stable order rather than
// shuffling between reloads of the same stats page.
func TestGetModelUsage_OrdersTiesDeterministically(t *testing.T) {
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
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES ('task-1', 'ws-1', 'board-1', '', 'Task', 0, ?, ?)`, nowStr, nowStr)

	for _, model := range []string{"zeta", "alpha", "mu"} {
		execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, agent_profile_snapshot, state, started_at, updated_at) VALUES (?, 'task-1', 'agent-1', ?, 'COMPLETED', ?, ?)`,
			"sess-"+model, fmt.Sprintf(`{"name":"Agent","model":%q}`, model), nowStr, nowStr)
	}

	want := []string{"alpha", "mu", "zeta"}
	for attempt := 0; attempt < 3; attempt++ {
		results, err := repo.GetModelUsage(ctx, "ws-1", 5, nil)
		if err != nil {
			t.Fatalf("GetModelUsage failed: %v", err)
		}
		got := make([]string, 0, len(results))
		for _, r := range results {
			got = append(got, r.Model)
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("attempt %d: expected %v, got %v", attempt, want, got)
		}
	}
}

// Rows dated on the range's first day used to be dropped wholesale: the
// boundary was bound as an RFC3339 string whose "T" separator sorts above the
// space in the layout SQLite actually stores, and the TIMESTAMP columns' NUMERIC
// affinity leaves a text parameter uncoerced, so ">=" was comparing strings.
// "Last 7 days" was really the last 6 days plus today.
//
// Every timestamp here is written the way production writes it — by handing the
// driver a time.Time — because the other tests in this file insert preformatted
// RFC3339 strings, which is exactly why none of them saw this.
func TestRangeStart_IncludesRowsLaterOnTheBoundaryDay(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	// Fixed clock: the boundary and both rows share one calendar day, so the
	// separator mismatch is what decides the result, not the wall clock.
	threshold := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	before := threshold.Add(-6 * time.Hour)
	after := threshold.Add(6 * time.Hour)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, before, before)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, before, before)
	execOrFatal(t, dbConn, `INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES ('repo-1', 'ws-1', 'Repo', ?, ?)`, before, before)

	for _, row := range []struct {
		suffix string
		at     time.Time
	}{{"before", before}, {"after", after}} {
		execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, created_at, updated_at) VALUES (?, 'ws-1', 'board-1', '', 'Task', 0, ?, ?)`,
			"task-"+row.suffix, row.at, row.at)
		execOrFatal(t, dbConn, `INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at) VALUES (?, ?, 'repo-1', ?, ?)`,
			"task-repo-"+row.suffix, "task-"+row.suffix, row.at, row.at)
		execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, agent_profile_snapshot, repository_id, state, started_at, updated_at) VALUES (?, ?, 'agent-1', '{"name":"Agent","model":"opus"}', 'repo-1', 'COMPLETED', ?, ?)`,
			"sess-"+row.suffix, "task-"+row.suffix, row.at, row.at)
		execOrFatal(t, dbConn, `INSERT INTO task_session_commits (id, session_id, commit_sha, committed_at, files_changed, insertions, deletions, created_at) VALUES (?, ?, ?, ?, 1, 1, 0, ?)`,
			"commit-"+row.suffix, "sess-"+row.suffix, "sha-"+row.suffix, row.at, row.at)
	}

	t.Run("model usage", func(t *testing.T) {
		usage, err := repo.GetModelUsage(ctx, "ws-1", 5, &threshold)
		if err != nil {
			t.Fatalf("GetModelUsage failed: %v", err)
		}
		if len(usage) != 1 || usage[0].SessionCount != 1 {
			t.Fatalf("expected the one session started after the boundary, got %+v", usage)
		}
	})

	t.Run("global stats", func(t *testing.T) {
		stats, err := repo.GetGlobalStats(ctx, "ws-1", &threshold)
		if err != nil {
			t.Fatalf("GetGlobalStats failed: %v", err)
		}
		if stats.TotalTasks != 1 {
			t.Errorf("expected 1 task created after the boundary, got %d", stats.TotalTasks)
		}
		if stats.TotalSessions != 1 {
			t.Errorf("expected 1 session started after the boundary, got %d", stats.TotalSessions)
		}
	})

	t.Run("task stats", func(t *testing.T) {
		stats, err := repo.GetTaskStats(ctx, "ws-1", &threshold, 10)
		if err != nil {
			t.Fatalf("GetTaskStats failed: %v", err)
		}
		if len(stats) != 1 || stats[0].TaskID != "task-after" || stats[0].SessionCount != 1 {
			t.Fatalf("expected only the task created after the boundary, got %+v", stats)
		}
	})

	t.Run("repository stats", func(t *testing.T) {
		stats, err := repo.GetRepositoryStats(ctx, "ws-1", &threshold)
		if err != nil {
			t.Fatalf("GetRepositoryStats failed: %v", err)
		}
		if len(stats) != 1 || stats[0].TotalTasks != 1 || stats[0].SessionCount != 1 || stats[0].TotalCommits != 1 {
			t.Fatalf("expected only boundary-day repository activity, got %+v", stats)
		}
	})

	t.Run("git stats", func(t *testing.T) {
		stats, err := repo.GetGitStats(ctx, "ws-1", &threshold)
		if err != nil {
			t.Fatalf("GetGitStats failed: %v", err)
		}
		if stats.TotalCommits != 1 {
			t.Errorf("expected 1 commit made after the boundary, got %d", stats.TotalCommits)
		}
	})

	t.Run("open ended range keeps both", func(t *testing.T) {
		stats, err := repo.GetGlobalStats(ctx, "ws-1", nil)
		if err != nil {
			t.Fatalf("GetGlobalStats failed: %v", err)
		}
		if stats.TotalTasks != 2 || stats.TotalSessions != 2 {
			t.Errorf("expected both rows with no range, got %d tasks / %d sessions", stats.TotalTasks, stats.TotalSessions)
		}
	})
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

// Automation runs are agent output, not workspace throughput. They used to fall
// out of these aggregates for free because they were ephemeral; now that they
// are ordinary tasks tagged by origin, the exclusion has to be explicit or a
// daily automation quietly inflates a workspace's task count by 365 a year.
func TestGetGlobalStats_ExcludesAutomationRuns(t *testing.T) {
	dbConn := createTestDB(t)
	repo, err := NewWithDB(dbConn, dbConn)
	if err != nil {
		t.Fatalf("NewWithDB failed: %v", err)
	}

	ctx := context.Background()
	nowStr := time.Now().UTC().Format(time.RFC3339)

	execOrFatal(t, dbConn, `INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'Test', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO boards (id, workspace_id, name, created_at, updated_at) VALUES ('board-1', 'ws-1', 'Board', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at) VALUES ('step-todo', 'board-1', 'To Do', 0, ?, ?)`, nowStr, nowStr)

	// A human task and an automation run, both non-ephemeral now.
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, origin, created_at, updated_at) VALUES ('task-human', 'ws-1', 'board-1', 'step-todo', 'Human', 0, '', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO tasks (id, workspace_id, board_id, workflow_step_id, title, is_ephemeral, origin, created_at, updated_at) VALUES ('task-automation', 'ws-1', 'board-1', 'step-todo', 'Nightly drift', 0, 'automation_run', ?, ?)`, nowStr, nowStr)

	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-human', 'task-human', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)
	execOrFatal(t, dbConn, `INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at) VALUES ('sess-automation', 'task-automation', 'agent-1', 'COMPLETED', ?, ?)`, nowStr, nowStr)

	stats, err := repo.GetGlobalStats(ctx, "ws-1", nil)
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}
	if stats.TotalTasks != 1 {
		t.Errorf("expected 1 task (automation run excluded), got %d", stats.TotalTasks)
	}
	if stats.TotalSessions != 1 {
		t.Errorf("expected 1 session (automation run excluded), got %d", stats.TotalSessions)
	}
}
