package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

func (r *Repository) nextUsageRepairScopeTx(ctx context.Context, tx *sqlx.Tx) (string, error) {
	queries := []string{
		`SELECT scope_id FROM usage_scopes WHERE work_pending=1 ORDER BY scope_id LIMIT 1`,
		`SELECT scope_id FROM usage_scopes WHERE segment_version=0 ORDER BY scope_id LIMIT 1`,
		`SELECT scope_id FROM usage_subjects WHERE work_repair_pending=1 ORDER BY scope_id,subject_id LIMIT 1`,
	}
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		for _, column := range []string{"selection_bundle", "selection_partition", "selection_scan_key"} {
			queries = append(queries, `SELECT scope_id FROM `+table+` WHERE `+column+`='' AND scope_id<>'' ORDER BY scope_id,id LIMIT 1`)
		}
	}
	winner := ""
	for _, query := range queries {
		var scope string
		err := tx.GetContext(ctx, &scope, query)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		if scope != "" && (winner == "" || scope < winner) {
			winner = scope
		}
	}
	return winner, nil
}

func (r *Repository) usageInitialRepairReferencesTx(ctx context.Context, tx *sqlx.Tx, scope string, required int64, limit int) ([]selectionReference, error) {
	var refs []selectionReference
	seen := map[string]bool{}
	budget := limit
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		for _, predicate := range []string{`selection_bundle=''`, `selection_partition=''`, `selection_scan_key=''`, `selection_version<?`} {
			if budget == 0 {
				return refs, nil
			}
			query, args := usageInitialRepairQuery(table, scope, predicate, required, budget)
			var page []selectionReference
			if err := tx.SelectContext(ctx, &page, r.db.Rebind(query), args...); err != nil {
				return nil, err
			}
			budget -= len(page)
			for _, ref := range page {
				key := ref.Table + ":" + ref.ID
				if !seen[key] {
					refs, seen[key] = append(refs, ref), true
				}
			}
		}
	}
	return refs, nil
}

func usageInitialRepairQuery(table, scope, predicate string, required int64, limit int) (string, []any) {
	query := fmt.Sprintf(`SELECT id,'%s' AS source_table,selection_coverage FROM %s WHERE scope_id=? AND %s`, table, table, predicate)
	args := []any{scope}
	if predicate == `selection_version<?` {
		query += ` ORDER BY selection_version,id LIMIT ?`
		args = append(args, required)
	} else {
		query += ` AND scope_id<>'' ORDER BY id LIMIT ?`
	}
	return query, append(args, limit)
}

func (r *Repository) usageInitialRepairPendingTx(ctx context.Context, tx *sqlx.Tx, scope string) (bool, error) {
	required, err := r.usageWorkVersionTx(ctx, tx, scope)
	if err != nil {
		return false, err
	}
	refs, err := r.usageInitialRepairReferencesTx(ctx, tx, scope, required, 1)
	return len(refs) > 0, err
}

func (r *Repository) repairUsageScopePageTx(ctx context.Context, tx *sqlx.Tx, scope string, limit int) (bool, error) {
	required, err := r.usageWorkVersionTx(ctx, tx, scope)
	if err != nil {
		return false, err
	}
	refs, err := r.usageInitialRepairReferencesTx(ctx, tx, scope, required, limit)
	if err != nil {
		return false, err
	}
	var changes []models.UsageMutation
	for _, ref := range refs {
		change, err := r.repairUsageWorkRowTx(ctx, tx, ref)
		if err != nil {
			return false, err
		}
		changes = append(changes, change)
	}
	if err := r.groupUsageProjectionMutationsTx(ctx, tx, changes); err != nil {
		return false, err
	}
	pending, err := r.usageInitialRepairPendingTx(ctx, tx, scope)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_scopes SET work_pending=?,segment_version=? WHERE scope_id=?`), boolInt(pending), boolInt(!pending), scope)
	return len(refs) > 0, err
}
