package sqlite

import (
	"context"
	"encoding/base64"
	"slices"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

func (r *Repository) selectUsageSegmentedWorkTx(ctx context.Context, tx *sqlx.Tx, query models.UsageSelectionQuery, scope, work string, choice usageSegmentChoice) (models.UsageSelectionPage, error) {
	page := models.UsageSelectionPage{}
	after, err := decodeUsageSelectionCursor(query.Cursor, work)
	if err != nil {
		return page, err
	}
	budget := max(1, maxSessionUsageBatch-choice.Probes)
	rows, err := r.usageSegmentScanTx(ctx, tx, scope, work, "", budget+1)
	if err != nil {
		return page, err
	}
	page.VisitedRows = len(rows) + choice.Probes
	if len(rows) > budget {
		page.HasMore, page.Preparing = true, true
		return page, nil
	}
	allRows := slices.Clone(rows)
	slices.SortFunc(rows, func(a, b indexedObservation) int { return strings.Compare(a.GroupKey, b.GroupKey) })
	return r.selectUsageSegmentRowsTx(ctx, tx, query, work, choice, rows, allRows, page, after)
}

func (r *Repository) selectUsageSegmentRowsTx(ctx context.Context, tx *sqlx.Tx, query models.UsageSelectionQuery, work string, choice usageSegmentChoice, rows, allRows []indexedObservation, page models.UsageSelectionPage, after string) (models.UsageSelectionPage, error) {
	limit := projectionLimit(query.Limit)
	for pos := 0; pos < len(rows); {
		end := pos + 1
		for end < len(rows) && rows[end].GroupKey == rows[pos].GroupKey {
			end++
		}
		if rows[pos].GroupKey <= after {
			pos = end
			continue
		}
		if len(page.Facts) >= limit {
			page.HasMore = true
			break
		}
		local := usageSegmentWindowChoice(allRows, usageSegmentRowWindow(rows[pos]), choice)
		state, err := r.usageSegmentGroupStateTx(ctx, tx, local, rows[pos])
		if err != nil {
			return page, err
		}
		for _, row := range rows[pos:end] {
			state.add(row)
		}
		if fact, keep := state.fact(); keep {
			page.Facts = append(page.Facts, fact)
		}
		after, pos = rows[pos].GroupKey, end
	}
	if page.HasMore && after != "" {
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(work + "\x00" + after))
	}
	return page, nil
}

func (r *Repository) usageSegmentReferencesTx(ctx context.Context, tx *sqlx.Tx, scope, work, afterGroup, afterTable, afterID string, exact bool, limit int) ([]selectionReference, error) {
	var selects []string
	var args []any
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		if exact && table < afterTable {
			continue
		}
		query := `SELECT id,'` + table + `' AS source_table,selection_group AS selection_coverage FROM ` + table + ` WHERE scope_id=? AND selection_partition=? AND retired=0`
		args = append(args, scope, work)
		if !exact {
			query += ` AND selection_group>?`
			args = append(args, afterGroup)
		} else {
			query += ` AND selection_group=?`
			args = append(args, afterGroup)
			if table == afterTable {
				query += ` AND id>?`
				args = append(args, afterID)
			}
		}
		selects = append(selects, query)
	}
	order := ` ORDER BY selection_coverage,id LIMIT ?`
	if exact {
		order = ` ORDER BY source_table,id LIMIT ?`
	}
	args = append(args, limit)
	var refs []selectionReference
	err := tx.SelectContext(ctx, &refs, r.db.Rebind(strings.Join(selects, ` UNION ALL `)+order), args...)
	return refs, err
}
