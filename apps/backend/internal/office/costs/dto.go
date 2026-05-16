package costs

// CostListResponse wraps a list of cost events.
type CostListResponse struct {
	Costs []*CostEvent `json:"costs"`
}

// CostBreakdownResponse wraps aggregated cost data.
type CostBreakdownResponse struct {
	Breakdown []*CostBreakdown `json:"breakdown"`
}

// CostBreakdownComposedResponse bundles the four cost views surfaced on the
// costs overview page (total + by-agent / by-project / by-model breakdowns)
// into a single response so the page renders in one round-trip — was four
// parallel calls (Stream D of office optimization). Aggregations run inside
// a single read transaction so the four views are snapshot-consistent.
type CostBreakdownComposedResponse struct {
	TotalSubcents int64            `json:"total_subcents"`
	ByAgent       []*CostBreakdown `json:"by_agent"`
	ByProject     []*CostBreakdown `json:"by_project"`
	ByModel       []*CostBreakdown `json:"by_model"`
	ByProvider    []*CostBreakdown `json:"by_provider"`
}

// CreateBudgetRequest is the request body for creating a budget policy.
// LimitSubcents is hundredths of a cent (see BudgetPolicy doc).
type CreateBudgetRequest struct {
	ScopeType         string `json:"scope_type"`
	ScopeID           string `json:"scope_id"`
	LimitSubcents     int64  `json:"limit_subcents"`
	Period            string `json:"period"`
	AlertThresholdPct int    `json:"alert_threshold_pct"`
	ActionOnExceed    string `json:"action_on_exceed"`
}

// UpdateBudgetRequest is the request body for updating a budget policy.
type UpdateBudgetRequest struct {
	ScopeType         *string `json:"scope_type,omitempty"`
	ScopeID           *string `json:"scope_id,omitempty"`
	LimitSubcents     *int64  `json:"limit_subcents,omitempty"`
	Period            *string `json:"period,omitempty"`
	AlertThresholdPct *int    `json:"alert_threshold_pct,omitempty"`
	ActionOnExceed    *string `json:"action_on_exceed,omitempty"`
}

// BudgetListResponse wraps a list of budget policies.
type BudgetListResponse struct {
	Budgets []*BudgetPolicy `json:"budgets"`
}
