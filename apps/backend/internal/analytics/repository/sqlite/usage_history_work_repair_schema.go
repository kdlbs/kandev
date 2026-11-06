package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/db"
)

func (r *Repository) ensureUsageWorkRepairSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE usage_scopes ADD COLUMN segment_version INTEGER NOT NULL DEFAULT 0`); err != nil && !db.IsDuplicateColumnError(err) {
		return err
	}
	for _, column := range []string{
		`work_repair_pending INTEGER NOT NULL DEFAULT 0`, `work_repair_after_table TEXT NOT NULL DEFAULT ''`,
		`work_repair_after_id TEXT NOT NULL DEFAULT ''`, `work_repair_new_work TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := r.db.ExecContext(ctx, `ALTER TABLE usage_subjects ADD COLUMN `+column); err != nil && !db.IsDuplicateColumnError(err) {
			return err
		}
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS idx_usage_scope_segment_repair ON usage_scopes(segment_version,scope_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_subject_work_repair ON usage_subjects(work_repair_pending,scope_id,subject_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_subject_work_target ON usage_subjects(scope_id,work_repair_pending,work_repair_new_work,subject_id)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_measurement_unproven ON session_usage_measurements(scope_id,subject_id,id) WHERE work_status='unproven' AND source<>'native'`,
		`CREATE INDEX IF NOT EXISTS idx_usage_bucket_unproven ON session_usage_buckets(scope_id,subject_id,id) WHERE work_status='unproven' AND source<>'native'`,
		`CREATE INDEX IF NOT EXISTS idx_usage_measurement_unindexed ON session_usage_measurements(scope_id,id) WHERE selection_partition='' AND scope_id<>''`,
		`CREATE INDEX IF NOT EXISTS idx_usage_bucket_unindexed ON session_usage_buckets(scope_id,id) WHERE selection_partition='' AND scope_id<>''`,
	} {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
