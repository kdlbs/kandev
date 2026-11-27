package coordinator

import (
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

const replayTablesSQL = `
	CREATE TABLE IF NOT EXISTS coordinator_replay_results (
		id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		status TEXT NOT NULL,
		dream_id TEXT,
		item_id TEXT,
		candidate_hash TEXT NOT NULL DEFAULT '',
		baseline_hash TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		prompt_version TEXT NOT NULL DEFAULT '',
		cases TEXT NOT NULL DEFAULT '[]',
		candidate_score INTEGER,
		baseline_score INTEGER,
		heldout_candidate_score INTEGER,
		heldout_baseline_score INTEGER,
		cases_ran_candidate INTEGER NOT NULL DEFAULT 0,
		cases_ran_baseline INTEGER NOT NULL DEFAULT 0,
		cases_compared INTEGER NOT NULL DEFAULT 0,
		heldout_compared INTEGER NOT NULL DEFAULT 0,
		cited_turn_ids TEXT NOT NULL DEFAULT '[]',
		flips TEXT NOT NULL DEFAULT '[]',
		unmatched_candidate INTEGER NOT NULL DEFAULT 0,
		unmatched_baseline INTEGER NOT NULL DEFAULT 0,
		guard TEXT NOT NULL DEFAULT '',
		verdict TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL DEFAULT '',
		cost_subcents INTEGER NOT NULL DEFAULT 0,
		created_at {{timestamp}} NOT NULL,
		finished_at {{timestamp}}
	);
`

const replayIndexesSQL = `
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_replay_results_item ON coordinator_replay_results(dream_id, item_id) WHERE dream_id IS NOT NULL AND item_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_coordinator_replay_results_coord ON coordinator_replay_results(coordinator_id, created_at, id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_replay_results_status ON coordinator_replay_results(status, created_at);
`

// migrateReplay applies the additive replay result schema; every step is
// replayable on SQLite and PostgreSQL.
func (s *Store) migrateReplay() error {
	driver := s.db.DriverName()
	for _, stmt := range []string{replayTablesSQL, replayIndexesSQL} {
		if _, err := s.db.Exec(dialect.MustRenderSchema(driver, stmt)); err != nil {
			return fmt.Errorf("coordinator replay schema: %w", err)
		}
	}
	return nil
}
