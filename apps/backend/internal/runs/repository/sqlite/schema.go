package sqlite

import (
	"fmt"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/runs/models"
)

// Migrate owns the core run queue schema, independent of any feature services.
func (r *Repository) Migrate() error {
	_, err := r.db.Exec(dialect.MustRenderSchema(r.db.DriverName(), `	CREATE TABLE IF NOT EXISTS runs (
		id TEXT PRIMARY KEY,
		agent_profile_id TEXT NOT NULL,
		reason TEXT NOT NULL,
		payload TEXT DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'queued',
		coalesced_count INTEGER DEFAULT 1,
		idempotency_key TEXT,
		context_snapshot TEXT DEFAULT '{}',
		capabilities TEXT NOT NULL DEFAULT '{}',
		input_snapshot TEXT NOT NULL DEFAULT '{}',
		output_summary TEXT NOT NULL DEFAULT '',
		failure_reason TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		retry_count INTEGER DEFAULT 0,
		scheduled_retry_at TIMESTAMP,
		error_message TEXT NOT NULL DEFAULT '',
		cancel_reason TEXT,
		-- Provider routing (office-provider-routing).
		logical_provider_order TEXT,
		requested_tier TEXT,
		resolved_execution_profile_id TEXT,
		resolved_provider_id TEXT,
		resolved_model TEXT,
		current_route_attempt_seq INTEGER NOT NULL DEFAULT 0,
		routing_blocked_status TEXT,
		earliest_retry_at TIMESTAMP,
		-- route_cycle_baseline_seq marks the floor at which the current
		-- retry cycle began. excludedFromAttempts filters prior attempt
		-- rows with seq <= baseline so a parked-then-lifted run gets a
		-- fresh exclusion list instead of re-inheriting every provider
		-- that failed in the previous cycle.
		route_cycle_baseline_seq INTEGER NOT NULL DEFAULT 0,
		-- Heartbeat-rework run inspection columns: structured adapter
		-- output, the assembled prompt the agent received, and the
		-- continuation summary that was prepended (if any).
		result_json TEXT NOT NULL DEFAULT '{}',
		assembled_prompt TEXT NOT NULL DEFAULT '',
		summary_injected TEXT NOT NULL DEFAULT '',
		requested_at TIMESTAMP NOT NULL,
		claimed_at TIMESTAMP,
		finished_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_run_status_requested ON runs(status, requested_at);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_run_idempotency ON runs(idempotency_key) WHERE idempotency_key IS NOT NULL;
	CREATE TABLE IF NOT EXISTS run_events (
		run_id TEXT NOT NULL,
		seq INTEGER NOT NULL,
		event_type TEXT NOT NULL,
		level TEXT NOT NULL DEFAULT 'info',
		payload TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		PRIMARY KEY (run_id, seq)
	);
	CREATE INDEX IF NOT EXISTS idx_run_events_run_created ON run_events(run_id, created_at);`))
	if err != nil {
		return err
	}
	_, err = r.db.Exec("ALTER TABLE runs ADD COLUMN resolved_execution_profile_id TEXT")
	if err != nil && !db.IsDuplicateColumnError(err) {
		return err
	}

	migrate := db.NewRequiredMigrateLogger(r.db, nil)
	for _, migration := range []struct{ name, statement string }{
		{"runs.outcome", "ALTER TABLE runs ADD COLUMN outcome TEXT"},
		{"runs.continuation_scope", "ALTER TABLE runs ADD COLUMN continuation_scope TEXT NOT NULL DEFAULT ''"},
	} {
		if err := migrate.Apply(migration.name, migration.statement); err != nil {
			return err
		}
	}
	if err := r.backfillContinuationScopes(); err != nil {
		return err
	}
	return migrate.Apply("runs.failure_scope_status_index", "CREATE INDEX IF NOT EXISTS idx_run_failure_scope_status ON runs(agent_profile_id, continuation_scope, status)")
}

func (r *Repository) backfillContinuationScopes() error {
	var legacyRuns []struct {
		ID              string `db:"id"`
		AgentProfileID  string `db:"agent_profile_id"`
		ContextSnapshot string `db:"context_snapshot"`
	}
	if err := r.db.Select(&legacyRuns,
		`SELECT id, agent_profile_id, context_snapshot
			 FROM runs WHERE continuation_scope = ''`); err != nil {
		return fmt.Errorf("runs.continuation_scope backfill query: %w", err)
	}
	for _, legacyRun := range legacyRuns {
		scope := models.ContinuationScopeForRun(
			&models.Run{ContextSnapshot: legacyRun.ContextSnapshot},
			legacyRun.AgentProfileID,
		)
		if _, err := r.db.Exec(r.db.Rebind(`
			UPDATE runs SET continuation_scope = ?
			WHERE id = ? AND continuation_scope = ''
		`), scope, legacyRun.ID); err != nil {
			return fmt.Errorf("runs.continuation_scope backfill update %q: %w", legacyRun.ID, err)
		}
	}
	return nil
}
