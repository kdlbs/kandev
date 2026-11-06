package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db"
)

func (r *Repository) ensureUsageContractSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE usage_scopes ADD COLUMN work_version BIGINT NOT NULL DEFAULT 1`); err != nil && !db.IsDuplicateColumnError(err) {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE usage_scopes ADD COLUMN work_pending INTEGER NOT NULL DEFAULT 1`); err != nil && !db.IsDuplicateColumnError(err) {
		return err
	}
	sequence := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if r.db.DriverName() == usageDriverPostgres {
		sequence = "BIGSERIAL PRIMARY KEY"
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS usage_legacy_adoptions (scope_id TEXT NOT NULL, workspace_id TEXT NOT NULL, task_id TEXT NOT NULL, task_created_at TIMESTAMP NOT NULL, session_id TEXT NOT NULL, native_upper_bound BIGINT NOT NULL, PRIMARY KEY(task_id,task_created_at,session_id))`,
		`CREATE INDEX IF NOT EXISTS idx_usage_legacy_adoption_scope ON usage_legacy_adoptions(scope_id,task_id,native_upper_bound)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_work_pending ON usage_scopes(work_pending, scope_id)`,
		`CREATE TABLE IF NOT EXISTS usage_collection_contexts (context_id TEXT PRIMARY KEY, scope_id TEXT NOT NULL, workspace_id TEXT NOT NULL, trusted INTEGER NOT NULL, mixed INTEGER NOT NULL, evidence TEXT NOT NULL DEFAULT 'creation')`,
		`CREATE INDEX IF NOT EXISTS idx_usage_collection_context_scope ON usage_collection_contexts(scope_id)`,
		`CREATE TABLE IF NOT EXISTS usage_collection_preparation (scope_id TEXT PRIMARY KEY, last_key TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS usage_work_identities (scope_id TEXT NOT NULL, subject_id TEXT NOT NULL, scheme TEXT NOT NULL, authority TEXT NOT NULL, external_id TEXT NOT NULL, event_id TEXT NOT NULL, PRIMARY KEY(scope_id, subject_id, scheme, authority, external_id))`,
		`CREATE TABLE IF NOT EXISTS usage_collection_bindings (
			sequence ` + sequence + `, binding_id TEXT NOT NULL UNIQUE, scope_id TEXT NOT NULL, subject_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL, task_id TEXT NOT NULL, session_id TEXT NOT NULL, task_incarnation_id TEXT NOT NULL,
			session_incarnation_id TEXT NOT NULL, external_session_id TEXT NOT NULL, session_started_at TEXT,
			session_ended_at TEXT, coverage_start TEXT, coverage_end TEXT, actor_id TEXT NOT NULL,
			actor_status TEXT NOT NULL, actor_kind TEXT NOT NULL, fingerprint TEXT NOT NULL,
			UNIQUE(subject_id, fingerprint))`,
		`CREATE INDEX IF NOT EXISTS idx_usage_collection_subject ON usage_collection_bindings(scope_id, subject_id, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_collection_sequence ON usage_collection_bindings(scope_id, sequence)`,
		`CREATE TABLE IF NOT EXISTS usage_collection_grants (source TEXT NOT NULL, binding_id TEXT NOT NULL, scope_id TEXT NOT NULL,
			PRIMARY KEY(source, binding_id))`,
		`CREATE TABLE IF NOT EXISTS usage_collection_snapshots (snapshot_id TEXT PRIMARY KEY, source TEXT NOT NULL,
			workspace_id TEXT NOT NULL, scope_id TEXT NOT NULL, upper_bound BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS usage_observation_items (source TEXT NOT NULL, item_id TEXT NOT NULL, scope_id TEXT NOT NULL,
			subject_id TEXT NOT NULL, binding_id TEXT NOT NULL, source_table TEXT NOT NULL DEFAULT '', observation_id TEXT NOT NULL,
			revision BIGINT NOT NULL DEFAULT 0, payload_digest TEXT NOT NULL DEFAULT '', retired INTEGER NOT NULL DEFAULT 0,
			set_id TEXT NOT NULL DEFAULT '', set_revision BIGINT NOT NULL DEFAULT 0, PRIMARY KEY(source, item_id))`,
		`CREATE INDEX IF NOT EXISTS idx_usage_items_scope ON usage_observation_items(scope_id, subject_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_items_set ON usage_observation_items(source, binding_id, set_id, retired, item_id)`,
		`CREATE TABLE IF NOT EXISTS usage_observation_sets (source TEXT NOT NULL, binding_id TEXT NOT NULL, scope_id TEXT NOT NULL,
			set_id TEXT NOT NULL, revision BIGINT NOT NULL, payload_digest TEXT NOT NULL, PRIMARY KEY(source, binding_id, set_id))`,
	}
	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("usage contract schema: %w", err)
		}
	}
	if err := r.ensureUsageCollectionContextEvidence(ctx); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE usage_collection_bindings ADD COLUMN source_scope_status TEXT NOT NULL DEFAULT 'unknown'`); err != nil && !db.IsDuplicateColumnError(err) {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_usage_collection_session ON usage_collection_bindings(task_id,task_incarnation_id,session_id,source_scope_status,sequence)`); err != nil {
		return err
	}
	if err := r.ensureUsageContractSourceSchema(ctx); err != nil {
		return err
	}
	if err := r.ensureUsageSegmentSchema(ctx); err != nil {
		return err
	}
	if err := r.ensureUsageWorkRepairSchema(ctx); err != nil {
		return err
	}
	return r.ensureUsageCurrentTargetSchema(ctx)
}

func (r *Repository) ensureUsageCollectionContextEvidence(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE usage_collection_contexts ADD COLUMN evidence TEXT NOT NULL DEFAULT 'creation'`); err != nil && !db.IsDuplicateColumnError(err) {
		return err
	}
	return nil
}

func (r *Repository) ensureUsageContractSourceSchema(ctx context.Context) error {
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		for _, column := range []string{
			`selection_version BIGINT NOT NULL DEFAULT 0`,
			`work_event_id TEXT NOT NULL DEFAULT ''`, `item_id TEXT NOT NULL DEFAULT ''`, `selection_partition TEXT NOT NULL DEFAULT ''`,
			`selection_coverage TEXT NOT NULL DEFAULT ''`, `work_status TEXT NOT NULL DEFAULT 'unproven'`,
			`retired INTEGER NOT NULL DEFAULT 0`, `observation_metadata TEXT NOT NULL DEFAULT ''`,
		} {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column); err != nil && !db.IsDuplicateColumnError(err) {
				return err
			}
		}
		for _, statement := range []string{
			`CREATE INDEX IF NOT EXISTS idx_` + table + `_work_version ON ` + table + `(scope_id, selection_version, id)`,
			`CREATE INDEX IF NOT EXISTS idx_` + table + `_source_key ON ` + table + `(scope_id, id)`,
			`CREATE INDEX IF NOT EXISTS idx_` + table + `_selection ON ` + table + `(scope_id, selection_partition, retired, selection_coverage, id)`,
			`CREATE INDEX IF NOT EXISTS idx_` + table + `_interval ON ` + table + `(scope_id, selection_partition, retired, coverage_start, coverage_end)`,
			`CREATE INDEX IF NOT EXISTS idx_` + table + `_coarse_interval_end ON ` + table + `(scope_id, selection_partition, retired, coverage_end, coverage_start) WHERE source <> 'native' AND coverage_key NOT LIKE 'point:%'`,
			`CREATE INDEX IF NOT EXISTS idx_` + table + `_dimensions ON ` + table + `(scope_id, selection_partition, model, provider, token_semantics_version)`,
		} {
			if _, err := r.db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	return nil
}
