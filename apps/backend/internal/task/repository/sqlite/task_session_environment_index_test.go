package sqlite

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

const taskSessionEnvironmentTaskIndex = "idx_task_sessions_environment_task"

// TestTaskSessionEnvironmentTaskIndexFreshUpgradeReplay covers the additive
// startup path for the delivery environment lookup index: fresh initialization,
// an existing database missing only the index, an identical operator-created
// index, both legacy session-table rebuilds, and a pre-column database.
func TestTaskSessionEnvironmentTaskIndexFreshUpgradeReplay(t *testing.T) {
	t.Run("fresh and replay", func(t *testing.T) {
		repo := newRepoForSessionTests(t)
		assertSQLiteTaskSessionEnvironmentIndex(t, repo)

		replayed, err := NewWithDB(repo.db, repo.db, nil)
		if err != nil {
			t.Fatalf("replay schema: %v", err)
		}
		assertSQLiteTaskSessionEnvironmentIndex(t, replayed)
	})

	t.Run("existing database without index", func(t *testing.T) {
		db, fixture := newSessionIndexRepo(t, "")
		fixture.seed(t, db, true)

		if _, err := db.Exec("DROP INDEX " + taskSessionEnvironmentTaskIndex); err != nil {
			t.Fatalf("drop environment index: %v", err)
		}
		upgraded, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("upgrade schema: %v", err)
		}
		assertSQLiteTaskSessionEnvironmentIndex(t, upgraded)
		fixture.assertPreserved(t, db)
	})

	t.Run("identical operator index", func(t *testing.T) {
		db, fixture := newSessionIndexRepo(t, "")
		fixture.seed(t, db, true)

		if _, err := db.Exec("DROP INDEX " + taskSessionEnvironmentTaskIndex); err != nil {
			t.Fatalf("drop environment index: %v", err)
		}
		if _, err := db.Exec(`CREATE INDEX idx_task_sessions_environment_task
			ON task_sessions(task_environment_id, task_id)`); err != nil {
			t.Fatalf("create operator index: %v", err)
		}
		replayed, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("replay operator index: %v", err)
		}
		assertSQLiteTaskSessionEnvironmentIndex(t, replayed)
		fixture.assertPreserved(t, db)
	})

	t.Run("workflow step rebuild", func(t *testing.T) {
		db, fixture := newSessionIndexRepo(t, legacySessionTableWorkflowStep)
		fixture.seed(t, db, true)

		upgraded, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("rebuild legacy workflow step schema: %v", err)
		}
		assertSQLiteTaskSessionEnvironmentIndex(t, upgraded)
		fixture.assertPreserved(t, db)
	})

	t.Run("agent execution rebuild", func(t *testing.T) {
		db, fixture := newSessionIndexRepo(t, legacySessionTableAgentExecution)
		fixture.seed(t, db, true)

		upgraded, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("rebuild legacy agent execution schema: %v", err)
		}
		assertSQLiteTaskSessionEnvironmentIndex(t, upgraded)
		fixture.assertPreserved(t, db)
	})

	t.Run("pre-column database", func(t *testing.T) {
		db, fixture := newSessionIndexRepo(t, legacySessionTablePreColumn)
		fixture.seed(t, db, false)

		upgraded, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("initialize pre-column schema: %v", err)
		}
		assertSQLiteTaskSessionEnvironmentIndex(t, upgraded)
		fixture.assertPreserved(t, db)
	})
}

// newSessionIndexRepo creates a fully initialized database and optionally
// replaces the session table with the supplied legacy shape. Each call owns a
// separate database.
func newSessionIndexRepo(t *testing.T, tableDDL string) (*sqlx.DB, sessionIndexFixture) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "session-environment-index.db")
	rawConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(rawConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("initialize database: %v", err)
	}
	if tableDDL != "" {
		replaceTaskSessionsTable(t, db, tableDDL)
	}

	startedAt := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	return db, sessionIndexFixture{
		workspaceID: "ws-session-index",
		taskID:      "task-session-index",
		environment: "env-session-index",
		sessionID:   "session-session-index",
		snapshotID:  "snapshot-session-index",
		startedAt:   startedAt,
		updatedAt:   startedAt.Add(time.Minute),
	}
}

func replaceTaskSessionsTable(t *testing.T, db *sqlx.DB, tableDDL string) {
	t.Helper()
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`PRAGMA foreign_keys=ON`) })
	dropSQLiteTriggers(t, db)
	if _, err := db.Exec(`DROP TABLE task_sessions`); err != nil {
		t.Fatalf("drop task sessions: %v", err)
	}
	if _, err := db.Exec(tableDDL); err != nil {
		t.Fatalf("create legacy task sessions: %v", err)
	}
}

// dropSQLiteTriggers removes the conversation triggers that reference
// task_sessions. Startup recreates them after the session rebuilds, so leaving
// them in place would make the fixture fail during the intermediate drop.
func dropSQLiteTriggers(t *testing.T, db *sqlx.DB) {
	t.Helper()
	var names []string
	if err := db.Select(&names, `SELECT name FROM sqlite_master WHERE type = 'trigger'`); err != nil {
		t.Fatalf("list sqlite triggers: %v", err)
	}
	for _, name := range names {
		if _, err := db.Exec(`DROP TRIGGER IF EXISTS "` + name + `"`); err != nil {
			t.Fatalf("drop sqlite trigger %s: %v", name, err)
		}
	}
}

type sessionIndexFixture struct {
	workspaceID string
	taskID      string
	environment string
	sessionID   string
	snapshotID  string
	startedAt   time.Time
	updatedAt   time.Time
}

func (f sessionIndexFixture) seed(t *testing.T, db *sqlx.DB, hasEnvironmentColumn bool) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), f.workspaceID, f.workspaceID, f.startedAt, f.startedAt); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`), f.taskID, f.workspaceID, f.taskID, f.startedAt, f.startedAt); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environments (id, task_id, status, created_at, updated_at) VALUES (?, ?, 'ready', ?, ?)
	`), f.environment, f.taskID, f.startedAt, f.startedAt); err != nil {
		t.Fatalf("seed environment: %v", err)
	}
	query := `
		INSERT INTO task_sessions
			(id, task_id, state, started_at, updated_at, executor_actor_user_id)
		VALUES (?, ?, 'RUNNING', ?, ?, 'actor-session-index')
	`
	args := []any{f.sessionID, f.taskID, f.startedAt, f.updatedAt}
	if hasEnvironmentColumn {
		query = `
			INSERT INTO task_sessions
				(id, task_id, state, started_at, updated_at, task_environment_id, executor_actor_user_id)
			VALUES (?, ?, 'RUNNING', ?, ?, ?, 'actor-session-index')
		`
		args = append(args, f.environment)
	}
	if _, err := db.Exec(db.Rebind(query), args...); err != nil {
		t.Fatalf("seed legacy session: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_git_snapshots
			(id, task_environment_id, session_id, snapshot_type, branch, head_commit, ahead, metadata, created_at)
		VALUES (?, ?, ?, 'test', 'main', 'sentinel', 1, '{}', ?)
	`), f.snapshotID, f.environment, f.sessionID, f.updatedAt); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
}

func (f sessionIndexFixture) assertPreserved(t *testing.T, db *sqlx.DB) {
	t.Helper()
	var row struct {
		TaskID      string    `db:"task_id"`
		Environment string    `db:"task_environment_id"`
		State       string    `db:"state"`
		StartedAt   time.Time `db:"started_at"`
		UpdatedAt   time.Time `db:"updated_at"`
	}
	if err := db.Get(&row, db.Rebind(`
		SELECT task_id, task_environment_id, state, started_at, updated_at
		FROM task_sessions WHERE id = ?
	`), f.sessionID); err != nil {
		t.Fatalf("read preserved session: %v", err)
	}
	// A pre-column table could not store the binding at all; startup healing
	// owns the established recovery outcome and points it at the task's
	// environment. Tables that did store it preserve the same value.
	if row.TaskID != f.taskID || row.Environment != f.environment || row.State != "RUNNING" ||
		!row.StartedAt.Equal(f.startedAt) || !row.UpdatedAt.Equal(f.updatedAt) {
		t.Fatalf("preserved session = %+v, want task %q environment %q state RUNNING timestamps %v/%v",
			row, f.taskID, f.environment, f.startedAt, f.updatedAt)
	}

	var snapshot struct {
		Environment string    `db:"task_environment_id"`
		SessionID   string    `db:"session_id"`
		CreatedAt   time.Time `db:"created_at"`
	}
	if err := db.Get(&snapshot, db.Rebind(`
		SELECT task_environment_id, session_id, created_at
		FROM task_session_git_snapshots WHERE id = ?
	`), f.snapshotID); err != nil {
		t.Fatalf("read preserved snapshot: %v", err)
	}
	if snapshot.Environment != f.environment || snapshot.SessionID != f.sessionID || !snapshot.CreatedAt.Equal(f.updatedAt) {
		t.Fatalf("preserved snapshot = %+v, want environment %q session %q created_at %v",
			snapshot, f.environment, f.sessionID, f.updatedAt)
	}

	var sessions int
	if err := db.Get(&sessions, `SELECT COUNT(*) FROM task_sessions`); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 1 {
		t.Fatalf("session count = %d, want 1", sessions)
	}
}

func assertSQLiteTaskSessionEnvironmentIndex(t *testing.T, repo *Repository) {
	t.Helper()
	var names []string
	if err := repo.db.Select(&names, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, taskSessionEnvironmentTaskIndex); err != nil {
		t.Fatalf("list environment indexes: %v", err)
	}
	if len(names) != 1 {
		t.Fatalf("environment indexes = %v, want exactly one", names)
	}
	var unique int
	if err := repo.db.Get(&unique, `SELECT "unique" FROM pragma_index_list('task_sessions') WHERE name = ?`, taskSessionEnvironmentTaskIndex); err != nil {
		t.Fatalf("read index uniqueness: %v", err)
	}
	if unique != 0 {
		t.Fatalf("environment index unique = %d, want 0", unique)
	}
	var keys []string
	if err := repo.db.Select(&keys, `SELECT name FROM pragma_index_info(?) ORDER BY seqno`, taskSessionEnvironmentTaskIndex); err != nil {
		t.Fatalf("read index keys: %v", err)
	}
	if got := strings.Join(keys, ","); got != "task_environment_id,task_id" {
		t.Fatalf("environment index keys = %q, want %q", got, "task_environment_id,task_id")
	}
}

const legacySessionColumns = `
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	container_id TEXT NOT NULL DEFAULT '',
	agent_profile_id TEXT,
	execution_profile_id TEXT NOT NULL DEFAULT '',
	route_generation INTEGER NOT NULL DEFAULT 0,
	route_state TEXT NOT NULL DEFAULT '',
	route_reason TEXT NOT NULL DEFAULT '',
	downstream_acp_session_id TEXT NOT NULL DEFAULT '',
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
	cost_subcents INTEGER NOT NULL DEFAULT 0,
	tokens_in INTEGER NOT NULL DEFAULT 0,
	tokens_cached_in BIGINT NOT NULL DEFAULT 0,
	tokens_out INTEGER NOT NULL DEFAULT 0,
	executor_actor_user_id TEXT NOT NULL DEFAULT '',`

var (
	legacySessionTableWorkflowStep = fmt.Sprintf(`CREATE TABLE task_sessions (
		%s
		task_environment_id TEXT DEFAULT '',
		workflow_step_id TEXT NOT NULL DEFAULT '',
		agent_execution_id TEXT NOT NULL DEFAULT '',
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	)`, legacySessionColumns)

	legacySessionTableAgentExecution = fmt.Sprintf(`CREATE TABLE task_sessions (
		%s
		task_environment_id TEXT DEFAULT '',
		agent_execution_id TEXT NOT NULL DEFAULT '',
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	)`, legacySessionColumns)

	legacySessionTablePreColumn = fmt.Sprintf(`CREATE TABLE task_sessions (
		%s
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	)`, legacySessionColumns)
)
