package sqlite

// Slice 1 (turn-start step stamp) adds no schema change: it's a new key in
// task_session_turns.metadata, an existing JSON column. This test pins the
// spec's scenario "a turn created before activation, when read after
// activation, carries no stamp and is not backfilled" — a pre-activation row
// is untouched by boot/replay, and there is no migration step to reverse.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

func TestPreActivationTurnIsNotBackfilledWithStepStamp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "step-stamp-migration.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pre-stamp", "ws-pre-stamp", "Pre-existing task", now, now); err != nil {
		t.Fatalf("seed pre-existing task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "session-pre-stamp", "task-pre-stamp", "COMPLETED", now, now); err != nil {
		t.Fatalf("seed pre-existing session: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), "turn-pre-stamp", "session-pre-stamp", "task-pre-stamp", now, `{"runtime_config_snapshot":{"model":"legacy"}}`, now, now); err != nil {
		t.Fatalf("seed pre-activation turn: %v", err)
	}

	// Replay schema init/migrations twice more, mirroring a boot after
	// activation. Nothing about this table's rows should change.
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations twice: %v", err)
	}

	turn, err := repo.GetTurn(t.Context(), "turn-pre-stamp")
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if _, ok := turn.Metadata["workflow_step_id_at_start"]; ok {
		t.Fatalf("pre-activation turn was backfilled with a step stamp: %v", turn.Metadata)
	}
	if turn.Metadata["runtime_config_snapshot"] == nil {
		t.Fatalf("pre-activation turn metadata lost its existing content: %v", turn.Metadata)
	}
}
