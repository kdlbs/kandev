package sqlite

import (
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

// initTaskUsageEventsSchema creates task_usage_events: one immutable row per
// observed agent usage event, carrying token counts, resolved dollar cost,
// and the inputs needed to re-derive that cost
// (docs/specs/task-cost-ledger/spec.md). This is a new table, not a column
// added to an existing one, so CREATE TABLE IF NOT EXISTS in the init block
// is correct and complete.
//
// usage_event_id's uniqueness is a NAMED unique index
// (uniq_task_usage_events_usage_event_id), not an inline column constraint,
// because AC-32's Postgres unique-violation detector matches on
// pgErr.ConstraintName - the same shape as uniq_tasks_external_id
// (task_external_id.go).
//
// task_id CASCADEs: task deletion is complete deletion of its spend record.
// session_id is ON DELETE SET NULL, mirroring task_step_transitions'
// rationale - the historical fact that this spend happened must survive a
// session being pruned. turn_id carries no FK for the same reason (AC-33,
// "Corrections to the task brief" #3).
//
// Every token/cost column is BIGINT, never INTEGER: a single turn in the
// spec's live-database sample reported 8,203,943 cached-read tokens, and a
// merged per-session cached-input total hit 98,805,109 - both already past
// Postgres's int4 ceiling.
func (r *Repository) initTaskUsageEventsSchema() error {
	idCol := "id INTEGER PRIMARY KEY AUTOINCREMENT"
	if dialect.IsPostgres(r.db.DriverName()) {
		idCol = "id BIGSERIAL PRIMARY KEY"
	}
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_usage_events (
		` + idCol + `,
		usage_event_id TEXT NOT NULL,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		session_id TEXT REFERENCES task_sessions(id) ON DELETE SET NULL,
		turn_id TEXT,
		agent_profile_id TEXT NOT NULL DEFAULT '',
		agent_type TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL DEFAULT '',
		tokens_in BIGINT NOT NULL DEFAULT 0,
		tokens_cached_read BIGINT,
		tokens_cached_write BIGINT,
		tokens_out BIGINT,
		tokens_thought BIGINT,
		tokens_total BIGINT NOT NULL DEFAULT 0,
		cost_subcents BIGINT NOT NULL DEFAULT 0,
		cost_source TEXT NOT NULL,
		estimated INTEGER NOT NULL DEFAULT 0,
		rate_input_per_million BIGINT,
		rate_cached_read_per_million BIGINT,
		rate_cached_write_per_million BIGINT,
		rate_output_per_million BIGINT,
		pricing_catalog_version TEXT,
		contract_version INTEGER NOT NULL,
		occurred_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL
	);
	CREATE UNIQUE INDEX IF NOT EXISTS uniq_task_usage_events_usage_event_id
		ON task_usage_events(usage_event_id);
	CREATE INDEX IF NOT EXISTS idx_task_usage_events_task
		ON task_usage_events(task_id, occurred_at, id);
	CREATE INDEX IF NOT EXISTS idx_task_usage_events_session_turn
		ON task_usage_events(session_id, turn_id);
	DROP INDEX IF EXISTS idx_task_usage_events_occurred;
	`)
	if err != nil {
		return fmt.Errorf("init task usage events schema: %w", err)
	}
	return nil
}
