package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// costSourceUnpriced mirrors internal/task/usage.CostSourceUnpriced. It is
// redeclared here rather than imported: internal/task/usage already imports
// this package to satisfy its Repository interface, so importing back would
// cycle.
const costSourceUnpriced = "unpriced"

// usageTotalsAggregateQuery is AC-18/AC-19's read-side aggregation: every sum
// is order-independent (SUM, COUNT, MIN, MAX), so it needs none of
// ListTaskUsageEvents' (occurred_at, id) ordering (AC-16). A nullable token
// column is coalesced to zero before summing (AC-12) so one not-recorded
// sample can never null out the whole total. tokens_total sums the stored
// per-row column verbatim - it is never recomputed from the per-kind sums at
// read time (AC-19).
const usageTotalsAggregateQuery = `
	SELECT
		COUNT(*),
		COALESCE(SUM(tokens_in), 0),
		COALESCE(SUM(COALESCE(tokens_cached_read, 0)), 0),
		COALESCE(SUM(COALESCE(tokens_cached_write, 0)), 0),
		COALESCE(SUM(COALESCE(tokens_out, 0)), 0),
		COALESCE(SUM(COALESCE(tokens_thought, 0)), 0),
		COALESCE(SUM(tokens_total), 0),
		COALESCE(SUM(cost_subcents), 0),
		COALESCE(SUM(CASE WHEN estimated = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN cost_source = ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN tokens_out IS NULL THEN 1 ELSE 0 END), 0),
		MIN(occurred_at),
		MAX(occurred_at)
	  FROM task_usage_events
	 WHERE %s = ?
`

// GetTaskUsageTotals aggregates every ledger row for taskID, including rows
// whose session_id has been cleared by session deletion (AC-18, AC-19).
// An unknown task, and a known task with no rows, both return a zeroed
// TaskUsageTotals and a nil error (AC-20) - the caller enforces the 404 for
// an unknown task, since the aggregate query alone cannot distinguish the
// two.
func (r *Repository) GetTaskUsageTotals(ctx context.Context, taskID string) (*models.TaskUsageTotals, error) {
	return r.scanUsageTotals(ctx, "task_id", taskID)
}

// GetSessionUsageTotals aggregates only the ledger rows still bound to
// sessionID (AC-18, AC-19). A session with no rows returns a zeroed
// TaskUsageTotals and a nil error (AC-20).
func (r *Repository) GetSessionUsageTotals(ctx context.Context, sessionID string) (*models.TaskUsageTotals, error) {
	return r.scanUsageTotals(ctx, "session_id", sessionID)
}

// scanUsageTotals runs usageTotalsAggregateQuery scoped to one column.
// scopeColumn is always one of the two internal literals passed by
// GetTaskUsageTotals/GetSessionUsageTotals, never caller input, so
// interpolating it into the query text carries no injection risk.
func (r *Repository) scanUsageTotals(ctx context.Context, scopeColumn, scopeValue string) (*models.TaskUsageTotals, error) {
	if scopeColumn != "task_id" && scopeColumn != "session_id" {
		return nil, fmt.Errorf("scanUsageTotals: unsupported scope column %q", scopeColumn)
	}
	query := r.ro.Rebind(fmt.Sprintf(usageTotalsAggregateQuery, scopeColumn))

	var (
		totals                     models.TaskUsageTotals
		rawFirstEventAt            interface{}
		rawLastEventAt             interface{}
		unpricedEventCount         sql.NullInt64
		outputIncompleteEventCount sql.NullInt64
	)
	err := r.ro.QueryRowxContext(ctx, query, costSourceUnpriced, scopeValue).Scan(
		&totals.EventCount,
		&totals.TokensIn,
		&totals.TokensCachedRead,
		&totals.TokensCachedWrite,
		&totals.TokensOut,
		&totals.TokensThought,
		&totals.TokensTotal,
		&totals.CostSubcents,
		&totals.EstimatedEventCount,
		&unpricedEventCount,
		&outputIncompleteEventCount,
		&rawFirstEventAt,
		&rawLastEventAt,
	)
	if err != nil {
		return nil, err
	}

	totals.UnpricedEventCount = unpricedEventCount.Int64
	totals.OutputTokensComplete = outputIncompleteEventCount.Int64 == 0

	firstEventAt, err := parseUsageTotalsTimestamp(rawFirstEventAt)
	if err != nil {
		return nil, fmt.Errorf("parse first_event_at: %w", err)
	}
	totals.FirstEventAt = firstEventAt
	lastEventAt, err := parseUsageTotalsTimestamp(rawLastEventAt)
	if err != nil {
		return nil, fmt.Errorf("parse last_event_at: %w", err)
	}
	totals.LastEventAt = lastEventAt

	return &totals, nil
}

// parseUsageTotalsTimestamp converts a MIN/MAX(occurred_at) scan result to a
// *time.Time, or nil for SQL NULL (no contributing rows). Scanned as
// interface{} because SQLite returns an aggregate over a TIMESTAMP column as
// TEXT/[]byte while PostgreSQL returns a native time.Time; reuses
// parseTaskActivityTime (task_status_summary.go), which already handles both.
func parseUsageTotalsTimestamp(raw interface{}) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	parsed, err := parseTaskActivityTime(raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
