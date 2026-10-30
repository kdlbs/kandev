package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db"
)

func (r *Repository) ensureUsageProjectionSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE usage_scopes ADD COLUMN projection_registered INTEGER NOT NULL DEFAULT 0`); err != nil && !db.IsDuplicateColumnError(err) {
		return err
	}
	for _, statement := range usageProjectionSchema {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("usage projection schema: %w", err)
		}
	}
	for _, statement := range []string{
		`ALTER TABLE usage_projection_generations ADD COLUMN work_version BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE usage_projection_work ADD COLUMN publish_group TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE usage_projection_log ADD COLUMN publication_group TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE usage_canonical_facts ADD COLUMN collected_at TIMESTAMP`,
		`ALTER TABLE usage_projection_history ADD COLUMN collected_at TIMESTAMP`,
		`ALTER TABLE usage_projection_generations ADD COLUMN collected_at TIMESTAMP`,
		`ALTER TABLE usage_projection_history ADD COLUMN provider_options_ready INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE usage_projection_history ADD COLUMN provider_options_version INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE usage_projection_history ADD COLUMN provider_options_build_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE usage_projection_history ADD COLUMN provider_options_cursor TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE usage_projection_history ADD COLUMN provider_options_phase TEXT NOT NULL DEFAULT 'clear'`,
	} {
		if _, err := r.db.ExecContext(ctx, statement); err != nil && !db.IsDuplicateColumnError(err) {
			return err
		}
	}
	for _, statement := range usageReportingIndexes {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_usage_projection_publication_group ON usage_projection_work(scope_id, generation, publish_group, state)`); err != nil {
		return err
	}
	if err := r.ensureUsageProjectionSegmentSchema(ctx); err != nil {
		return err
	}
	return nil
}

var usageReportingIndexes = []string{
	`CREATE INDEX IF NOT EXISTS idx_usage_measurement_source_start ON session_usage_measurements(scope_id, source, retired, coverage_start, coverage_end, id)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_measurement_source_end ON session_usage_measurements(scope_id, source, retired, coverage_end, coverage_start, id)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_measurement_source_page ON session_usage_measurements(scope_id, source, retired, id)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_bucket_source_start ON session_usage_buckets(scope_id, source, retired, coverage_start, coverage_end, id)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_bucket_source_end ON session_usage_buckets(scope_id, source, retired, coverage_end, coverage_start, id)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_bucket_source_page ON session_usage_buckets(scope_id, source, retired, id)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_contribution_points ON usage_projection_contributions(scope_id, generation, calendar, coverage_kind, coverage_start)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_contribution_overlap ON usage_projection_contributions(scope_id, generation, coverage_kind, coverage_end, coverage_start)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_contribution_coarse_range ON usage_projection_contributions(scope_id, generation, coverage_end, coverage_start) WHERE coverage_kind IN ('calendar','interval')`,
	`CREATE INDEX IF NOT EXISTS idx_usage_contribution_undated_interval_options ON usage_projection_contributions(scope_id, generation, model, source, provider, work_key, build_id) WHERE coverage_kind='interval' AND usage_date=''`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_provider_options_report ON usage_projection_provider_options(scope_id, generation, model, source, provider, work_key)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_provider_options_work ON usage_projection_provider_options(scope_id, generation, work_key, model, source, provider)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_provider_options_pending ON usage_projection_history(scope_id, generation, work_key) WHERE provider_options_ready=0 OR provider_options_version=0`,
	`CREATE INDEX IF NOT EXISTS idx_usage_contribution_provider_points ON usage_projection_contributions(scope_id, generation, provider, calendar, coverage_kind, coverage_start, coverage_end)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_canonical_lifetime ON usage_canonical_facts(scope_id, generation, work_key, model, provider, subject_id, build_id) WHERE coverage_kind='lifetime'`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_collected ON usage_projection_history(scope_id, generation, collected_at)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_migration_gap_scope ON usage_history_migration_gaps(workspace_id, organization_id)`,
}

var usageProjectionSchema = []string{
	`CREATE INDEX IF NOT EXISTS idx_usage_scope_projection_registration ON usage_scopes(projection_registered, scope_id)`,
	`CREATE TABLE IF NOT EXISTS usage_projection_scopes (
		scope_id TEXT PRIMARY KEY, commit_sequence BIGINT NOT NULL DEFAULT 0,
		active_generation TEXT NOT NULL DEFAULT '', revision BIGINT NOT NULL DEFAULT 0,
		native_accounting_source_version INTEGER NOT NULL DEFAULT 0,
		native_accounting_required_sequence BIGINT NOT NULL DEFAULT 0,
		updated_at TIMESTAMP NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS usage_projection_log (
		scope_id TEXT NOT NULL, commit_sequence BIGINT NOT NULL, payload TEXT NOT NULL, publication_group TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL, PRIMARY KEY(scope_id, commit_sequence))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_partitions (
		scope_id TEXT NOT NULL, work_key TEXT NOT NULL, subject_id TEXT NOT NULL,
		commit_sequence BIGINT NOT NULL, PRIMARY KEY(scope_id, work_key))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_source_versions (
		scope_id TEXT NOT NULL, source_table TEXT NOT NULL, source_id TEXT NOT NULL,
		commit_sequence BIGINT NOT NULL, tombstone INTEGER NOT NULL,
		PRIMARY KEY(scope_id, source_table, source_id))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_generations (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, state TEXT NOT NULL,
		start_sequence BIGINT NOT NULL, replay_sequence BIGINT NOT NULL,
		revision BIGINT NOT NULL DEFAULT 0, fact_count BIGINT NOT NULL DEFAULT 0,
		dated_count BIGINT NOT NULL DEFAULT 0, lifetime_count BIGINT NOT NULL DEFAULT 0,
		interval_count BIGINT NOT NULL DEFAULT 0, native_count BIGINT NOT NULL DEFAULT 0,
		earliest TIMESTAMP, latest TIMESTAMP, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY(scope_id, generation))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_checkpoints (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, source_table TEXT NOT NULL,
		upper_bound TEXT NOT NULL, last_key TEXT NOT NULL DEFAULT '', complete INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY(scope_id, generation, source_table))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_items (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, source_table TEXT NOT NULL, source_id TEXT NOT NULL,
		commit_sequence BIGINT NOT NULL, tombstone INTEGER NOT NULL,
		PRIMARY KEY(scope_id, generation, source_table, source_id))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_work (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, work_key TEXT NOT NULL, subject_id TEXT NOT NULL,
		requested_sequence BIGINT NOT NULL, selection_sequence BIGINT NOT NULL DEFAULT -1,
		cursor TEXT NOT NULL DEFAULT '', build_id TEXT NOT NULL DEFAULT '', state TEXT NOT NULL DEFAULT 'pending',
		PRIMARY KEY(scope_id, generation, work_key))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_applied (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, work_key TEXT NOT NULL, build_id TEXT NOT NULL,
		commit_sequence BIGINT NOT NULL, PRIMARY KEY(scope_id, generation, work_key))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_resolutions (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, work_key TEXT NOT NULL, payload TEXT NOT NULL,
		PRIMARY KEY(scope_id, generation, work_key))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_replay_groups (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, group_id TEXT NOT NULL, work_key TEXT NOT NULL,
		PRIMARY KEY(scope_id, generation, group_id))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_builds (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, work_key TEXT NOT NULL, build_id TEXT NOT NULL,
		state TEXT NOT NULL, PRIMARY KEY(scope_id, generation, build_id))`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_build_gc ON usage_projection_builds(state, scope_id, generation, build_id)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_generation_work ON usage_projection_generations(state, updated_at)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_scope_active ON usage_projection_scopes(active_generation, updated_at)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_unindexed_measurements ON session_usage_measurements(id) WHERE selection_partition = '' AND scope_id <> '' AND subject_id <> ''`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_unindexed_buckets ON session_usage_buckets(id) WHERE selection_partition = '' AND scope_id <> '' AND subject_id <> ''`,
	`CREATE TABLE IF NOT EXISTS usage_canonical_facts (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, build_id TEXT NOT NULL, fact_key TEXT NOT NULL,
		work_key TEXT NOT NULL, subject_id TEXT NOT NULL, task_id TEXT NOT NULL, session_id TEXT NOT NULL,
		coverage_kind TEXT NOT NULL, coverage_start TIMESTAMP, coverage_end TIMESTAMP,
		source_date TEXT NOT NULL, calendar TEXT NOT NULL, model TEXT NOT NULL, provider TEXT NOT NULL,
		token_source TEXT NOT NULL, cost_source TEXT NOT NULL, payload TEXT NOT NULL,
		PRIMARY KEY(scope_id, generation, build_id, fact_key))`,
	`CREATE INDEX IF NOT EXISTS idx_usage_canonical_time ON usage_canonical_facts(scope_id, generation, coverage_start, coverage_end)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_canonical_subject ON usage_canonical_facts(scope_id, generation, subject_id, coverage_start)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_canonical_work ON usage_canonical_facts(scope_id, generation, work_key, build_id)`,
	`CREATE TABLE IF NOT EXISTS usage_projection_contributions (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, build_id TEXT NOT NULL, fact_key TEXT NOT NULL,
		work_key TEXT NOT NULL, summary_key TEXT NOT NULL, coverage_kind TEXT NOT NULL,
		coverage_start TIMESTAMP, coverage_end TIMESTAMP, ` + usageProjectionDimensionsDDL + `,
		` + usageProjectionAmountsDDL + `, partial INTEGER NOT NULL, estimated INTEGER NOT NULL, stale INTEGER NOT NULL,
		cost_basis TEXT NOT NULL,
		PRIMARY KEY(scope_id, generation, build_id, fact_key, metric_kind, calendar, usage_date))`,
	`CREATE TABLE IF NOT EXISTS usage_projection_provider_options (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, work_key TEXT NOT NULL,
		model TEXT NOT NULL, source TEXT NOT NULL, provider TEXT NOT NULL,
		PRIMARY KEY(scope_id, generation, work_key, model, source, provider))`,
	`CREATE INDEX IF NOT EXISTS idx_usage_contribution_summary ON usage_projection_contributions(scope_id, generation, summary_key, build_id)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_contribution_time ON usage_projection_contributions(scope_id, generation, coverage_start, coverage_end)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_contribution_work ON usage_projection_contributions(scope_id, generation, work_key, build_id)`,
	`CREATE TABLE IF NOT EXISTS usage_projection_summary_queue (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, summary_key TEXT NOT NULL,
		PRIMARY KEY(scope_id, generation, summary_key))`,
	`CREATE TABLE IF NOT EXISTS usage_daily_summaries (` + usageProjectionSummaryDDL + `)`,
	`CREATE TABLE IF NOT EXISTS usage_lifetime_summaries (` + usageProjectionSummaryDDL + `)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_daily_date ON usage_daily_summaries(scope_id, generation, calendar, usage_date, provider)`,
	`CREATE TABLE IF NOT EXISTS usage_projection_history (
		scope_id TEXT NOT NULL, generation TEXT NOT NULL, work_key TEXT NOT NULL,
		fact_count BIGINT NOT NULL, dated_count BIGINT NOT NULL, lifetime_count BIGINT NOT NULL,
		interval_count BIGINT NOT NULL, native_count BIGINT NOT NULL, earliest TIMESTAMP, latest TIMESTAMP,
		provider_options_ready INTEGER NOT NULL DEFAULT 0, provider_options_version INTEGER NOT NULL DEFAULT 0,
		provider_options_build_id TEXT NOT NULL DEFAULT '', provider_options_cursor TEXT NOT NULL DEFAULT '',
		provider_options_phase TEXT NOT NULL DEFAULT 'clear',
		PRIMARY KEY(scope_id, generation, work_key))`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_history_mixed_coverage ON usage_projection_history(scope_id, generation) WHERE lifetime_count > 0 AND dated_count > 0`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_earliest ON usage_projection_history(scope_id, generation, earliest)`,
	`CREATE INDEX IF NOT EXISTS idx_usage_projection_latest ON usage_projection_history(scope_id, generation, latest)`,
}

const usageProjectionDimensionsDDL = `metric_kind TEXT NOT NULL, actor_id TEXT NOT NULL, actor_status TEXT NOT NULL,
	actor_kind TEXT NOT NULL, source TEXT NOT NULL, model TEXT NOT NULL, provider TEXT NOT NULL,
	agent_client TEXT NOT NULL, semantics TEXT NOT NULL, currency TEXT NOT NULL, calendar TEXT NOT NULL, usage_date TEXT NOT NULL`

const usageProjectionAmountsDDL = `input_tokens BIGINT, output_tokens BIGINT, cache_read_tokens BIGINT,
	cache_write_tokens BIGINT, reasoning_tokens BIGINT, total_tokens BIGINT, source_total_tokens BIGINT,
	turns BIGINT, cost_subcents BIGINT, rate_tokens BIGINT`

const usageProjectionSummaryDDL = `scope_id TEXT NOT NULL, generation TEXT NOT NULL, summary_key TEXT NOT NULL,
	` + usageProjectionDimensionsDDL + `, ` + usageProjectionAmountsDDL + `,
	input_missing BIGINT NOT NULL, output_missing BIGINT NOT NULL, cache_read_missing BIGINT NOT NULL,
	cache_write_missing BIGINT NOT NULL, reasoning_missing BIGINT NOT NULL, total_missing BIGINT NOT NULL,
	source_total_missing BIGINT NOT NULL, turns_missing BIGINT NOT NULL, cost_missing BIGINT NOT NULL,
	rate_missing BIGINT NOT NULL, token_count BIGINT NOT NULL, cost_count BIGINT NOT NULL,
	partial_count BIGINT NOT NULL, estimated_count BIGINT NOT NULL, stale_count BIGINT NOT NULL,
	cost_basis TEXT NOT NULL, PRIMARY KEY(scope_id, generation, summary_key)`
