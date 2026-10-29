package sqlite

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"
)

var usageProjectionTableKeys = []struct{ table, key string }{
	{"usage_projection_contributions", "scope_id, generation, build_id, fact_key, metric_kind, calendar, usage_date"},
	{"usage_canonical_facts", "scope_id, generation, build_id, fact_key"},
	{"usage_daily_summaries", "scope_id, generation, summary_key"},
	{"usage_lifetime_summaries", "scope_id, generation, summary_key"},
	{"usage_projection_summary_queue", "scope_id, generation, summary_key"},
	{"usage_projection_resolutions", "scope_id, generation, work_key"},
	{"usage_projection_replay_groups", "scope_id, generation, group_id"},
	{"usage_projection_builds", "scope_id, generation, build_id"},
	{"usage_projection_applied", "scope_id, generation, work_key"},
	{"usage_projection_history", "scope_id, generation, work_key"},
	{"usage_projection_work", "scope_id, generation, work_key"},
	{"usage_projection_items", "scope_id, generation, source_table, source_id"},
	{"usage_projection_checkpoints", "scope_id, generation, source_table"},
	{"usage_projection_generations", "scope_id, generation"},
	{"usage_projection_source_versions", "scope_id, source_table, source_id"},
	{"usage_projection_partitions", "scope_id, work_key"},
	{"usage_projection_log", "scope_id, commit_sequence"},
	{"usage_projection_scopes", "scope_id"},
}

func (r *Repository) deleteUsageProjectionScopesTx(ctx context.Context, tx *sqlx.Tx, filter string, args ...any) error {
	for _, entry := range usageProjectionTableKeys {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM `+entry.table+` WHERE `+filter), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) eraseUsageProjectionBatchTx(ctx context.Context, tx *sqlx.Tx, filter string, limit int, args ...any) (bool, error) {
	for _, entry := range usageProjectionTableKeys {
		key := entry.key
		if strings.Contains(key, ",") {
			key = "(" + key + ")"
		}
		query := `DELETE FROM ` + entry.table + ` WHERE ` + key + ` IN (SELECT ` + entry.key + ` FROM ` + entry.table + ` WHERE ` + filter + ` LIMIT ?)`
		queryArgs := append(append([]any{}, args...), limit)
		result, err := tx.ExecContext(ctx, r.db.Rebind(query), queryArgs...)
		if err != nil {
			return false, err
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if deleted == int64(limit) {
			return false, nil
		}
	}
	return true, nil
}
