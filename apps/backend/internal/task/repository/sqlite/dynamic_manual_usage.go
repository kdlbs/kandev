package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// ManualWindowUsage is the recorded usage of one concrete execution profile
// inside one reset window. It is a recorded-only total: usage that Kandev never
// observed, and spend deleted outside the ledger, cannot be recovered by this
// query, so the value is a lower bound rather than a provider-wide remaining
// quota.
type ManualWindowUsage struct {
	RequestCount     int64
	TurnCount        int64
	RequestEstimated bool
	TokensTotal      int64
	CostSubcents     int64
	EventCount       int64
	UnpricedCount    int64
	IncompleteCount  int64
	// TurnsAttributed counts rows resolved through the stored concrete turn
	// profile, which is the preferred evidence. ProfileAttributed counts the
	// narrower fallback where a concrete event profile was proven directly.
	TurnsAttributed   int64
	ProfileAttributed int64
}

// Complete reports whether every contributing event carried both a price and a
// complete measurement. An incomplete total is still returned, so the caller can
// show a recorded lower bound instead of pretending the window is exhausted.
func (u ManualWindowUsage) Complete() bool {
	return u.UnpricedCount == 0 && u.IncompleteCount == 0
}

// usageCompletenessComplete is the ledger's complete measurement marker.
const usageCompletenessComplete = "complete"

// manualWindowUsageQuery resolves a usage event to the concrete profile that
// actually executed it.
//
// task_usage_events.agent_profile_id carries the logical session profile, which
// for a dynamic session is the dynamic profile rather than the concrete
// candidate. The concrete identity therefore comes from the stored turn
// (task_session_turns.execution_profile_id), never from the session's mutable
// current route. A row whose turn is missing or deleted is unattributed, except
// for the narrow fallback where the event itself proves a concrete profile,
// which the caller has already established is not a dynamic profile.
const manualWindowUsageQuery = `
	SELECT
		COUNT(*),
		COALESCE(SUM(COALESCE(e.tokens_total, 0)), 0),
		COALESCE(SUM(COALESCE(e.cost_subcents, 0)), 0),
		COALESCE(SUM(CASE WHEN e.cost_source = ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN e.usage_completeness <> ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN t.id IS NOT NULL THEN 1 ELSE 0 END), 0)
		,COUNT(DISTINCT COALESCE(NULLIF(e.turn_id,''), e.usage_event_id))
		,COALESCE(SUM(CASE WHEN e.provider_response_id='' THEN 1 ELSE 0 END),0)
	  FROM task_usage_events e
	 LEFT JOIN task_session_turns t ON t.id = e.turn_id
	 WHERE e.occurred_at >= ?
	   AND e.occurred_at < ?
	   AND (
	     t.execution_profile_id IN (?)
	     OR (t.id IS NULL AND e.agent_profile_id IN (?))
	   )
`

// GetManualWindowUsage aggregates one concrete execution profile's recorded
// usage over the half-open interval [start, end). The interval is clamped by
// the caller to [windowStart, min(now, reset)), so a reset boundary excludes the
// event exactly at the reset instant.
//
// A successful query with no rows is zero *recorded* usage, which is not proof
// that a subscription has never been used.
func (r *Repository) GetManualWindowUsage(
	ctx context.Context,
	executionProfileID string,
	start time.Time,
	end time.Time,
) (ManualWindowUsage, error) {
	return r.GetManualWindowUsageForProfiles(ctx, []string{executionProfileID}, start, end)
}

// GetManualWindowUsageForProfiles aggregates the recorded usage of several
// concrete execution profiles over [start, end). It answers an account-scoped
// window, where a provider enforces one quota across every profile that uses
// the same account. Each event is attributed exactly as for a single profile,
// so an event counts once even when several of the profiles are listed.
func (r *Repository) GetManualWindowUsageForProfiles(
	ctx context.Context,
	executionProfileIDs []string,
	start time.Time,
	end time.Time,
) (ManualWindowUsage, error) {
	ids := make([]string, 0, len(executionProfileIDs))
	for _, id := range executionProfileIDs {
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ManualWindowUsage{}, fmt.Errorf("manual window usage requires a concrete execution profile")
	}
	if !end.After(start) {
		return ManualWindowUsage{}, nil
	}
	query, args, err := sqlx.In(manualWindowUsageQuery,
		costSourceUnpriced, usageCompletenessComplete, start, end, ids, ids)
	if err != nil {
		return ManualWindowUsage{}, err
	}
	usage := ManualWindowUsage{}
	var attributedByTurn int64
	var summaryRows int64
	err = r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), args...).Scan(
		&usage.EventCount,
		&usage.TokensTotal,
		&usage.CostSubcents,
		&usage.UnpricedCount,
		&usage.IncompleteCount,
		&attributedByTurn,
		&usage.TurnCount,
		&summaryRows,
	)
	if err != nil {
		return ManualWindowUsage{}, err
	}
	usage.TurnsAttributed = attributedByTurn
	usage.RequestCount = usage.EventCount
	usage.RequestEstimated = summaryRows > 0
	usage.ProfileAttributed = usage.EventCount - attributedByTurn
	if usage.ProfileAttributed < 0 {
		usage.ProfileAttributed = 0
	}
	return usage, nil
}

var _ = models.TaskUsageTotals{}
