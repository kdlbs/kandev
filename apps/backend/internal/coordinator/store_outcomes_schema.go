package coordinator

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// outcomesRetention is how long outcome, feedback and seen rows are kept.
const outcomesRetention = 400 * 24 * time.Hour

const outcomesTablesSQL = `
	CREATE TABLE IF NOT EXISTS coordinator_outcomes (
		proposal_id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		turn_id TEXT,
		kind TEXT NOT NULL,
		decision TEXT NOT NULL,
		automatic {{boolean}} NOT NULL DEFAULT FALSE,
		decided_at {{timestamp}} NOT NULL,
		edited_fields TEXT NOT NULL DEFAULT '[]',
		reason_code TEXT NOT NULL DEFAULT 'none',
		task_id TEXT,
		task_result TEXT,
		cost_subcents INTEGER,
		reopen_count INTEGER NOT NULL DEFAULT 0,
		approved_at {{timestamp}},
		merged_at {{timestamp}},
		last_step_id TEXT NOT NULL DEFAULT '',
		final {{boolean}} NOT NULL DEFAULT FALSE,
		graded_at {{timestamp}} NOT NULL
	);

	CREATE TABLE IF NOT EXISTS coordinator_feedback (
		id TEXT PRIMARY KEY,
		coordinator_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		proposal_id TEXT NOT NULL,
		turn_id TEXT,
		user_id TEXT NOT NULL,
		reason_code TEXT NOT NULL DEFAULT 'none',
		proposal_kind TEXT NOT NULL DEFAULT '',
		to_step_id TEXT NOT NULL DEFAULT '',
		transition_key TEXT NOT NULL DEFAULT '',
		created_at {{timestamp}} NOT NULL
	);

	CREATE TABLE IF NOT EXISTS coordinator_moveback_seen (
		history_row_id INTEGER NOT NULL,
		coordinator_id TEXT NOT NULL,
		seen_at {{timestamp}} NOT NULL,
		state TEXT NOT NULL DEFAULT 'judged',
		PRIMARY KEY (history_row_id, coordinator_id)
	);
`

const outcomesIndexesSQL = `
	CREATE INDEX IF NOT EXISTS idx_coordinator_outcomes_decided ON coordinator_outcomes(coordinator_id, decided_at, proposal_id);
	CREATE INDEX IF NOT EXISTS idx_coordinator_outcomes_final ON coordinator_outcomes(final, graded_at);
	CREATE INDEX IF NOT EXISTS idx_coordinator_outcomes_task ON coordinator_outcomes(task_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coordinator_feedback_unique ON coordinator_feedback(proposal_id, kind, transition_key);
	CREATE INDEX IF NOT EXISTS idx_coordinator_feedback_created ON coordinator_feedback(coordinator_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_coordinator_moveback_seen_at ON coordinator_moveback_seen(seen_at);
`

// migrateOutcomes applies the additive outcomes schema; every step is
// replayable on SQLite and PostgreSQL.
func (s *Store) migrateOutcomes(_ *db.MigrateLogger) error {
	driver := s.db.DriverName()
	for _, stmt := range []string{outcomesTablesSQL, outcomesIndexesSQL} {
		if _, err := s.db.Exec(dialect.MustRenderSchema(driver, stmt)); err != nil {
			return fmt.Errorf("coordinator outcomes schema: %w", err)
		}
	}
	return nil
}

// PruneOutcomeRows deletes up to limit outcome, feedback and seen rows each
// older than cutoff (by decided_at, created_at and seen_at) in one
// transaction and returns how many rows it removed in total.
func (s *Store) PruneOutcomeRows(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin prune outcome rows: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmts := []string{
		`DELETE FROM coordinator_outcomes WHERE proposal_id IN (
			SELECT proposal_id FROM coordinator_outcomes WHERE decided_at < ? ORDER BY decided_at, proposal_id LIMIT ?)`,
		`DELETE FROM coordinator_feedback WHERE id IN (
			SELECT id FROM coordinator_feedback WHERE created_at < ? ORDER BY created_at, id LIMIT ?)`,
		`DELETE FROM coordinator_moveback_seen WHERE (history_row_id, coordinator_id) IN (
			SELECT history_row_id, coordinator_id FROM coordinator_moveback_seen WHERE seen_at < ? ORDER BY seen_at, history_row_id LIMIT ?)`,
	}
	var total int64
	for _, stmt := range stmts {
		res, err := tx.ExecContext(ctx, tx.Rebind(stmt), cutoff.UTC(), limit)
		if err != nil {
			return 0, fmt.Errorf("prune outcome rows: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count pruned outcome rows: %w", err)
		}
		total += n
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit prune outcome rows: %w", err)
	}
	return total, nil
}
