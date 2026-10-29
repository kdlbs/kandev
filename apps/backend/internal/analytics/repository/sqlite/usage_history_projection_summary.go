package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

var usageProjectionMissingColumns = []string{"input_missing", "output_missing", "cache_read_missing", "cache_write_missing", "reasoning_missing", "total_missing", "source_total_missing", "turns_missing", "cost_missing", "rate_missing"}

func (r *Repository) rebuildUsageProjectionSummariesTx(ctx context.Context, tx *sqlx.Tx, scope, generation string) error {
	for _, table := range []string{"usage_daily_summaries", "usage_lifetime_summaries"} {
		_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM `+table+` WHERE scope_id = ? AND generation = ? AND summary_key IN (
			SELECT summary_key FROM usage_projection_summary_queue WHERE scope_id = ? AND generation = ?)`), scope, generation, scope, generation)
		if err != nil {
			return err
		}
		predicate := `c.coverage_kind = 'lifetime'`
		if table == "usage_daily_summaries" {
			predicate = `c.coverage_kind <> 'lifetime' AND c.usage_date <> ''`
		}
		query := usageProjectionSummaryInsert(table, predicate)
		if _, err := tx.ExecContext(ctx, r.db.Rebind(query), scope, generation); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_projection_summary_queue WHERE scope_id = ? AND generation = ?`), scope, generation)
	return err
}

func usageProjectionSummaryInsert(table, predicate string) string {
	columns := []string{"scope_id", "generation", "summary_key"}
	columns = append(columns, usageProjectionDimensionColumns...)
	columns = append(columns, usageProjectionAmountColumns...)
	columns = append(columns, usageProjectionMissingColumns...)
	columns = append(columns, "token_count", "cost_count", "partial_count", "estimated_count", "stale_count", "cost_basis")
	values := []string{"c.scope_id", "c.generation", "c.summary_key"}
	for _, column := range usageProjectionDimensionColumns {
		values = append(values, "MIN(c."+column+")")
	}
	for _, column := range usageProjectionAmountColumns {
		values = append(values, "SUM(c."+column+")")
	}
	for i, column := range usageProjectionAmountColumns {
		metric := projectionMetricTokens
		if i >= 8 {
			metric = projectionMetricCost
		}
		values = append(values, "SUM(CASE WHEN c.metric_kind = '"+metric+"' AND c."+column+" IS NULL THEN 1 ELSE 0 END)")
	}
	values = append(values, `SUM(CASE WHEN c.metric_kind = 'tokens' THEN 1 ELSE 0 END)`, `SUM(CASE WHEN c.metric_kind = 'cost' THEN 1 ELSE 0 END)`, "SUM(c.partial)", "SUM(c.estimated)", "SUM(c.stale)", `CASE WHEN MIN(c.cost_basis) = MAX(c.cost_basis) THEN MIN(c.cost_basis) ELSE 'mixed' END`)
	return `INSERT INTO ` + table + ` (` + strings.Join(columns, ", ") + `) SELECT ` + strings.Join(values, ", ") + `
		FROM usage_projection_summary_queue q JOIN usage_projection_contributions c
		ON c.scope_id = q.scope_id AND c.generation = q.generation AND c.summary_key = q.summary_key
		JOIN usage_projection_applied a ON a.scope_id = c.scope_id AND a.generation = c.generation AND a.work_key = c.work_key AND a.build_id = c.build_id
		WHERE q.scope_id = ? AND q.generation = ? AND ` + predicate + ` GROUP BY c.scope_id, c.generation, c.summary_key`
}

type usageProjectionHistory struct {
	Facts     int64          `db:"fact_count"`
	Dated     int64          `db:"dated_count"`
	Lifetime  int64          `db:"lifetime_count"`
	Interval  int64          `db:"interval_count"`
	Native    int64          `db:"native_count"`
	Earliest  sql.NullString `db:"earliest"`
	Latest    sql.NullString `db:"latest"`
	Collected sql.NullString `db:"collected_at"`
}

func (r *Repository) updateUsageProjectionHistoryTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork) error {
	var old, current usageProjectionHistory
	err := tx.GetContext(ctx, &old, r.db.Rebind(`SELECT fact_count, dated_count, lifetime_count, interval_count, native_count, earliest, latest FROM usage_projection_history WHERE scope_id = ? AND generation = ? AND work_key = ?`), work.Scope, work.Generation, work.Work)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	err = tx.GetContext(ctx, &current, r.db.Rebind(`SELECT COUNT(*) AS fact_count,
		COALESCE(SUM(CASE WHEN coverage_kind <> 'lifetime' THEN 1 ELSE 0 END), 0) AS dated_count,
		COALESCE(SUM(CASE WHEN coverage_kind = 'lifetime' THEN 1 ELSE 0 END), 0) AS lifetime_count,
		COALESCE(SUM(CASE WHEN coverage_kind = 'interval' THEN 1 ELSE 0 END), 0) AS interval_count,
		COALESCE(SUM(CASE WHEN token_source = 'native' OR cost_source = 'native' THEN 1 ELSE 0 END), 0) AS native_count,
		MIN(coverage_start) AS earliest, MAX(coverage_end) AS latest, MAX(collected_at) AS collected_at FROM usage_canonical_facts WHERE scope_id = ? AND generation = ? AND work_key = ? AND build_id = ?`), work.Scope, work.Generation, work.Work, work.Build)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_history
		(scope_id, generation, work_key, fact_count, dated_count, lifetime_count, interval_count, native_count, earliest, latest, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(scope_id, generation, work_key) DO UPDATE SET
		fact_count = excluded.fact_count, dated_count = excluded.dated_count, lifetime_count = excluded.lifetime_count,
		interval_count = excluded.interval_count, native_count = excluded.native_count, earliest = excluded.earliest, latest = excluded.latest, collected_at = excluded.collected_at`), work.Scope, work.Generation, work.Work, current.Facts, current.Dated, current.Lifetime, current.Interval, current.Native, current.Earliest, current.Latest, current.Collected)
	if err != nil {
		return err
	}
	return r.commitUsageProjectionMetadataTx(ctx, tx, work, old, current)
}

func (r *Repository) commitUsageProjectionMetadataTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, old, current usageProjectionHistory) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_generations SET fact_count = fact_count + ?, dated_count = dated_count + ?,
		lifetime_count = lifetime_count + ?, interval_count = interval_count + ?, native_count = native_count + ?,
		earliest = (SELECT earliest FROM usage_projection_history WHERE scope_id = ? AND generation = ? AND earliest IS NOT NULL ORDER BY earliest LIMIT 1),
		latest = (SELECT latest FROM usage_projection_history WHERE scope_id = ? AND generation = ? AND latest IS NOT NULL ORDER BY latest DESC LIMIT 1),
		collected_at = (SELECT collected_at FROM usage_projection_history WHERE scope_id = ? AND generation = ? AND collected_at IS NOT NULL ORDER BY collected_at DESC LIMIT 1)
		WHERE scope_id = ? AND generation = ?`), current.Facts-old.Facts, current.Dated-old.Dated, current.Lifetime-old.Lifetime, current.Interval-old.Interval, current.Native-old.Native, work.Scope, work.Generation, work.Scope, work.Generation, work.Scope, work.Generation, work.Scope, work.Generation)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_scopes SET revision = revision + 1 WHERE scope_id = ? AND active_generation = ?`), work.Scope, work.Generation)
	return err
}
