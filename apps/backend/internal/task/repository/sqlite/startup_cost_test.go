package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	dbpkg "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/mattn/go-sqlite3"
)

// TestStartupMigrationCosts records fresh-schema and replay costs on an
// isolated SQLite file. The output is evidence for startup budgeting; it does
// not impose a machine-specific duration threshold.
func TestStartupMigrationCosts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "startup-costs.db")

	freshDB := openStartupCostDB(t, dbPath)
	t.Cleanup(func() { _ = freshDB.Close() })
	freshStarted := time.Now()
	if _, err := NewWithDB(freshDB, freshDB, nil); err != nil {
		t.Fatalf("initialize fresh schema: %v", err)
	}
	freshDuration := time.Since(freshStarted)
	if err := freshDB.Close(); err != nil {
		t.Fatalf("close fresh database: %v", err)
	}

	replayDB := openStartupCostDB(t, dbPath)
	t.Cleanup(func() { _ = replayDB.Close() })
	replayStarted := time.Now()
	if _, err := NewWithDB(replayDB, replayDB, nil); err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	replayDuration := time.Since(replayStarted)
	if err := replayDB.Close(); err != nil {
		t.Fatalf("close replay database: %v", err)
	}

	t.Logf("startup migration cost: fresh=%s replay=%s", freshDuration, replayDuration)
}

// TestStartupMigrationCostsPopulated measures startup against a repeatable
// legacy-shaped fixture with enough messages for the two recurring backfills
// to do real work. The fixture is intentionally local and synthetic: its
// purpose is to expose the relative costs and the backup/liveness boundary,
// not to predict a particular production machine's total startup time.
func TestStartupMigrationCostsPopulated(t *testing.T) {
	const (
		sessionCount       = 24
		messagesPerSession = 250
	)
	dbPath := filepath.Join(t.TempDir(), "startup-costs-populated.db")

	freshDB := openStartupCostDB(t, dbPath)
	freshStarted := time.Now()
	if _, err := NewWithDB(freshDB, freshDB, nil); err != nil {
		t.Fatalf("initialize fresh schema: %v", err)
	}
	freshDuration := time.Since(freshStarted)
	if err := freshDB.Close(); err != nil {
		t.Fatalf("close fresh database: %v", err)
	}

	seedDB := openStartupCostDB(t, dbPath)
	if err := makePopulatedLegacyMessageFixture(seedDB, sessionCount, messagesPerSession); err != nil {
		_ = seedDB.Close()
		t.Fatalf("prepare populated legacy fixture: %v", err)
	}
	messageCount := sessionCount * messagesPerSession
	var measuredMessageCount int
	if err := seedDB.Get(&measuredMessageCount, `SELECT COUNT(*) FROM task_session_messages`); err != nil {
		_ = seedDB.Close()
		t.Fatalf("count fixture messages: %v", err)
	}
	if measuredMessageCount != messageCount {
		_ = seedDB.Close()
		t.Fatalf("fixture message count = %d, want %d", measuredMessageCount, messageCount)
	}

	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		_ = seedDB.Close()
		t.Fatalf("stat populated fixture: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "startup-costs-backup.db")
	backupStarted := time.Now()
	backupBytes, err := persistence.SnapshotSQLite(seedDB, backupPath)
	backupDuration := time.Since(backupStarted)
	if err != nil {
		_ = seedDB.Close()
		t.Fatalf("snapshot populated fixture: %v", err)
	}
	if err := seedDB.Close(); err != nil {
		t.Fatalf("close seeded database: %v", err)
	}

	replayDB := openStartupCostDB(t, dbPath)
	replayStarted := time.Now()
	replayRepo, err := NewWithDB(replayDB, replayDB, nil)
	if err != nil {
		_ = replayDB.Close()
		t.Fatalf("replay populated schema: %v", err)
	}
	replayDuration := time.Since(replayStarted)

	// Measure the two recurring data migrations separately on the same
	// populated database. Reset statements are excluded from the timings.
	if _, err := replayDB.Exec(`UPDATE task_session_messages SET updated_at = NULL`); err != nil {
		_ = replayDB.Close()
		t.Fatalf("reset updated_at fixture: %v", err)
	}
	updatedAtStarted := time.Now()
	if err := dbpkg.NewRequiredMigrateLogger(replayDB, nil).Apply(
		"evidence.task_session_messages.updated_at.backfill",
		`UPDATE task_session_messages SET updated_at = created_at WHERE updated_at IS NULL`,
	); err != nil {
		_ = replayDB.Close()
		t.Fatalf("measure updated_at backfill: %v", err)
	}
	updatedAtDuration := time.Since(updatedAtStarted)

	if _, err := replayDB.Exec(`UPDATE task_session_messages SET prompt_seq = 0`); err != nil {
		_ = replayDB.Close()
		t.Fatalf("reset prompt sequence fixture: %v", err)
	}
	if _, err := replayDB.Exec(`DELETE FROM task_session_prompt_seq`); err != nil {
		_ = replayDB.Close()
		t.Fatalf("reset prompt sequence counters: %v", err)
	}
	promptSeqStarted := time.Now()
	if err := replayRepo.backfillPromptSeq(); err != nil {
		_ = replayDB.Close()
		t.Fatalf("measure prompt sequence backfill: %v", err)
	}
	promptSeqDuration := time.Since(promptSeqStarted)
	if err := replayDB.Close(); err != nil {
		t.Fatalf("close replay database: %v", err)
	}

	t.Logf("populated startup evidence: sessions=%d messages=%d db_bytes=%d backup_bytes=%d fresh_schema=%s backup=%s replay=%s updated_at_backfill=%s prompt_seq_backfill=%s environment=%s/%s go=%s cpus=%d",
		sessionCount, messageCount, dbInfo.Size(), backupBytes, freshDuration, backupDuration,
		replayDuration, updatedAtDuration, promptSeqDuration, runtime.GOOS, runtime.GOARCH,
		runtime.Version(), runtime.NumCPU())
}

// makePopulatedLegacyMessageFixture turns the current empty schema into the
// shape that exercises the recurring startup migrations: updated_at is present
// but nullable, and prompt_seq is absent. The rebuild is isolated to this
// measurement test and does not alter production migration behavior.
func makePopulatedLegacyMessageFixture(db *sqlx.DB, sessionCount, messagesPerSession int) error {
	for _, index := range []string{
		"idx_messages_session_id",
		"idx_messages_created_at",
		"idx_messages_session_created",
		"idx_messages_turn_id",
		"idx_messages_task_author_created",
		"idx_messages_session_updated",
		"idx_messages_metadata_tool_call_id",
		"idx_messages_metadata_pending_id",
		"idx_messages_metadata_pending_id_lookup",
		"idx_messages_metadata_pending_id_lookup_ordered",
	} {
		if _, err := db.Exec("DROP INDEX IF EXISTS " + index); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE task_session_messages RENAME TO startup_messages_current`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE task_session_messages (
			id TEXT PRIMARY KEY,
			task_session_id TEXT NOT NULL,
			task_id TEXT DEFAULT '',
			turn_id TEXT NOT NULL,
			author_type TEXT NOT NULL DEFAULT 'user',
			author_id TEXT DEFAULT '',
			content TEXT NOT NULL,
			requests_input INTEGER DEFAULT 0,
			type TEXT NOT NULL DEFAULT 'message',
			metadata TEXT DEFAULT '{}',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP,
			FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE,
			FOREIGN KEY (turn_id) REFERENCES task_session_turns(id) ON DELETE CASCADE
		)`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at)
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
		FROM startup_messages_current`); err != nil {
		return err
	}
	if _, err := db.Exec(`DROP TABLE startup_messages_current`); err != nil {
		return err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return err
	}

	var workspaceID, workflowID string
	if err := db.QueryRow(`SELECT id FROM workspaces ORDER BY created_at LIMIT 1`).Scan(&workspaceID); err != nil {
		return err
	}
	if err := db.QueryRow(`SELECT id FROM workflows ORDER BY created_at LIMIT 1`).Scan(&workflowID); err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	baseTime := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	for sessionIndex := 0; sessionIndex < sessionCount; sessionIndex++ {
		taskID := "startup-evidence-task-" + strconv.Itoa(sessionIndex)
		sessionID := "startup-evidence-session-" + strconv.Itoa(sessionIndex)
		turnID := "startup-evidence-turn-" + strconv.Itoa(sessionIndex)
		now := baseTime.Add(time.Duration(sessionIndex) * time.Hour)
		if _, err := tx.Exec(`INSERT INTO tasks (id, workspace_id, workflow_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, taskID, workspaceID, workflowID, taskID, now, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO task_sessions (id, task_id, started_at, updated_at) VALUES (?, ?, ?, ?)`, sessionID, taskID, now, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, turnID, sessionID, taskID, now, now, now); err != nil {
			return err
		}
		for messageIndex := 0; messageIndex < messagesPerSession; messageIndex++ {
			authorType := "agent"
			if messageIndex%3 == 0 {
				authorType = "user"
			}
			createdAt := now.Add(time.Duration(messageIndex) * time.Microsecond)
			messageID := "startup-evidence-message-" + strconv.Itoa(sessionIndex) + "-" + strconv.Itoa(messageIndex)
			if _, err := tx.Exec(`
				INSERT INTO task_session_messages
					(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, '', 'startup evidence', 0, 'message', '{}', ?, NULL)
			`, messageID, sessionID, taskID, turnID, authorType, createdAt); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func openStartupCostDB(t *testing.T, path string) *sqlx.DB {
	t.Helper()
	conn, err := dbpkg.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	return sqlx.NewDb(conn, "sqlite3")
}

// TestBackfillPromptSeqStopsOnCanceledContext exercises a real recurring
// migration while its UPDATE is executing. The trigger blocks the statement
// at the SQLite driver boundary so cancellation is deterministic and does not
// depend on a timer racing a particular database size.
func TestBackfillPromptSeqStopsOnCanceledContext(t *testing.T) {
	db := openStartupCostDB(t, filepath.Join(t.TempDir(), "prompt-cancel.db"))
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		CREATE TABLE task_session_messages (
			id TEXT PRIMARY KEY,
			task_session_id TEXT NOT NULL,
			author_type TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			prompt_seq INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE task_session_prompt_seq (
			task_session_id TEXT PRIMARY KEY,
			last_seq INTEGER NOT NULL
		);
		INSERT INTO task_session_messages(id, task_session_id, author_type, created_at, prompt_seq)
		VALUES ('message-1', 'session-1', 'user', '2026-01-01T00:00:00Z', 0);
	`); err != nil {
		t.Fatalf("seed prompt sequence fixture: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve SQLite connection: %v", err)
	}
	if err := conn.Raw(func(driverConn any) error {
		sqliteConn, ok := driverConn.(*sqlite3.SQLiteConn)
		if !ok {
			t.Fatalf("driver connection = %T, want *sqlite3.SQLiteConn", driverConn)
		}
		return sqliteConn.RegisterFunc("wait_for_prompt_release", func() int64 {
			startedOnce.Do(func() { close(started) })
			<-release
			return 1
		}, true)
	}); err != nil {
		_ = conn.Close()
		t.Fatalf("register SQLite function: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("release SQLite connection: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER block_prompt_sequence_update
		BEFORE UPDATE OF prompt_seq ON task_session_messages
		BEGIN
			SELECT wait_for_prompt_release();
		END;
	`); err != nil {
		t.Fatalf("create blocking trigger: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	repo := &Repository{
		db:      db,
		migrate: dbpkg.NewRequiredMigrateLoggerContext(db, nil, ctx),
	}
	done := make(chan error, 1)
	go func() { done <- repo.backfillPromptSeq() }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt sequence migration did not enter the active statement")
	}
	cancel()
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("prompt sequence migration error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt sequence migration did not stop after cancellation")
	}
}

// TestNewWithDBContextStopsActiveMigration proves that cancellation reaches
// the real repository constructor at the shared startup boundary. The
// trigger blocks the production prompt-sequence UPDATE, then cancellation
// must abort that constructor before it can admit a later schema step.
func TestNewWithDBContextStopsActiveMigration(t *testing.T) {
	db := openStartupCostDB(t, filepath.Join(t.TempDir(), "constructor-cancel.db"))
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	seedForMsgTest(t, repo, "constructor-task", "constructor-session", "constructor-turn")
	var workspaceID, workflowID string
	if err := db.QueryRow(`SELECT id FROM workspaces ORDER BY created_at LIMIT 1`).Scan(&workspaceID); err != nil {
		t.Fatalf("read workspace: %v", err)
	}
	if err := db.QueryRow(`SELECT id FROM workflows ORDER BY created_at LIMIT 1`).Scan(&workflowID); err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	if _, err := db.Exec(`UPDATE tasks SET workspace_id = ?, workflow_id = ? WHERE id = ?`, workspaceID, workflowID, "constructor-task"); err != nil {
		t.Fatalf("make constructor fixture replayable: %v", err)
	}
	insertPromptRow(t, repo, "constructor-message", "constructor-session", "constructor-turn", "user", "startup cancellation", time.Now().UTC())

	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve SQLite connection: %v", err)
	}
	if err := conn.Raw(func(driverConn any) error {
		sqliteConn, ok := driverConn.(*sqlite3.SQLiteConn)
		if !ok {
			t.Fatalf("driver connection = %T, want *sqlite3.SQLiteConn", driverConn)
		}
		return sqliteConn.RegisterFunc("wait_for_constructor_release", func() int64 {
			startedOnce.Do(func() { close(started) })
			<-release
			return 1
		}, true)
	}); err != nil {
		_ = conn.Close()
		t.Fatalf("register SQLite function: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("release SQLite connection: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER block_constructor_prompt_sequence
		BEFORE UPDATE OF prompt_seq ON task_session_messages
		BEGIN
			SELECT wait_for_constructor_release();
		END;
	`); err != nil {
		t.Fatalf("create blocking trigger: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewWithDBContext(ctx, db, db, nil)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("repository constructor did not enter the active migration")
	}
	cancel()
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("constructor error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("repository constructor did not stop after cancellation")
	}
}
