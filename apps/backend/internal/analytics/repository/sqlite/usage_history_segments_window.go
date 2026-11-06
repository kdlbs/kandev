package sqlite

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type usageSegmentWindow struct {
	Coverage   string
	Start, End *time.Time
}

func usageSegmentRowWindow(row indexedObservation) usageSegmentWindow {
	w := usageSegmentWindow{Coverage: row.CoverageKey, Start: row.Measurement.CoverageStart, End: row.Measurement.CoverageEnd}
	if w.Start != nil && (w.End == nil || !w.End.After(*w.Start)) {
		end := w.Start.Add(time.Nanosecond)
		w.End = &end
	}
	return w
}

func (w usageSegmentWindow) overlaps(row indexedObservation) bool {
	other := usageSegmentRowWindow(row)
	if w.Start == nil || other.Start == nil {
		return w.Start == nil && other.Start == nil
	}
	return w.Start.Before(*other.End) && other.Start.Before(*w.End)
}

func (w usageSegmentWindow) reusable(row indexedObservation) bool {
	if w.Coverage == row.CoverageKey {
		return true
	}
	other := usageSegmentRowWindow(row)
	return row.Measurement.Source == usageSourceNative && w.Start != nil && other.Start != nil &&
		!other.Start.Before(*w.Start) && other.End.Before(*w.End)
}

func (r *Repository) usageSegmentWindowBoundsTx(ctx context.Context, tx *sqlx.Tx, scope, work string, window usageSegmentWindow) (string, string, error) {
	if window.Start == nil {
		return "0", "1", nil
	}
	var span int64
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		var candidate int64
		query := `SELECT COALESCE((SELECT selection_span_seconds FROM ` + table + ` WHERE scope_id=? AND selection_partition=? AND retired=0 ORDER BY selection_span_seconds DESC LIMIT 1),0)`
		if err := tx.GetContext(ctx, &candidate, r.db.Rebind(query), scope, work); err != nil {
			return "", "", err
		}
		span = max(span, candidate)
	}
	start := time.Unix(window.Start.Unix()-span, 0).UTC()
	return usageSegmentTimeKey(&start), usageSegmentTimeKey(window.End), nil
}

func usageSegmentWindowChoice(rows []indexedObservation, window usageSegmentWindow, choice usageSegmentChoice) usageSegmentChoice {
	state := usageSegmentAssessment{Choice: choice}
	for _, row := range rows {
		if window.overlaps(row) {
			state.add(row)
		}
	}
	return state.finish()
}
