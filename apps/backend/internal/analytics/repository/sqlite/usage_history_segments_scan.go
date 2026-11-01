package sqlite

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"
)

type usageSegmentScanReference struct {
	ID    string `db:"id"`
	Table string `db:"source_table"`
	Key   string `db:"selection_scan_key"`
}

func (r *Repository) usageSegmentScanTx(ctx context.Context, tx *sqlx.Tx, scope, work, after string, limit int) ([]indexedObservation, error) {
	return r.usageSegmentScanRangeTx(ctx, tx, scope, work, after, "", "", limit)
}

func (r *Repository) usageSegmentScanRangeTx(ctx context.Context, tx *sqlx.Tx, scope, work, after, lower, upper string, limit int) ([]indexedObservation, error) {
	var selects []string
	var args []any
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		query := `SELECT id,'` + table + `' AS source_table,selection_scan_key FROM ` + table + ` WHERE scope_id=? AND selection_partition=? AND retired=0 AND selection_scan_key>?`
		args = append(args, scope, work, after)
		if upper != "" {
			query += ` AND selection_scan_key>=? AND selection_scan_key<?`
			args = append(args, lower, upper)
		}
		selects = append(selects, query)
	}
	args = append(args, limit)
	var refs []usageSegmentScanReference
	if err := tx.SelectContext(ctx, &refs, r.db.Rebind(strings.Join(selects, ` UNION ALL `)+` ORDER BY selection_scan_key LIMIT ?`), args...); err != nil {
		return nil, err
	}
	rows := make([]indexedObservation, 0, len(refs))
	for _, ref := range refs {
		row, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}
