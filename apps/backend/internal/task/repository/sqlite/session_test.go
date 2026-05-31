package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func newRepoForSessionTests(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "session-test.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo
}

// seedForMsgTest seeds task, session, and turn rows so that all FK constraints
// on task_session_messages are satisfied. Returns the turn ID for use in inserts.
func seedForMsgTest(t *testing.T, repo *Repository, taskID, sessionID, turnID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT OR IGNORE INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, '', 'test task', ?, ?)
	`), taskID, now, now)
	if err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT OR IGNORE INTO task_sessions
			(id, task_id, started_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), sessionID, taskID, now, now)
	if err != nil {
		t.Fatalf("seed session %s: %v", sessionID, err)
	}
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT OR IGNORE INTO task_session_turns
			(id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), turnID, sessionID, taskID, now, now, now)
	if err != nil {
		t.Fatalf("seed turn %s: %v", turnID, err)
	}
}

// insertAgentMsg inserts a message row directly into the DB under the given
// session and turn. authorType must be 'agent' or 'user'.
func insertAgentMsg(t *testing.T, repo *Repository, id, sessionID, turnID, authorType, content string, ts time.Time) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, '', ?, ?, '', ?, 0, 'message', '{}', ?)
	`), id, sessionID, turnID, authorType, content, ts)
	if err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
}

// TestGetLastAgentMessage_NoMessages verifies that a session with no messages
// returns an empty string and sql.ErrNoRows.
func TestGetLastAgentMessage_NoMessages(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	msg, err := repo.GetLastAgentMessage(ctx, "sess-empty")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
	if msg != "" {
		t.Errorf("expected empty string, got %q", msg)
	}
}

// TestGetLastAgentMessage_MessagesAllEmptyContent verifies that when the agent
// message has empty content the function returns "" without error (content
// column allows empty string).
func TestGetLastAgentMessage_MessagesAllEmptyContent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedForMsgTest(t, repo, "task-ec", "sess-ec", "turn-ec")
	insertAgentMsg(t, repo, "msg-ec-1", "sess-ec", "turn-ec", "agent", "", time.Now().UTC())

	msg, err := repo.GetLastAgentMessage(ctx, "sess-ec")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "" {
		t.Errorf("expected empty string for empty-content message, got %q", msg)
	}
}

// TestGetLastAgentMessage_ReturnsLatestAgentMessage verifies that the most
// recent agent message is returned, and that user messages are ignored.
func TestGetLastAgentMessage_ReturnsLatestAgentMessage(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedForMsgTest(t, repo, "task-1", "sess-1", "turn-1")

	base := time.Now().UTC()
	// User message — must be ignored by GetLastAgentMessage.
	insertAgentMsg(t, repo, "msg-u-1", "sess-1", "turn-1", "user", "user question", base)
	// First agent message.
	insertAgentMsg(t, repo, "msg-a-1", "sess-1", "turn-1", "agent", "first agent reply", base.Add(time.Second))
	// Second (latest) agent message — this must be returned.
	insertAgentMsg(t, repo, "msg-a-2", "sess-1", "turn-1", "agent", "second agent reply", base.Add(2*time.Second))

	msg, err := repo.GetLastAgentMessage(ctx, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "second agent reply" {
		t.Errorf("expected 'second agent reply', got %q", msg)
	}
}

// TestGetLastAgentMessage_SessionDoesNotExist verifies that looking up a
// session that has no messages returns an empty string and sql.ErrNoRows.
func TestGetLastAgentMessage_SessionDoesNotExist(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	msg, err := repo.GetLastAgentMessage(ctx, "sess-nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
	if msg != "" {
		t.Errorf("expected empty string for non-existent session, got %q", msg)
	}
}

// TestIncrementTaskSessionUsage_AccumulatesAcrossCalls confirms multiple
// calls compound onto the same row. The DB-only columns are seeded via
// the migration's CREATE TABLE defaults (zero) and bumped via the
// UPDATE in the helper.
func TestIncrementTaskSessionUsage_AccumulatesAcrossCalls(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-usage", "sess-usage", "turn-usage")

	if err := repo.IncrementTaskSessionUsage(ctx, "sess-usage", 100, 200, 50); err != nil {
		t.Fatalf("first increment: %v", err)
	}
	if err := repo.IncrementTaskSessionUsage(ctx, "sess-usage", 10, 20, 5); err != nil {
		t.Fatalf("second increment: %v", err)
	}

	var tokensIn, tokensOut, costSubcents int64
	err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_in, tokens_out, cost_subcents FROM task_sessions WHERE id = ?`),
		"sess-usage").Scan(&tokensIn, &tokensOut, &costSubcents)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if tokensIn != 110 || tokensOut != 220 || costSubcents != 55 {
		t.Errorf("totals = (%d,%d,%d), want (110,220,55)", tokensIn, tokensOut, costSubcents)
	}
}

// TestIncrementTaskSessionUsage_UnknownSessionNoError tolerates a
// missing row (subscriber may race against session creation).
func TestIncrementTaskSessionUsage_UnknownSessionNoError(t *testing.T) {
	repo := newRepoForSessionTests(t)
	if err := repo.IncrementTaskSessionUsage(context.Background(), "no-such", 1, 2, 3); err != nil {
		t.Errorf("expected no error for unknown session, got %v", err)
	}
}

// TestIncrementTaskSessionUsage_EmptySessionIDNoOp guards against the
// orchestrator publishing a usage event before SessionID is set.
func TestIncrementTaskSessionUsage_EmptySessionIDNoOp(t *testing.T) {
	repo := newRepoForSessionTests(t)
	if err := repo.IncrementTaskSessionUsage(context.Background(), "", 1, 2, 3); err != nil {
		t.Errorf("empty session id should be a no-op, got %v", err)
	}
}

// rebuildSessionsWithoutCostColumns drops and recreates task_sessions with the
// post-migration schema MINUS the cost/token columns (cost_subcents, tokens_in,
// tokens_out) and without the agent_execution_id / workflow_step_id trigger
// columns. This reproduces a legacy DB that can never gain the cost columns: the
// gated CREATE-TABLE rebuilds won't fire (their trigger columns are absent) and
// the fresh-create is a no-op because the table already exists.
func rebuildSessionsWithoutCostColumns(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable fk: %v", err)
	}
	defer func() { _, _ = repo.db.Exec(`PRAGMA foreign_keys=ON`) }()
	stmts := []string{
		`DROP TABLE task_sessions`,
		`CREATE TABLE task_sessions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			agent_profile_id TEXT,
			executor_id TEXT DEFAULT '',
			executor_profile_id TEXT DEFAULT '',
			environment_id TEXT DEFAULT '',
			repository_id TEXT DEFAULT '',
			base_branch TEXT DEFAULT '',
			agent_profile_snapshot TEXT DEFAULT '{}',
			executor_snapshot TEXT DEFAULT '{}',
			environment_snapshot TEXT DEFAULT '{}',
			repository_snapshot TEXT DEFAULT '{}',
			state TEXT NOT NULL DEFAULT 'CREATED',
			error_message TEXT DEFAULT '',
			metadata TEXT DEFAULT '{}',
			started_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			updated_at TIMESTAMP NOT NULL,
			is_primary INTEGER DEFAULT 0,
			is_passthrough INTEGER DEFAULT 0,
			review_status TEXT DEFAULT '',
			base_commit_sha TEXT DEFAULT '',
			task_environment_id TEXT DEFAULT '',
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`,
	}
	for _, stmt := range stmts {
		if _, err := repo.db.Exec(stmt); err != nil {
			t.Fatalf("rebuild task_sessions: %v", err)
		}
	}
}

// TestMigrateSessionsAddCostColumns_BackfillsLegacySchema reproduces the office
// cost subscriber failure ("no such column: tokens_in"): a task_sessions table
// that predates the cost columns and no longer contains the rebuild trigger
// columns can never gain them, so IncrementTaskSessionUsage fails. The additive
// migration must backfill the columns idempotently.
func TestMigrateSessionsAddCostColumns_BackfillsLegacySchema(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	rebuildSessionsWithoutCostColumns(t, repo)
	seedForMsgTest(t, repo, "task-mig", "sess-mig", "turn-mig")

	// Precondition: this is the reported bug on a legacy schema.
	if err := repo.IncrementTaskSessionUsage(ctx, "sess-mig", 1, 2, 3); err == nil {
		t.Fatal("expected missing-column error before backfill")
	}

	repo.migrateSessionsAddCostColumns()

	if err := repo.IncrementTaskSessionUsage(ctx, "sess-mig", 1, 2, 3); err != nil {
		t.Fatalf("IncrementTaskSessionUsage after backfill: %v", err)
	}

	// Idempotent: a second pass over a table that already has the columns is a no-op.
	repo.migrateSessionsAddCostColumns()
	if err := repo.IncrementTaskSessionUsage(ctx, "sess-mig", 10, 20, 30); err != nil {
		t.Fatalf("IncrementTaskSessionUsage after second pass: %v", err)
	}
}

// seedRepoLink wires up a workspace, repository, task, task_repositories link
// row, and a task_session row in the given state. Used to exercise the join
// in CountActiveTaskSessionsByRepository.
func seedRepoLink(t *testing.T, repo *Repository, workspaceID, repositoryID, taskID, sessionID, state string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT OR IGNORE INTO workspaces (id, name, created_at, updated_at)
		VALUES (?, 'ws', ?, ?)
	`), workspaceID, now, now)
	if err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT OR IGNORE INTO repositories (id, workspace_id, name, created_at, updated_at)
		VALUES (?, ?, 'repo', ?, ?)
	`), repositoryID, workspaceID, now, now)
	if err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT OR IGNORE INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, 'test task', ?, ?)
	`), taskID, workspaceID, now, now)
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "tr-"+taskID+"-"+repositoryID, taskID, repositoryID, now, now)
	if err != nil {
		t.Fatalf("seed task_repositories: %v", err)
	}
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), sessionID, taskID, state, now, now)
	if err != nil {
		t.Fatalf("seed task_session: %v", err)
	}
}

// TestCountActiveTaskSessionsByRepository_NoSessions verifies the count is
// zero when no sessions reference the repository at all.
func TestCountActiveTaskSessionsByRepository_NoSessions(t *testing.T) {
	repo := newRepoForSessionTests(t)
	count, err := repo.CountActiveTaskSessionsByRepository(context.Background(), "repo-empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

// TestCountActiveTaskSessionsByRepository_CountsActiveOnly verifies the join
// counts sessions in active states (CREATED, STARTING, RUNNING,
// WAITING_FOR_INPUT) and excludes sessions in terminal states.
func TestCountActiveTaskSessionsByRepository_CountsActiveOnly(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	// Two active sessions across two tasks linked to the repo.
	seedRepoLink(t, repo, "ws-a", "repo-a", "task-a1", "sess-a1", "RUNNING")
	seedRepoLink(t, repo, "ws-a", "repo-a", "task-a2", "sess-a2", "WAITING_FOR_INPUT")
	// Terminal-state session linked to the repo — must NOT count.
	seedRepoLink(t, repo, "ws-a", "repo-a", "task-a3", "sess-a3", "COMPLETED")
	// Active session linked to a different repo — must NOT count.
	seedRepoLink(t, repo, "ws-a", "repo-b", "task-b1", "sess-b1", "RUNNING")

	count, err := repo.CountActiveTaskSessionsByRepository(ctx, "repo-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 active sessions, got %d", count)
	}
}

// TestCountActiveTaskSessionsByRepository_RequiresJoinRow verifies that a
// session whose task is NOT linked via task_repositories is not counted, even
// if the session is active. This guards against accidentally widening the
// query to use task_sessions.repository_id.
func TestCountActiveTaskSessionsByRepository_RequiresJoinRow(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	// Seed workspace, repo, and a task with the link.
	seedRepoLink(t, repo, "ws-j", "repo-j", "task-j1", "sess-j1", "RUNNING")

	// Seed a second task in the same workspace with an active session, but
	// without inserting a task_repositories row pointing at repo-j.
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-j2', 'ws-j', 'orphan', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed orphan task: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES ('sess-j2', 'task-j2', 'RUNNING', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed orphan session: %v", err)
	}

	count, err := repo.CountActiveTaskSessionsByRepository(ctx, "repo-j")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected only the linked session to be counted, got %d", count)
	}
}
