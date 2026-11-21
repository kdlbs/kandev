package coordinator

import (
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

const notesTablesSQL = `
	CREATE TABLE IF NOT EXISTS coordinator_notes (
		id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		source_item_id TEXT,
		current_version INTEGER NOT NULL,
		created_at {{timestamp}} NOT NULL,
		expires_at {{timestamp}} NOT NULL,
		deleted_at {{timestamp}},
		use_count INTEGER NOT NULL DEFAULT 0,
		last_validated_at {{timestamp}},
		last_result TEXT NOT NULL DEFAULT '',
		last_attempt_at {{timestamp}},
		last_attempt TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS coordinator_note_versions (
		note_id TEXT NOT NULL,
		version INTEGER NOT NULL,
		note_type TEXT NOT NULL,
		body TEXT NOT NULL,
		citations TEXT NOT NULL DEFAULT '[]',
		prov_kind TEXT NOT NULL,
		prov_user_id TEXT NOT NULL DEFAULT '',
		prov_model TEXT NOT NULL DEFAULT '',
		actor_id TEXT NOT NULL DEFAULT '',
		reverted_from INTEGER NOT NULL DEFAULT 0,
		created_at {{timestamp}} NOT NULL,
		PRIMARY KEY (note_id, version)
	);

	CREATE TABLE IF NOT EXISTS coordinator_note_snapshots (
		session_turn_id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		failed INTEGER NOT NULL DEFAULT 0,
		entries TEXT NOT NULL DEFAULT '[]',
		created_at {{timestamp}} NOT NULL
	);

	CREATE TABLE IF NOT EXISTS coordinator_note_receipts (
		note_id TEXT NOT NULL,
		session_turn_id TEXT NOT NULL,
		note_version INTEGER NOT NULL,
		counted INTEGER NOT NULL DEFAULT 0,
		created_at {{timestamp}} NOT NULL,
		PRIMARY KEY (note_id, session_turn_id)
	);

	CREATE TABLE IF NOT EXISTS coordinator_proposal_notes (
		proposal_id TEXT NOT NULL,
		note_id TEXT NOT NULL,
		note_version INTEGER NOT NULL,
		PRIMARY KEY (proposal_id, note_id)
	);
`

const notesIndexesSQL = `
	CREATE INDEX IF NOT EXISTS idx_coordinator_notes_list ON coordinator_notes(coordinator_id, created_at, id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_notes_source ON coordinator_notes(coordinator_id, source_item_id) WHERE source_item_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_coordinator_note_receipts_turn ON coordinator_note_receipts(session_turn_id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_proposal_notes_note ON coordinator_proposal_notes(note_id);
`

// migrateNotes applies the additive agent-tier notes schema; every step is
// replayable on SQLite and PostgreSQL.
func (s *Store) migrateNotes() error {
	for _, stmt := range []string{notesTablesSQL, notesIndexesSQL} {
		if _, err := s.db.Exec(dialect.MustRenderSchema(s.db.DriverName(), stmt)); err != nil {
			return fmt.Errorf("coordinator notes schema: %w", err)
		}
	}
	return nil
}
