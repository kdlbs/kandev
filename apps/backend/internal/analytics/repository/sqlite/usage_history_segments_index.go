package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kdlbs/kandev/pluginsdk"
)

func (r *Repository) persistUsageSegmentTx(ctx context.Context, tx *sqlx.Tx, row *indexedObservation, input *pluginsdk.UsageObservationV1) error {
	setUsageObservationSegment(row, input)
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE `+row.Table+` SET selection_bundle=?,selection_segment=?,selection_group=?,selection_layout=?,selection_token_bundle=?,selection_cost_bundle=?,selection_scan_key=?,selection_span_seconds=? WHERE id=?`),
		row.Bundle, row.Segment, row.GroupKey, row.Layout, row.TokenBundle, row.CostBundle, row.ScanKey, usageSegmentSpanSeconds(*row), row.Measurement.ID)
	return err
}

func unsupportedUsageSegmentVersion(batch pluginsdk.UsageObservationBatchV1) bool {
	if batch.SchemaVersion != pluginsdk.UsageObservationSchemaVersionV1 && batch.SchemaVersion != pluginsdk.UsageObservationSchemaVersionV2 {
		return true
	}
	for _, item := range batch.Observations {
		if item.Segment != nil && batch.SchemaVersion < pluginsdk.UsageObservationSchemaVersionV2 {
			return true
		}
	}
	return false
}

func sameUsageSegmentDeclaration(before, after *pluginsdk.UsageSegmentIdentityV1) bool {
	if before == nil {
		return true
	}
	return after != nil && before.DecompositionID == after.DecompositionID && before.SegmentID == after.SegmentID
}
