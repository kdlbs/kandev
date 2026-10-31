package sqlite

import (
	"context"
	"fmt"
)

// ensureUsageSchema stores source-aware cumulative observations separately
// from task_usage_events. The latter is an immutable native event ledger and
// must remain unchanged when external collectors are installed or removed.
func (r *Repository) ensureUsageSchema() error {
	ctx := context.Background()
	// PostgreSQL's default pgx prepared-statement mode accepts one statement per
	// execution. Keep the bootstrap statements separate so the same repository
	// works with both SQLite and PostgreSQL connections.
	for _, statement := range usageSchemaStatements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure session usage schema: %w", err)
		}
	}
	if err := r.upgradeUsageObservationTables(ctx); err != nil {
		return err
	}
	if err := r.addUsageTaskIncarnationColumns(ctx); err != nil {
		return err
	}
	if err := r.ensureUsageSubjectTaskIncarnationKey(ctx); err != nil {
		return err
	}
	if err := r.removeUsageLiveForeignKeys(ctx); err != nil {
		return err
	}
	if err := r.ensureUsageContractSchema(ctx); err != nil {
		return err
	}
	if err := r.ensureUsageProjectionSchema(ctx); err != nil {
		return err
	}
	return r.ensureNativeUsageAccountingRepairSchema(ctx)
}

var usageSchemaStatements = []string{`
CREATE TABLE IF NOT EXISTS session_usage_measurements (
	 id TEXT PRIMARY KEY,
	 workspace_id TEXT NOT NULL,
 task_id TEXT NOT NULL,
 session_id TEXT,
	 scope_id TEXT NOT NULL DEFAULT '',
	 subject_id TEXT NOT NULL DEFAULT '',
	 usage_binding_id TEXT NOT NULL DEFAULT '',
	 actor_id TEXT NOT NULL DEFAULT '',
	 actor_status TEXT NOT NULL DEFAULT 'unknown',
	 actor_kind TEXT NOT NULL DEFAULT 'unknown',
	 transcript_id TEXT NOT NULL DEFAULT '',
	 source TEXT NOT NULL,
	 source_record_id TEXT NOT NULL,
	 usage_identity TEXT NOT NULL,
	 model TEXT NOT NULL DEFAULT '',
	 provider TEXT NOT NULL DEFAULT '',
	 agent_client TEXT NOT NULL DEFAULT '',
	 source_version TEXT NOT NULL DEFAULT '',
	 native_scope TEXT NOT NULL DEFAULT '',
	 measurement_source TEXT NOT NULL DEFAULT '',
	 usage_completeness TEXT NOT NULL DEFAULT '',
	 usage_schema_version INTEGER NOT NULL DEFAULT 0,
	 pricing_catalog_version TEXT NOT NULL DEFAULT '',
	 revision BIGINT NOT NULL DEFAULT 0,
	 payload_digest TEXT NOT NULL DEFAULT '',
	 observed_at TIMESTAMP NOT NULL,
	 collected_at TIMESTAMP NOT NULL,
	 input_tokens BIGINT,
	 output_tokens BIGINT,
	 cache_read_tokens BIGINT,
	 cache_write_tokens BIGINT,
	reasoning_tokens BIGINT,
	reasoning_output_tokens BIGINT,
	total_tokens BIGINT,
	 source_total_tokens BIGINT,
	 reported_cache_write_tokens BIGINT,
	 token_semantics_version INTEGER NOT NULL DEFAULT 0,
	 reasoning_inside_output INTEGER NOT NULL DEFAULT 0,
	 cache_read_inside_input INTEGER NOT NULL DEFAULT 0,
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
 task_id TEXT NOT NULL,
 session_id TEXT,
	 scope_id TEXT NOT NULL DEFAULT '',
	 subject_id TEXT NOT NULL DEFAULT '',
	 usage_binding_id TEXT NOT NULL DEFAULT '',
	 actor_id TEXT NOT NULL DEFAULT '',
	 actor_status TEXT NOT NULL DEFAULT 'unknown',
	 actor_kind TEXT NOT NULL DEFAULT 'unknown',
	 transcript_id TEXT NOT NULL DEFAULT '',
	 source TEXT NOT NULL,
	 source_record_id TEXT NOT NULL,
	 usage_identity TEXT NOT NULL,
	 model TEXT NOT NULL DEFAULT '',
	 provider TEXT NOT NULL DEFAULT '',
	 agent_client TEXT NOT NULL DEFAULT '',
	 source_version TEXT NOT NULL DEFAULT '',
	 native_scope TEXT NOT NULL DEFAULT '',
	 measurement_source TEXT NOT NULL DEFAULT '',
	 usage_completeness TEXT NOT NULL DEFAULT '',
	 usage_schema_version INTEGER NOT NULL DEFAULT 0,
	 pricing_catalog_version TEXT NOT NULL DEFAULT '',
	 revision BIGINT NOT NULL DEFAULT 0,
	 payload_digest TEXT NOT NULL DEFAULT '',
	 observed_at TIMESTAMP NOT NULL,
	 collected_at TIMESTAMP NOT NULL,
	 input_tokens BIGINT,
	 output_tokens BIGINT,
	 cache_read_tokens BIGINT,
	 cache_write_tokens BIGINT,
	reasoning_tokens BIGINT,
	reasoning_output_tokens BIGINT,
	total_tokens BIGINT,
	 source_total_tokens BIGINT,
	 reported_cache_write_tokens BIGINT,
	 token_semantics_version INTEGER NOT NULL DEFAULT 0,
	 reasoning_inside_output INTEGER NOT NULL DEFAULT 0,
	 cache_read_inside_input INTEGER NOT NULL DEFAULT 0,
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
	`CREATE TABLE IF NOT EXISTS usage_scopes (
			scope_id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			organization_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			UNIQUE(workspace_id, organization_id)
		)`,
	`CREATE TABLE IF NOT EXISTS usage_subjects (
			subject_id TEXT PRIMARY KEY,
			scope_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			task_incarnation_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			session_incarnation_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			UNIQUE(scope_id, task_id, task_incarnation_id, session_id, session_incarnation_id)
		)`,
	`CREATE TABLE IF NOT EXISTS usage_history_schema_version (
			version_key TEXT PRIMARY KEY,
			version BIGINT NOT NULL
		)`,
	`CREATE TABLE IF NOT EXISTS usage_execution_bindings (
			binding_id TEXT PRIMARY KEY,
			binding_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			task_incarnation_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL,
			session_incarnation_id TEXT NOT NULL,
			turn_id TEXT NOT NULL DEFAULT '',
			execution_id TEXT NOT NULL DEFAULT '',
			parent_binding_id TEXT NOT NULL DEFAULT '',
			actor_id TEXT NOT NULL DEFAULT '',
			actor_status TEXT NOT NULL,
			actor_kind TEXT NOT NULL,
			actor_source TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_execution_bindings_turn
			ON usage_execution_bindings(session_id, turn_id)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_execution_bindings_subject
			ON usage_execution_bindings(scope_id, subject_id)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_native_event
			ON session_usage_buckets(source_record_id) WHERE source = 'native'`,
	`CREATE TABLE IF NOT EXISTS usage_dirty_partitions (
			scope_id TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			partition_key TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			PRIMARY KEY(scope_id, subject_id, partition_key)
		)`,
	`CREATE TABLE IF NOT EXISTS usage_live_links (
			source_table TEXT NOT NULL,
			observation_id TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			subject_id TEXT NOT NULL,
			live_task_id TEXT,
			live_session_id TEXT,
			PRIMARY KEY(source_table, observation_id)
		)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_live_links_task ON usage_live_links(live_task_id)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_live_links_session ON usage_live_links(live_session_id)`,
	`CREATE TABLE IF NOT EXISTS usage_history_migration_state (
			source_key TEXT PRIMARY KEY,
			high_water TEXT NOT NULL,
			last_key TEXT NOT NULL DEFAULT '',
			complete INTEGER NOT NULL DEFAULT 0,
			updated_at TIMESTAMP NOT NULL
		)`,
	`CREATE TABLE IF NOT EXISTS usage_history_migration_gaps (
			source_key TEXT NOT NULL,
			record_key TEXT NOT NULL,
			task_id TEXT NOT NULL DEFAULT '',
			workspace_id TEXT NOT NULL DEFAULT '',
			organization_id TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL,
			detected_at TIMESTAMP NOT NULL,
			PRIMARY KEY(source_key, record_key)
		)`,
	`CREATE TABLE IF NOT EXISTS usage_preservation_fences (
			fence_id TEXT PRIMARY KEY,
			fence_kind TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			organization_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL,
			task_created_at TIMESTAMP NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			session_incarnation_id TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_preservation_task ON usage_preservation_fences(task_id, task_created_at, state)`,
	`CREATE TABLE IF NOT EXISTS usage_preservation_cursors (
			fence_id TEXT NOT NULL,
			source_key TEXT NOT NULL,
			high_water TEXT NOT NULL,
			last_key TEXT NOT NULL DEFAULT '',
			complete INTEGER NOT NULL DEFAULT 0,
			updated_at TIMESTAMP NOT NULL,
			PRIMARY KEY(fence_id, source_key)
		)`,
	`CREATE TABLE IF NOT EXISTS usage_scope_fences (
			fence_kind TEXT NOT NULL,
			fence_id TEXT NOT NULL,
			state TEXT NOT NULL,
			generation BIGINT NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			PRIMARY KEY(fence_kind, fence_id)
		)`,
}

var usageHistoryAdditiveColumns = []usageSchemaColumn{
	{"scope_id", `scope_id TEXT NOT NULL DEFAULT ''`},
	{"subject_id", `subject_id TEXT NOT NULL DEFAULT ''`},
	{"usage_binding_id", `usage_binding_id TEXT NOT NULL DEFAULT ''`},
	{"actor_id", `actor_id TEXT NOT NULL DEFAULT ''`},
	{"actor_status", `actor_status TEXT NOT NULL DEFAULT 'unknown'`},
	{"actor_kind", `actor_kind TEXT NOT NULL DEFAULT 'unknown'`},
	{"agent_client", `agent_client TEXT NOT NULL DEFAULT ''`},
	{"source_total_tokens", `source_total_tokens BIGINT`},
	{"token_semantics_version", `token_semantics_version INTEGER NOT NULL DEFAULT 0`},
	{"reasoning_inside_output", `reasoning_inside_output INTEGER NOT NULL DEFAULT 0`},
	{"cache_read_inside_input", `cache_read_inside_input INTEGER NOT NULL DEFAULT 0`},
}

var usageNativeAdditiveColumns = []usageSchemaColumn{
	{"native_scope", `native_scope TEXT NOT NULL DEFAULT ''`},
	{"measurement_source", `measurement_source TEXT NOT NULL DEFAULT ''`},
	{"usage_completeness", `usage_completeness TEXT NOT NULL DEFAULT ''`},
	{"usage_schema_version", `usage_schema_version INTEGER NOT NULL DEFAULT 0`},
	{"pricing_catalog_version", `pricing_catalog_version TEXT NOT NULL DEFAULT ''`},
	{"reported_cache_write_tokens", `reported_cache_write_tokens BIGINT`},
	{"reasoning_output_tokens", `reasoning_output_tokens BIGINT`},
}
