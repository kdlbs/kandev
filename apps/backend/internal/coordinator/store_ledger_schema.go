package coordinator

import (
	"fmt"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// ledgerColumnMigrations add the turn link columns to tables phases 2 and 3
// created.
var ledgerColumnMigrations = []struct{ name, stmt string }{
	{"coordinator_proposals.turn_id", `ALTER TABLE coordinator_proposals ADD COLUMN turn_id TEXT`},
	{"coordinator_activity.turn_id", `ALTER TABLE coordinator_activity ADD COLUMN turn_id TEXT`},
	{"coordinator_unattended_turns.ledger_turn_id", `ALTER TABLE coordinator_unattended_turns ADD COLUMN ledger_turn_id TEXT`},
}

const ledgerTablesSQL = `
	CREATE TABLE IF NOT EXISTS coordinator_turns (
		id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		session_turn_id TEXT NOT NULL,
		"trigger" TEXT NOT NULL,
		wake_kinds TEXT NOT NULL DEFAULT '[]',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		harness TEXT NOT NULL DEFAULT '',
		config_revision INTEGER NOT NULL DEFAULT 0,
		policy_revision INTEGER NOT NULL DEFAULT 0,
		prompt_hash TEXT NOT NULL DEFAULT '',
		snapshot_hash TEXT NOT NULL DEFAULT '',
		watch_scope TEXT NOT NULL DEFAULT 'all',
		watch_ids TEXT NOT NULL DEFAULT '[]',
		started_at {{timestamp}} NOT NULL,
		finished_at {{timestamp}},
		outcome TEXT,
		verdict TEXT,
		calls_truncated {{boolean}} NOT NULL DEFAULT FALSE
	);

	CREATE TABLE IF NOT EXISTS coordinator_turn_snapshots (
		hash TEXT PRIMARY KEY,
		body TEXT NOT NULL,
		created_at {{timestamp}} NOT NULL
	);
`

const ledgerCallsSQL = `
	CREATE TABLE IF NOT EXISTS coordinator_turn_calls (
		id %s,
		turn_id TEXT NOT NULL,
		action TEXT NOT NULL,
		target_task_id TEXT,
		allowed {{boolean}} NOT NULL,
		recorded_at {{timestamp}} NOT NULL
	);
`

const ledgerIndexesSQL = `
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_turns_session_turn ON coordinator_turns(session_id, session_turn_id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_turns_list ON coordinator_turns(coordinator_id, started_at, id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_turn_calls_turn ON coordinator_turn_calls(turn_id, id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_proposals_turn ON coordinator_proposals(turn_id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_activity_turn ON coordinator_activity(turn_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_unattended_turns_ledger ON coordinator_unattended_turns(ledger_turn_id) WHERE ledger_turn_id IS NOT NULL;
`

// migrateLedger applies the additive turn ledger schema. Every step is
// replayable on SQLite and PostgreSQL.
func (s *Store) migrateLedger(migrate *db.MigrateLogger) error {
	driver := s.db.DriverName()
	for _, m := range ledgerColumnMigrations {
		if err := migrate.Apply(m.name, dialect.MustRenderSchema(driver, m.stmt)); err != nil {
			return fmt.Errorf("coordinator ledger column %s: %w", m.name, err)
		}
	}
	identity := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if dialect.IsPostgres(driver) {
		identity = "BIGSERIAL PRIMARY KEY"
	}
	for _, stmt := range []string{ledgerTablesSQL, fmt.Sprintf(ledgerCallsSQL, identity), ledgerIndexesSQL} {
		if _, err := s.db.Exec(dialect.MustRenderSchema(driver, stmt)); err != nil {
			return fmt.Errorf("coordinator ledger schema: %w", err)
		}
	}
	return nil
}
