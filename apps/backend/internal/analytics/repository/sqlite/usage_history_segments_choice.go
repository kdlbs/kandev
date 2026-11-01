package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

type usageSegmentChoice struct {
	Enabled, Ambiguous bool
	Tokens, Cost       string
	Probes             int
	Ready              bool
	TokenQuality       usageSegmentMetricQuality
	CostQuality        usageSegmentMetricQuality
}

func (r *Repository) usageSegmentChoiceTx(ctx context.Context, tx *sqlx.Tx, scope, work string) (usageSegmentChoice, error) {
	choice := usageSegmentChoice{}
	var legacy string
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		var declared string
		query := `SELECT id FROM ` + table + ` WHERE scope_id=? AND selection_partition=? AND retired=0 AND selection_layout IN (1,3) LIMIT 1`
		if err := usageSegmentProbe(ctx, tx, r.db.Rebind(query), &declared, &choice.Probes, scope, work); err != nil {
			return choice, err
		}
		choice.Enabled = choice.Enabled || declared != ""
		if choice.Enabled {
			break
		}
		var first string
		query = `SELECT selection_segment FROM ` + table + ` WHERE scope_id=? AND selection_partition=? AND retired=0 AND selection_layout=2 AND selection_segment>? ORDER BY selection_segment LIMIT 1`
		if err := usageSegmentProbe(ctx, tx, r.db.Rebind(query), &first, &choice.Probes, scope, work, ""); err != nil {
			return choice, err
		}
		if first == "" {
			continue
		}
		choice.Enabled = legacy != "" && first != legacy
		legacy = first
		var second string
		if err := usageSegmentProbe(ctx, tx, r.db.Rebind(query), &second, &choice.Probes, scope, work, first); err != nil {
			return choice, err
		}
		choice.Enabled = choice.Enabled || second != ""
	}
	if !choice.Enabled {
		return choice, nil
	}
	return choice, nil
}

func usageSegmentProbe(ctx context.Context, tx *sqlx.Tx, query string, out *string, probes *int, args ...any) error {
	err := tx.GetContext(ctx, out, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err == nil {
		(*probes)++
	}
	return err
}
