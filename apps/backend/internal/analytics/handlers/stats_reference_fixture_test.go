package handlers

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	analyticsqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/db"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

const statsReferenceWorkspaceID = "ws-reference"

type statsFixtureWorkload struct {
	tasks    int
	sessions int
	turns    int
	messages int
	commits  int
	repos    int
}

var (
	statsReferenceWorkload = statsFixtureWorkload{
		tasks: 700, sessions: 800, turns: 5000, messages: 650000, commits: 2200, repos: 5,
	}
	statsAvailabilityWorkload = statsFixtureWorkload{
		tasks: 20000, sessions: 20000, turns: 20000, messages: 100000, commits: 1000, repos: 5,
	}
)

type statsReferenceFixture struct {
	repo        *analyticsqlite.Repository
	writer      *sqlx.DB
	reader      *sqlx.DB
	workspaceID string
}

func newStatsReferenceFixture(
	t testing.TB,
	workload statsFixtureWorkload,
	includeOtherWorkspace bool,
) statsReferenceFixture {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "stats-reference.db")
	rawWriter, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	writer := sqlx.NewDb(rawWriter, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := tasksqlite.NewWithDB(writer, writer, nil); err != nil {
		t.Fatalf("task repository schema: %v", err)
	}
	setReferenceBulkLoadMode(t, writer)
	base := time.Now().UTC().Truncate(24 * time.Hour)
	seedStatsReferenceWorkspace(t, writer, statsReferenceWorkspaceID, base, workload)
	if includeOtherWorkspace {
		seedStatsReferenceWorkspace(t, writer, "ws-other-reference", base, workload)
	}
	restoreReferenceDatabaseMode(t, writer)
	rawReader, err := db.OpenSQLiteReader(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteReader: %v", err)
	}
	reader := sqlx.NewDb(rawReader, "sqlite3")
	reader.SetMaxOpenConns(4)
	reader.SetMaxIdleConns(4)
	t.Cleanup(func() { _ = reader.Close() })
	repo, err := analyticsqlite.NewWithDB(writer, reader)
	if err != nil {
		t.Fatalf("analytics repository: %v", err)
	}
	return statsReferenceFixture{
		repo:        repo,
		writer:      writer,
		reader:      reader,
		workspaceID: statsReferenceWorkspaceID,
	}
}

func setReferenceBulkLoadMode(t testing.TB, writer *sqlx.DB) {
	t.Helper()
	if _, err := writer.Exec(
		`PRAGMA synchronous=OFF; PRAGMA journal_mode=MEMORY; PRAGMA foreign_keys=OFF`,
	); err != nil {
		t.Fatalf("set reference database bulk-load mode: %v", err)
	}
}

func restoreReferenceDatabaseMode(t testing.TB, writer *sqlx.DB) {
	t.Helper()
	if _, err := writer.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA foreign_keys=ON`); err != nil {
		t.Errorf("restore reference database mode: %v", err)
	}
}

func seedStatsReferenceWorkspace(
	t testing.TB,
	writer *sqlx.DB,
	workspaceID string,
	base time.Time,
	workload statsFixtureWorkload,
) {
	t.Helper()
	baseValue := base.Format(time.RFC3339)
	execReferenceFixture(t, writer,
		`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		workspaceID, workspaceID, base, base,
	)
	execReferenceFixture(t, writer, `
		WITH RECURSIVE numbers(n) AS (
			SELECT 1 UNION ALL SELECT n + 1 FROM numbers WHERE n < ?
		)
		INSERT INTO repositories (id, workspace_id, name, created_at, updated_at)
		SELECT ? || '-repo-' || n, ?, ? || '-repo-' || n, ?, ? FROM numbers`,
		workload.repos, workspaceID, workspaceID, workspaceID, base, base)
	execReferenceFixture(t, writer, `
		WITH RECURSIVE numbers(n) AS (
			SELECT 1 UNION ALL SELECT n + 1 FROM numbers WHERE n < ?
		)
		INSERT INTO tasks (
			id, workspace_id, workflow_id, workflow_step_id, title,
			state, is_ephemeral, origin, metadata, created_at, updated_at
		)
		SELECT
			? || '-task-' || n, ?, '', '', 'Reference task',
			'TODO', 0, '', '{}',
			?, ?
		FROM numbers`,
		workload.tasks, workspaceID, workspaceID, baseValue, baseValue)
	execReferenceFixture(t, writer, `
		WITH RECURSIVE numbers(n) AS (
			SELECT 1 UNION ALL SELECT n + 1 FROM numbers WHERE n < ?
		)
		INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at)
		SELECT
			? || '-task-repo-' || n,
			? || '-task-' || n,
			? || '-repo-' || (((n - 1) % ?) + 1), ?, ?
		FROM numbers`,
		workload.tasks, workspaceID, workspaceID, workspaceID,
		workload.repos, base, base)
	execReferenceFixture(t, writer, `
		WITH RECURSIVE numbers(n) AS (
			SELECT 1 UNION ALL SELECT n + 1 FROM numbers WHERE n < ?
		)
		INSERT INTO task_sessions (
			id, task_id, agent_profile_id, agent_profile_snapshot, repository_id,
			state, started_at, completed_at, updated_at
		)
		SELECT
			? || '-session-' || n,
			? || '-task-' || (((n - 1) % ?) + 1),
			'availability-agent', '{"model":"reference-model"}',
			? || '-repo-' || (((n - 1) % ?) + 1), 'COMPLETED',
			?, ?, ?
		FROM numbers`,
		workload.sessions, workspaceID, workspaceID, workload.tasks,
		workspaceID, workload.repos, baseValue, baseValue, baseValue)
	execReferenceFixture(t, writer, `
		WITH RECURSIVE numbers(n) AS (
			SELECT 1 UNION ALL SELECT n + 1 FROM numbers WHERE n < ?
		)
		INSERT INTO task_session_turns (
			id, task_session_id, task_id, started_at, completed_at, created_at, updated_at
		)
		SELECT
			? || '-turn-' || n,
			? || '-session-' || (((n - 1) % ?) + 1),
			? || '-task-' || (((n - 1) % ?) + 1),
			?, ?, ?, ?
		FROM numbers`,
		workload.turns, workspaceID, workspaceID, workload.sessions,
		workspaceID, workload.tasks, baseValue, baseValue, baseValue, baseValue)
	execReferenceFixture(t, writer, `
		WITH RECURSIVE numbers(n) AS (
			SELECT 1 UNION ALL SELECT n + 1 FROM numbers WHERE n < ?
		)
		INSERT INTO task_session_messages (
			id, task_session_id, turn_id, author_type, type, content, created_at
		)
		SELECT
			? || '-message-' || n,
			? || '-session-' || (((n - 1) % ?) + 1),
			? || '-turn-' || (((n - 1) % ?) + 1),
			CASE WHEN n % 5 = 0 THEN 'agent' ELSE 'user' END,
			CASE WHEN n % 7 = 0 THEN 'tool_call' ELSE 'message' END,
			'reference', ?
		FROM numbers`,
		workload.messages, workspaceID, workspaceID, workload.sessions,
		workspaceID, workload.turns, baseValue)
	execReferenceFixture(t, writer, `
		WITH RECURSIVE numbers(n) AS (
			SELECT 1 UNION ALL SELECT n + 1 FROM numbers WHERE n < ?
		)
		INSERT INTO task_session_commits (
			id, session_id, commit_sha, committed_at, files_changed,
			insertions, deletions, created_at
		)
		SELECT
			? || '-commit-' || n,
			? || '-session-' || (((n - 1) % ?) + 1),
			? || '-sha-' || n,
			?, 1, 2, 1, ?
		FROM numbers`,
		workload.commits, workspaceID, workspaceID, workload.sessions,
		workspaceID, baseValue, baseValue)
}

func execReferenceFixture(t testing.TB, writer *sqlx.DB, query string, args ...any) {
	t.Helper()
	if _, err := writer.Exec(query, args...); err != nil {
		t.Fatalf("reference fixture SQL failed: %v", err)
	}
}
