package coordinator

import (
	"fmt"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

const dreamTablesSQL = `
	CREATE TABLE IF NOT EXISTS coordinator_dreams (
		id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		status TEXT NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		window_start {{timestamp}} NOT NULL,
		window_end {{timestamp}} NOT NULL,
		input_hash TEXT NOT NULL DEFAULT '',
		turn_ids TEXT NOT NULL DEFAULT '[]',
		considered TEXT NOT NULL DEFAULT '[]',
		model TEXT NOT NULL DEFAULT '',
		cost_subcents INTEGER,
		episode_task_id TEXT,
		episode_session_id TEXT,
		episode_archived_at {{timestamp}},
		started_at {{timestamp}} NOT NULL,
		refreshed_at {{timestamp}} NOT NULL,
		finished_at {{timestamp}}
	);

	CREATE TABLE IF NOT EXISTS coordinator_dream_items (
		id TEXT PRIMARY KEY,
		dream_id TEXT NOT NULL,
		position INTEGER NOT NULL,
		kind TEXT NOT NULL,
		text TEXT NOT NULL,
		target_id TEXT NOT NULL DEFAULT '',
		cited_turn_ids TEXT NOT NULL DEFAULT '[]',
		gate TEXT NOT NULL,
		replay_id TEXT,
		replay_verdict TEXT NOT NULL DEFAULT '',
		replay_reason TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS coordinator_dream_ratings (
		item_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		rating TEXT NOT NULL,
		rated_at {{timestamp}} NOT NULL,
		PRIMARY KEY (item_id, user_id)
	);
`

const dreamIndexesSQL = `
	CREATE INDEX IF NOT EXISTS idx_coordinator_dreams_list ON coordinator_dreams(coordinator_id, started_at, id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_dreams_running ON coordinator_dreams(coordinator_id) WHERE status = 'running';
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_dreams_skipped ON coordinator_dreams(coordinator_id, input_hash) WHERE status = 'skipped';
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_dream_items_pos ON coordinator_dream_items(dream_id, position);
`

// migrateDreams applies the additive shadow dream schema; every step is
// replayable on SQLite and PostgreSQL.
func (s *Store) migrateDreams(migrate *db.MigrateLogger) error {
	driver := s.db.DriverName()
	if err := migrate.Apply("coordinators.shadow_dream_enabled",
		`ALTER TABLE coordinators ADD COLUMN shadow_dream_enabled INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("coordinator dream column: %w", err)
	}
	for _, stmt := range []string{dreamTablesSQL, dreamIndexesSQL} {
		if _, err := s.db.Exec(dialect.MustRenderSchema(driver, stmt)); err != nil {
			return fmt.Errorf("coordinator dream schema: %w", err)
		}
	}
	return nil
}
