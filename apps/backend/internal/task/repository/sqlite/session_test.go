package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
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

func TestRenameTaskSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	if err := repo.RenameTaskSession(ctx, "missing-session", "reviewer"); !errors.Is(err, models.ErrTaskSessionNotFound) {
		t.Fatalf("RenameTaskSession error = %v, want ErrTaskSessionNotFound", err)
	}

	seedForMsgTest(t, repo, "task-rename", "session-rename", "turn-rename")
	if err := repo.RenameTaskSession(ctx, "session-rename", "reviewer"); err != nil {
		t.Fatalf("RenameTaskSession: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-rename")
	if err != nil {
		t.Fatalf("GetTaskSession after rename: %v", err)
	}
	if session.Name != "reviewer" {
		t.Fatalf("session.Name = %q, want %q", session.Name, "reviewer")
	}
	if got := session.ToAPI()["name"]; got != "reviewer" {
		t.Fatalf(`ToAPI()["name"] = %v, want "reviewer"`, got)
	}

	// Clearing the name falls back to the derived tab title on the frontend.
	if err := repo.RenameTaskSession(ctx, "session-rename", ""); err != nil {
		t.Fatalf("RenameTaskSession clear: %v", err)
	}
	session, err = repo.GetTaskSession(ctx, "session-rename")
	if err != nil {
		t.Fatalf("GetTaskSession after clear: %v", err)
	}
	if session.Name != "" {
		t.Fatalf("session.Name = %q, want empty after clear", session.Name)
	}
	if _, ok := session.ToAPI()["name"]; ok {
		t.Fatalf("ToAPI() should omit name when empty")
	}

	// Name survives CreateTaskSession round-trips and list scans.
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-named", TaskID: "task-rename", Name: "verifier",
	}); err != nil {
		t.Fatalf("CreateTaskSession with name: %v", err)
	}
	sessions, err := repo.ListTaskSessions(ctx, "task-rename")
	if err != nil {
		t.Fatalf("ListTaskSessionsByTaskID: %v", err)
	}
	found := false
	for _, s := range sessions {
		if s.ID == "session-named" {
			found = true
			if s.Name != "verifier" {
				t.Fatalf("listed session Name = %q, want %q", s.Name, "verifier")
			}
		}
	}
	if !found {
		t.Fatalf("session-named not returned by ListTaskSessions")
	}
}

func TestTaskSessionNotFoundErrorsAreTyped(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	if _, err := repo.GetTaskSession(ctx, "missing-session"); !errors.Is(err, models.ErrTaskSessionNotFound) {
		t.Fatalf("GetTaskSession error = %v, want ErrTaskSessionNotFound", err)
	}
	if err := repo.UpdateTaskSession(ctx, &models.TaskSession{ID: "missing-session"}); !errors.Is(err, models.ErrTaskSessionNotFound) {
		t.Fatalf("UpdateTaskSession error = %v, want ErrTaskSessionNotFound", err)
	}
	if err := repo.UpdateTaskSessionState(ctx, "missing-session", models.TaskSessionStateCompleted, ""); !errors.Is(err, models.ErrTaskSessionNotFound) {
		t.Fatalf("UpdateTaskSessionState error = %v, want ErrTaskSessionNotFound", err)
	}
	if err := repo.UpdateTaskSessionBaseCommit(ctx, "missing-session", "abc123"); !errors.Is(err, models.ErrTaskSessionNotFound) {
		t.Fatalf("UpdateTaskSessionBaseCommit error = %v, want ErrTaskSessionNotFound", err)
	}

	session, err := repo.GetTaskSessionByTaskAndAgent(ctx, "task-missing", "agent-missing")
	if err != nil {
		t.Fatalf("GetTaskSessionByTaskAndAgent should translate not found to nil, nil: %v", err)
	}
	if session != nil {
		t.Fatalf("GetTaskSessionByTaskAndAgent session = %#v, want nil", session)
	}
	if _, err := repo.GetPrimarySessionByTaskID(ctx, "task-missing"); !errors.Is(err, ErrNoPrimarySession) {
		t.Fatalf("GetPrimarySessionByTaskID error = %v, want ErrNoPrimarySession", err)
	}

	seedForMsgTest(t, repo, "task-found", "session-found", "turn-found")
	if _, err := repo.GetTaskSession(ctx, "session-found"); err != nil {
		t.Fatalf("GetTaskSession existing row: %v", err)
	}
	if err := repo.UpdateTaskSessionState(ctx, "session-found", models.TaskSessionStateCompleted, ""); err != nil {
		t.Fatalf("UpdateTaskSessionState existing row: %v", err)
	}
}

func TestSetSessionMetadataKeyIfAbsentSQLiteIsWriteOnce(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-baseline", "session-baseline", "turn-baseline")
	ctx := context.Background()

	stored, err := repo.SetSessionMetadataKeyIfAbsent(ctx, "session-baseline", "baseline", map[string]string{"effort": "high"})
	if err != nil {
		t.Fatalf("first SetSessionMetadataKeyIfAbsent: %v", err)
	}
	if !stored {
		t.Fatal("first SetSessionMetadataKeyIfAbsent should store")
	}
	stored, err = repo.SetSessionMetadataKeyIfAbsent(ctx, "session-baseline", "baseline", map[string]string{"effort": "low"})
	if err != nil {
		t.Fatalf("second SetSessionMetadataKeyIfAbsent: %v", err)
	}
	if stored {
		t.Fatal("second SetSessionMetadataKeyIfAbsent should not overwrite")
	}

	session, err := repo.GetTaskSession(ctx, "session-baseline")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	baseline, ok := session.Metadata["baseline"].(map[string]interface{})
	if !ok || baseline["effort"] != "high" {
		t.Fatalf("baseline = %#v, want effort=high", session.Metadata["baseline"])
	}
}

func TestSetSessionMetadataKeyIfAbsentQueryUsesPostgresJSONB(t *testing.T) {
	query := setSessionMetadataKeyIfAbsentQuery(dialect.PGX)
	if strings.Contains(query, "json_set") || strings.Contains(query, "json_type") || strings.Contains(query, "json(?)") {
		t.Fatalf("postgres write-once query uses SQLite JSON functions: %s", query)
	}
	if !strings.Contains(query, "jsonb_set") || !strings.Contains(query, "jsonb_extract_path") {
		t.Fatalf("postgres write-once query must use JSONB set/existence operations: %s", query)
	}
}

func TestListTaskSessionWorktreesFiltersInactiveRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-worktrees", "session-worktrees", "turn-worktrees")
	worktrees := []*models.TaskSessionWorktree{
		{
			ID:           "wt-active",
			SessionID:    "session-worktrees",
			WorktreeID:   "worktree-active",
			RepositoryID: "repo-1",
			BranchSlug:   "main",
		},
		{
			ID:           "wt-status-deleted",
			SessionID:    "session-worktrees",
			WorktreeID:   "worktree-status-deleted",
			RepositoryID: "repo-1",
			BranchSlug:   "deleted-status",
		},
		{
			ID:           "wt-timestamp-deleted",
			SessionID:    "session-worktrees",
			WorktreeID:   "worktree-timestamp-deleted",
			RepositoryID: "repo-1",
			BranchSlug:   "deleted-at",
		},
	}
	for _, wt := range worktrees {
		if err := repo.CreateTaskSessionWorktree(ctx, wt); err != nil {
			t.Fatalf("CreateTaskSessionWorktree(%s): %v", wt.ID, err)
		}
	}
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE task_session_worktrees
		SET status = 'deleted', updated_at = ?
		WHERE id = ?
	`), now, "wt-status-deleted"); err != nil {
		t.Fatalf("mark status deleted: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE task_session_worktrees
		SET deleted_at = ?, updated_at = ?
		WHERE id = ?
	`), now, now, "wt-timestamp-deleted"); err != nil {
		t.Fatalf("mark timestamp deleted: %v", err)
	}

	listed, err := repo.ListTaskSessionWorktrees(ctx, "session-worktrees")
	if err != nil {
		t.Fatalf("ListTaskSessionWorktrees: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "wt-active" {
		t.Fatalf("ListTaskSessionWorktrees = %+v, want only wt-active", listed)
	}
	batched, err := repo.ListWorktreesBySessionIDs(ctx, []string{"session-worktrees"})
	if err != nil {
		t.Fatalf("ListWorktreesBySessionIDs: %v", err)
	}
	rows := batched["session-worktrees"]
	if len(rows) != 1 || rows[0].ID != "wt-active" {
		t.Fatalf("ListWorktreesBySessionIDs = %+v, want only wt-active", rows)
	}
}

func TestUpdateTaskSessionWorktreeBranchByRepositoryScopesUpdate(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-worktrees", "session-worktrees", "turn-worktrees")

	worktrees := []*models.TaskSessionWorktree{
		{
			ID:             "wt-repo-1",
			SessionID:      "session-worktrees",
			WorktreeID:     "worktree-repo-1",
			RepositoryID:   "repo-1",
			WorktreeBranch: "feature/old-one",
		},
		{
			ID:             "wt-repo-2",
			SessionID:      "session-worktrees",
			WorktreeID:     "worktree-repo-2",
			RepositoryID:   "repo-2",
			WorktreeBranch: "feature/old-two",
		},
	}
	for _, wt := range worktrees {
		if err := repo.CreateTaskSessionWorktree(ctx, wt); err != nil {
			t.Fatalf("CreateTaskSessionWorktree(%s): %v", wt.ID, err)
		}
	}

	if err := repo.UpdateTaskSessionWorktreeBranchByRepository(ctx, "session-worktrees", "repo-1", "feature/new-one"); err != nil {
		t.Fatalf("UpdateTaskSessionWorktreeBranchByRepository: %v", err)
	}

	listed, err := repo.ListTaskSessionWorktrees(ctx, "session-worktrees")
	if err != nil {
		t.Fatalf("ListTaskSessionWorktrees: %v", err)
	}
	branches := map[string]string{}
	for _, wt := range listed {
		branches[wt.RepositoryID] = wt.WorktreeBranch
	}
	if branches["repo-1"] != "feature/new-one" {
		t.Fatalf("repo-1 branch = %q, want feature/new-one", branches["repo-1"])
	}
	if branches["repo-2"] != "feature/old-two" {
		t.Fatalf("repo-2 branch = %q, want feature/old-two", branches["repo-2"])
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
// counts sessions in active or resumable states (CREATED, STARTING, RUNNING,
// IDLE, WAITING_FOR_INPUT) and excludes sessions in terminal states.
func TestCountActiveTaskSessionsByRepository_CountsActiveOnly(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	// Two active sessions across two tasks linked to the repo.
	seedRepoLink(t, repo, "ws-a", "repo-a", "task-a1", "sess-a1", "RUNNING")
	seedRepoLink(t, repo, "ws-a", "repo-a", "task-a2", "sess-a2", "WAITING_FOR_INPUT")
	seedRepoLink(t, repo, "ws-a", "repo-a", "task-a4", "sess-a4", "IDLE")
	// Terminal-state session linked to the repo — must NOT count.
	seedRepoLink(t, repo, "ws-a", "repo-a", "task-a3", "sess-a3", "COMPLETED")
	// Active session linked to a different repo — must NOT count.
	seedRepoLink(t, repo, "ws-a", "repo-b", "task-b1", "sess-b1", "RUNNING")

	count, err := repo.CountActiveTaskSessionsByRepository(ctx, "repo-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 active or resumable sessions, got %d", count)
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

// archiveTask marks a seeded task as archived so the repo-delete guard tests
// can exercise the archived_at exclusion.
func archiveTask(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(
		`UPDATE tasks SET archived_at = ? WHERE id = ?`), time.Now().UTC(), taskID); err != nil {
		t.Fatalf("archive task %s: %v", taskID, err)
	}
}

// TestCountActiveTaskSessionsByRepository_ExcludesArchivedTasks verifies that an
// active session belonging to an archived task is not counted, so archived tasks
// never block repository deletion, while a live task's active session still is.
func TestCountActiveTaskSessionsByRepository_ExcludesArchivedTasks(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	// Active session on an archived task — must NOT count.
	seedRepoLink(t, repo, "ws-x", "repo-x", "task-x1", "sess-x1", "WAITING_FOR_INPUT")
	archiveTask(t, repo, "task-x1")

	count, err := repo.CountActiveTaskSessionsByRepository(ctx, "repo-x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("archived task must not block deletion, got %d active sessions", count)
	}

	// A live (non-archived) task with an active session on the same repo still
	// counts — pins that the archived_at filter did not over-broaden.
	seedRepoLink(t, repo, "ws-x", "repo-x", "task-x2", "sess-x2", "RUNNING")
	count, err = repo.CountActiveTaskSessionsByRepository(ctx, "repo-x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("live task session must still count, got %d", count)
	}
}

// TestHasActiveTaskSessionsByRepository_ExcludesArchivedTasks verifies the
// boolean delete guard mirrors the count: an archived task's active session
// does not report the repository as in use, but a live task's does.
func TestHasActiveTaskSessionsByRepository_ExcludesArchivedTasks(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedRepoLink(t, repo, "ws-h", "repo-h", "task-h1", "sess-h1", "WAITING_FOR_INPUT")
	archiveTask(t, repo, "task-h1")

	active, err := repo.HasActiveTaskSessionsByRepository(ctx, "repo-h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("archived task must not mark repository as having active sessions")
	}

	// Add a live task on the same repo — now it must report active.
	seedRepoLink(t, repo, "ws-h", "repo-h", "task-h2", "sess-h2", "RUNNING")
	active, err = repo.HasActiveTaskSessionsByRepository(ctx, "repo-h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("live task session must mark repository as active")
	}
}

// insertSession inserts a task_session row in the given state directly, for
// seeding multiple sessions on a single task (seedRepoLink can only create one
// session per task because its task_repositories PK is keyed on task+repo).
func insertSession(t *testing.T, repo *Repository, sessionID, taskID, state string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), sessionID, taskID, state, now, now); err != nil {
		t.Fatalf("seed session %s: %v", sessionID, err)
	}
}

func sessionState(t *testing.T, repo *Repository, sessionID string) string {
	t.Helper()
	var state string
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT state FROM task_sessions WHERE id = ?`), sessionID).Scan(&state); err != nil {
		t.Fatalf("read state for %s: %v", sessionID, err)
	}
	return state
}

func TestCancelActiveTaskSessionIsTerminalSafe(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-cas", "session-running", "turn-cas")
	if err := repo.UpdateTaskSessionState(ctx, "session-running", models.TaskSessionStateRunning, ""); err != nil {
		t.Fatalf("seed running state: %v", err)
	}
	insertSession(t, repo, "session-completed", "task-cas", string(models.TaskSessionStateCompleted))

	changed, cancelledAt, err := repo.CancelActiveTaskSession(ctx, "session-running", "coordinator stop")
	if err != nil {
		t.Fatalf("cancel running session: %v", err)
	}
	if !changed {
		t.Fatal("running session was not cancelled")
	}
	if got := sessionState(t, repo, "session-running"); got != string(models.TaskSessionStateCancelled) {
		t.Fatalf("running session state = %q, want CANCELLED", got)
	}
	cancelled, err := repo.GetTaskSession(ctx, "session-running")
	if err != nil {
		t.Fatalf("read cancelled session: %v", err)
	}
	if !cancelled.UpdatedAt.Equal(cancelledAt) {
		t.Fatalf("cancel timestamp = %s, stored updated_at = %s", cancelledAt, cancelled.UpdatedAt)
	}

	changed, _, err = repo.CancelActiveTaskSession(ctx, "session-completed", "coordinator stop")
	if err != nil {
		t.Fatalf("cancel completed session: %v", err)
	}
	if changed {
		t.Fatal("completed session reported a cancellation")
	}
	if got := sessionState(t, repo, "session-completed"); got != string(models.TaskSessionStateCompleted) {
		t.Fatalf("completed session state = %q, want COMPLETED", got)
	}
}

func TestUpdateTaskSessionStateIfCurrentRejectsStaleActiveWriter(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-state-cas", "session-state-cas", "turn-state-cas")
	if err := repo.UpdateTaskSessionState(ctx, "session-state-cas", models.TaskSessionStateRunning, ""); err != nil {
		t.Fatalf("seed running state: %v", err)
	}
	if changed, _, err := repo.CancelActiveTaskSession(ctx, "session-state-cas", "coordinator stop"); err != nil || !changed {
		t.Fatalf("cancel session: changed=%v err=%v", changed, err)
	}

	changed, _, err := repo.UpdateTaskSessionStateIfCurrent(
		ctx,
		"session-state-cas",
		models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput,
		"",
	)
	if err != nil {
		t.Fatalf("stale conditional update: %v", err)
	}
	if changed {
		t.Fatal("stale RUNNING writer changed a CANCELLED session")
	}
	if got := sessionState(t, repo, "session-state-cas"); got != string(models.TaskSessionStateCancelled) {
		t.Fatalf("session state = %q, want CANCELLED", got)
	}
}

func TestUpdateTaskSessionIfCurrentStateRejectsStaleFullRowWriter(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-full-row-cas", "session-full-row-cas", "turn-full-row-cas")
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx, "session-full-row-cas", models.TaskSessionStateRunning, "",
	))
	stale, err := repo.GetTaskSession(ctx, "session-full-row-cas")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, stale.State)
	changed, _, err := repo.CancelActiveTaskSession(ctx, stale.ID, "stopped by parent task via MCP")
	require.NoError(t, err)
	require.True(t, changed)

	stale.State = models.TaskSessionStateStarting
	stale.ExecutorID = "late-executor"
	changed, err = repo.UpdateTaskSessionIfCurrentState(
		ctx, stale, models.TaskSessionStateRunning,
	)
	require.NoError(t, err)
	require.False(t, changed)
	stored, err := repo.GetTaskSession(ctx, stale.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, stored.State)
	require.Empty(t, stored.ExecutorID)
}

func TestUpdateTaskSessionWithMetadataRejectsInvalidMetadataBeforeStateWrite(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedForMsgTest(t, repo, "task-atomic", "sess-atomic", "turn-atomic")
	if err := repo.UpdateSessionMetadata(ctx, "sess-atomic", map[string]interface{}{"keep": "yes"}); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "sess-atomic")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateFailed

	err = repo.UpdateTaskSessionWithMetadata(ctx, session, map[string]interface{}{"bad": func() {}})
	if err == nil {
		t.Fatal("expected invalid metadata error")
	}
	if got := sessionState(t, repo, "sess-atomic"); got == string(models.TaskSessionStateFailed) {
		t.Fatalf("state was partially updated to %q", got)
	}
	got, err := repo.GetTaskSession(ctx, "sess-atomic")
	if err != nil {
		t.Fatalf("get session after failed update: %v", err)
	}
	if got.Metadata["keep"] != "yes" {
		t.Fatalf("metadata keep = %v, want yes", got.Metadata["keep"])
	}
}

func TestUpdateTaskSessionIfCurrentStateRemovingMetadataKeys(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-remove-metadata", "sess-remove-metadata", "turn-remove-metadata")
	require.NoError(t, repo.UpdateSessionMetadata(ctx, "sess-remove-metadata", map[string]interface{}{
		"provider_state": "stale",
		"keep":           "newer",
	}))
	session, err := repo.GetTaskSession(ctx, "sess-remove-metadata")
	require.NoError(t, err)
	changed, err := repo.UpdateTaskSessionIfCurrentStateRemovingMetadataKeys(
		ctx,
		session,
		models.TaskSessionStateCreated,
		[]string{"provider_state"},
	)
	require.NoError(t, err)
	require.True(t, changed)
	stored, err := repo.GetTaskSession(ctx, "sess-remove-metadata")
	require.NoError(t, err)
	require.NotContains(t, stored.Metadata, "provider_state")
	require.Equal(t, "newer", stored.Metadata["keep"])
}

func TestUpdateTaskSessionIfCurrentStateRemovingMetadataKeysStateMismatch(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-remove-metadata-mismatch", "sess-remove-metadata-mismatch", "turn-remove-metadata-mismatch")
	require.NoError(t, repo.UpdateSessionMetadata(ctx, "sess-remove-metadata-mismatch", map[string]interface{}{
		"provider_state": "stale",
		"keep":           "untouched",
	}))
	session, err := repo.GetTaskSession(ctx, "sess-remove-metadata-mismatch")
	require.NoError(t, err)
	changed, err := repo.UpdateTaskSessionIfCurrentStateRemovingMetadataKeys(
		ctx,
		session,
		models.TaskSessionStateRunning,
		[]string{"provider_state"},
	)
	require.NoError(t, err)
	require.False(t, changed)
	stored, err := repo.GetTaskSession(ctx, "sess-remove-metadata-mismatch")
	require.NoError(t, err)
	require.Equal(t, "stale", stored.Metadata["provider_state"])
	require.Equal(t, "untouched", stored.Metadata["keep"])
}

func TestDismissLastAgentErrorDoesNotOverwriteNewerError(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedForMsgTest(t, repo, "task-error", "sess-error", "turn-error")
	oldErr := models.LastAgentError{
		Message:    "old error",
		OccurredAt: time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC),
	}
	newErr := models.LastAgentError{
		Message:    "new error",
		OccurredAt: time.Date(2026, 6, 14, 10, 5, 0, 0, time.UTC),
	}
	if err := repo.SetSessionMetadataKey(ctx, "sess-error", models.SessionMetaKeyLastAgentError, oldErr); err != nil {
		t.Fatalf("seed old error: %v", err)
	}
	if err := repo.SetSessionMetadataKey(ctx, "sess-error", models.SessionMetaKeyLastAgentError, newErr); err != nil {
		t.Fatalf("seed new error: %v", err)
	}

	updated, err := repo.DismissLastAgentError(ctx, "sess-error", oldErr, time.Now().UTC())
	if err != nil {
		t.Fatalf("dismiss stale error: %v", err)
	}
	if updated {
		t.Fatalf("expected stale dismiss to be ignored")
	}
	session, err := repo.GetTaskSession(ctx, "sess-error")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	got, ok := models.LoadLastAgentError(session.Metadata)
	if !ok {
		t.Fatalf("expected last agent error metadata")
	}
	if got.Message != newErr.Message || got.IsDismissed() {
		t.Fatalf("last agent error = %#v, want undismissed newer error", got)
	}
}

func TestDismissLastAgentErrorMatchesEquivalentTimestampText(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedForMsgTest(t, repo, "task-error", "sess-error", "turn-error")
	occurredAt, err := time.Parse(time.RFC3339Nano, "2026-06-14T12:00:00.310Z")
	if err != nil {
		t.Fatalf("parse occurred_at: %v", err)
	}
	lastErr := models.LastAgentError{
		Message:    "peer disconnected before response",
		OccurredAt: occurredAt,
	}
	if err := repo.SetSessionMetadataKey(ctx, "sess-error", models.SessionMetaKeyLastAgentError, map[string]any{
		"message":     lastErr.Message,
		"occurred_at": "2026-06-14T12:00:00.310Z",
	}); err != nil {
		t.Fatalf("seed last agent error: %v", err)
	}

	updated, err := repo.DismissLastAgentError(ctx, "sess-error", lastErr, time.Now().UTC())
	if err != nil {
		t.Fatalf("dismiss last agent error: %v", err)
	}
	if !updated {
		t.Fatalf("expected equivalent timestamp text to match")
	}
	session, err := repo.GetTaskSession(ctx, "sess-error")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	got, ok := models.LoadLastAgentError(session.Metadata)
	if !ok {
		t.Fatalf("expected last agent error metadata")
	}
	if !got.IsDismissed() {
		t.Fatalf("last agent error = %#v, want dismissed", got)
	}
}

func sessionCancellationMetadata(t *testing.T, repo *Repository, sessionID string) (string, sql.NullTime, time.Time) {
	t.Helper()
	var errorMessage string
	var completedAt sql.NullTime
	var updatedAt time.Time
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT error_message, completed_at, updated_at FROM task_sessions WHERE id = ?`),
		sessionID,
	).Scan(&errorMessage, &completedAt, &updatedAt); err != nil {
		t.Fatalf("read cancellation metadata for %s: %v", sessionID, err)
	}
	return errorMessage, completedAt, updatedAt
}

func assertReapedSession(t *testing.T, repo *Repository, sessionID string, reapedAfter time.Time) {
	t.Helper()
	if got := sessionState(t, repo, sessionID); got != "CANCELLED" {
		t.Errorf("%s = %q, want CANCELLED", sessionID, got)
	}
	errorMessage, completedAt, updatedAt := sessionCancellationMetadata(t, repo, sessionID)
	if errorMessage != "task archived" {
		t.Errorf("%s error_message = %q, want task archived", sessionID, errorMessage)
	}
	if !completedAt.Valid {
		t.Errorf("%s completed_at should be set", sessionID)
	}
	if updatedAt.Before(reapedAfter) {
		t.Errorf("%s updated_at = %s, want >= %s", sessionID, updatedAt, reapedAfter)
	}
}

// TestCancelActiveTaskSessionsByTaskID verifies the archive reaper transitions
// only the target task's still-active sessions to CANCELLED, leaves terminal
// sessions and other tasks untouched, and reports the rows changed. It also
// confirms the repo-delete guard reports the repository as free afterward —
// the end-to-end purpose of the reap.
func TestCancelActiveTaskSessionsByTaskID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	// Target task: one active session via the link helper, plus a second active
	// session, two pre-run sessions, and an already-terminal session inserted directly.
	seedRepoLink(t, repo, "ws-r", "repo-r", "task-r", "sess-r1", "WAITING_FOR_INPUT")
	insertSession(t, repo, "sess-r2", "task-r", "RUNNING")
	insertSession(t, repo, "sess-r3", "task-r", "CREATED")
	insertSession(t, repo, "sess-r4", "task-r", "STARTING")
	insertSession(t, repo, "sess-r5", "task-r", "COMPLETED")
	// A different task on a different repo — must be untouched.
	seedRepoLink(t, repo, "ws-r", "repo-other", "task-other", "sess-o1", "RUNNING")

	reapedAfter := time.Now().UTC()
	reaped, err := repo.CancelActiveTaskSessionsByTaskID(ctx, "task-r", "task archived")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reaped != 4 {
		t.Errorf("expected 4 active sessions reaped, got %d", reaped)
	}
	for _, sessionID := range []string{"sess-r1", "sess-r2", "sess-r3", "sess-r4"} {
		assertReapedSession(t, repo, sessionID, reapedAfter)
	}
	if got := sessionState(t, repo, "sess-r5"); got != "COMPLETED" {
		t.Errorf("sess-r5 (terminal) = %q, want unchanged COMPLETED", got)
	}
	if got := sessionState(t, repo, "sess-o1"); got != "RUNNING" {
		t.Errorf("sess-o1 (other task) = %q, want unchanged RUNNING", got)
	}

	// End-to-end: the repository that only the reaped task referenced is now
	// reported as free by the delete guard.
	active, err := repo.HasActiveTaskSessionsByRepository(ctx, "repo-r")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("repo-r should have no active sessions after reaping its task")
	}

	// Idempotent: a second call changes nothing.
	reaped, err = repo.CancelActiveTaskSessionsByTaskID(ctx, "task-r", "task archived")
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if reaped != 0 {
		t.Errorf("expected 0 rows on idempotent re-run, got %d", reaped)
	}
}
