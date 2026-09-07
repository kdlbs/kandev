package sqlite

import (
	"context"
	"time"
)

// SpendWindow reports one scope's priced spend and degradation state for a
// single [start, before) window, in one query round-trip.
// AC-OFFICE-BUDGET-002.10/.15, REQ-OFFICE-BUDGET-004.
type SpendWindow struct {
	// PricedSubcents excludes any event whose cost_source is 'unpriced'
	// (AC-OFFICE-BUDGET-004.1); a NULL cost_source is treated as priced.
	PricedSubcents int64
	// Degraded is true when the window contains at least one unpriced
	// event, determined from cost_source, never from estimated.
	Degraded bool
}

// spendWindowQuery is shared by SpendWindowForWorkspace/Agent/Project: each
// caller supplies its own WHERE-clause scope predicate and args, sharing the
// SELECT and the half-open occurred_at bound so a policy and the built-in
// default can never disagree about which events count in the same window
// (AC-OFFICE-BUDGET-001.9). hasStart=false (the `total` period) omits the
// lower bound.
func (r *Repository) spendWindowQuery(
	ctx context.Context, scopeWhere string, scopeArgs []interface{},
	start time.Time, hasStart bool, before time.Time,
) (SpendWindow, error) {
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN e.cost_source = 'unpriced' THEN 0 ELSE e.cost_subcents END), 0) AS priced_subcents,
			COALESCE(SUM(CASE WHEN e.cost_source = 'unpriced' THEN 1 ELSE 0 END), 0) > 0 AS degraded
		FROM office_cost_events e
		` + scopeWhere + ` AND e.occurred_at < ?`
	args := append(append([]interface{}{}, scopeArgs...), before.UTC())
	if hasStart {
		query += ` AND e.occurred_at >= ?`
		args = append(args, start.UTC())
	}

	var out SpendWindow
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), args...).Scan(&out.PricedSubcents, &out.Degraded)
	return out, err
}

// SpendWindowForWorkspace scopes by the event's agent's workspace via
// agent_profiles, never via tasks — closing the orphaned-cost-event gap a
// tasks join produces (W4 human disposition; see costs.go's SumCostsSince
// doc comment for the gap this deliberately does not share).
func (r *Repository) SpendWindowForWorkspace(
	ctx context.Context, workspaceID string, start time.Time, hasStart bool, before time.Time,
) (SpendWindow, error) {
	const where = `JOIN agent_profiles a ON a.id = e.agent_profile_id WHERE a.workspace_id = ?`
	return r.spendWindowQuery(ctx, where, []interface{}{workspaceID}, start, hasStart, before)
}

// SpendWindowForAgent scopes directly on office_cost_events.agent_profile_id.
func (r *Repository) SpendWindowForAgent(
	ctx context.Context, agentInstanceID string, start time.Time, hasStart bool, before time.Time,
) (SpendWindow, error) {
	const where = `WHERE e.agent_profile_id = ?`
	return r.spendWindowQuery(ctx, where, []interface{}{agentInstanceID}, start, hasStart, before)
}

// SpendWindowForProject scopes directly on office_cost_events.project_id,
// the value recorded on the cost event itself at write time — never the
// event's task's current project_id — so reparenting a task cannot move
// historical spend between ceilings (AC-OFFICE-BUDGET-002.15).
func (r *Repository) SpendWindowForProject(
	ctx context.Context, projectID string, start time.Time, hasStart bool, before time.Time,
) (SpendWindow, error) {
	const where = `WHERE e.project_id = ?`
	return r.spendWindowQuery(ctx, where, []interface{}{projectID}, start, hasStart, before)
}
