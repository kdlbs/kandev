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
	repo, err := NewWithDB(sqlxDB, sqlxDB)
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
