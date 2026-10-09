package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db"
)

// ensureUsageSchema stores source-aware cumulative observations separately
// from task_usage_events. The latter is an immutable native event ledger and
// must remain unchanged when external collectors are installed or removed.
//
//nolint:funlen // The bootstrap migration must keep both table definitions and additive upgrades together.
func (r *Repository) ensureUsageSchema() error {
	// PostgreSQL's default pgx prepared-statement mode accepts one statement per
	// execution. Keep the bootstrap statements separate so the same repository
	// works with both SQLite and PostgreSQL connections.
	schemaStatements := []string{`
CREATE TABLE IF NOT EXISTS session_usage_measurements (
	 id TEXT PRIMARY KEY,
	 workspace_id TEXT NOT NULL,
	 task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	 session_id TEXT REFERENCES task_sessions(id) ON DELETE SET NULL,
	 transcript_id TEXT NOT NULL DEFAULT '',
	 source TEXT NOT NULL,
	 source_record_id TEXT NOT NULL,
	 usage_identity TEXT NOT NULL,
	 model TEXT NOT NULL DEFAULT '',
	 provider TEXT NOT NULL DEFAULT '',
	 source_version TEXT NOT NULL DEFAULT '',
	 revision BIGINT NOT NULL DEFAULT 0,
	 payload_digest TEXT NOT NULL DEFAULT '',
	 observed_at TIMESTAMP NOT NULL,
	 collected_at TIMESTAMP NOT NULL,
	 input_tokens BIGINT,
	 output_tokens BIGINT,
	 cache_read_tokens BIGINT,
	 cache_write_tokens BIGINT,
	reasoning_tokens BIGINT,
	total_tokens BIGINT,
	turns BIGINT,
	cost_subcents BIGINT,
	 currency TEXT NOT NULL DEFAULT '',
	 cost_basis TEXT NOT NULL DEFAULT 'unknown',
	 cost_coverage TEXT NOT NULL DEFAULT 'missing',
	 coverage TEXT NOT NULL DEFAULT 'complete',
	 source_timezone TEXT NOT NULL DEFAULT '',
	 attribution_status TEXT NOT NULL DEFAULT 'attributed',
	 coverage_start TIMESTAMP,
	 coverage_end TIMESTAMP,
	 source_date TEXT NOT NULL DEFAULT '',
	 coverage_key TEXT NOT NULL DEFAULT '',
	 estimated INTEGER NOT NULL DEFAULT 0,
	 stale INTEGER NOT NULL DEFAULT 0,
 UNIQUE(workspace_id, source, source_record_id, usage_identity, model, provider)
)`, `
CREATE TABLE IF NOT EXISTS session_usage_buckets (
	 id TEXT PRIMARY KEY,
	 workspace_id TEXT NOT NULL,
	 task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	 session_id TEXT REFERENCES task_sessions(id) ON DELETE SET NULL,
	 transcript_id TEXT NOT NULL DEFAULT '',
	 source TEXT NOT NULL,
	 source_record_id TEXT NOT NULL,
	 usage_identity TEXT NOT NULL,
	 model TEXT NOT NULL DEFAULT '',
	 provider TEXT NOT NULL DEFAULT '',
	 source_version TEXT NOT NULL DEFAULT '',
	 revision BIGINT NOT NULL DEFAULT 0,
	 payload_digest TEXT NOT NULL DEFAULT '',
	 observed_at TIMESTAMP NOT NULL,
	 collected_at TIMESTAMP NOT NULL,
	 input_tokens BIGINT,
	 output_tokens BIGINT,
	 cache_read_tokens BIGINT,
	 cache_write_tokens BIGINT,
	reasoning_tokens BIGINT,
	total_tokens BIGINT,
	turns BIGINT,
	cost_subcents BIGINT,
	 currency TEXT NOT NULL DEFAULT '',
	 cost_basis TEXT NOT NULL DEFAULT 'unknown',
	 cost_coverage TEXT NOT NULL DEFAULT 'missing',
	 coverage TEXT NOT NULL DEFAULT 'complete',
	 source_timezone TEXT NOT NULL DEFAULT '',
	 attribution_status TEXT NOT NULL DEFAULT 'attributed',
	 coverage_start TIMESTAMP,
	 coverage_end TIMESTAMP,
	 source_date TEXT NOT NULL DEFAULT '',
	 coverage_key TEXT NOT NULL,
	 estimated INTEGER NOT NULL DEFAULT 0,
	 stale INTEGER NOT NULL DEFAULT 0,
 UNIQUE(workspace_id, source, source_record_id, usage_identity, model, provider, coverage_key)
)`, `
CREATE INDEX IF NOT EXISTS idx_session_usage_measurements_workspace
	ON session_usage_measurements(workspace_id, collected_at)`, `
CREATE INDEX IF NOT EXISTS idx_session_usage_measurements_session
	ON session_usage_measurements(workspace_id, session_id, model)`, `
CREATE INDEX IF NOT EXISTS idx_session_usage_measurements_task
	ON session_usage_measurements(workspace_id, task_id, model)`, `
CREATE INDEX IF NOT EXISTS idx_session_usage_buckets_workspace_date
	ON session_usage_buckets(workspace_id, coverage_start, source_date)`, `
CREATE INDEX IF NOT EXISTS idx_session_usage_buckets_session
	ON session_usage_buckets(workspace_id, session_id, coverage_start)`,
	}
	for _, statement := range schemaStatements {
		if _, err := r.db.ExecContext(context.Background(), statement); err != nil {
			return fmt.Errorf("ensure session usage schema: %w", err)
		}
	}
	// The tables were introduced before cost coverage became a separate
	// dimension. Keep existing installations readable during the additive
	// migration; SQLite reports a duplicate-column error when the column is
	// already present.
	for _, table := range []string{"session_usage_measurements", "session_usage_buckets"} {
		if _, alterErr := r.db.ExecContext(context.Background(), `ALTER TABLE `+table+` ADD COLUMN cost_coverage TEXT NOT NULL DEFAULT 'missing'`); alterErr != nil {
			if !db.IsDuplicateColumnError(alterErr) {
				return fmt.Errorf("add cost coverage to %s: %w", table, alterErr)
			}
		}
		// Rows written before cost coverage was split from token coverage have a
		// default of "missing" after the additive ALTER. Recover the old known
		// cost semantics without overwriting an explicitly partial/missing row
		// from the new writer.
		if _, backfillErr := r.db.ExecContext(context.Background(), `UPDATE `+table+`
			SET cost_coverage = coverage
			WHERE cost_coverage = 'missing'
			  AND cost_subcents IS NOT NULL
			  AND COALESCE(cost_basis, 'unknown') NOT IN ('', 'unknown')`); backfillErr != nil {
			return fmt.Errorf("backfill cost coverage for %s: %w", table, backfillErr)
		}
	}
	return nil
}
