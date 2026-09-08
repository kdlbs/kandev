package models

import "time"

// TaskUsageEvent is one immutable row of internal/task/repository/sqlite's
// task_usage_events ledger (docs/specs/task-cost-ledger/spec.md). A pointer
// token/rate field means the value is *not recorded*, distinct from a
// measured zero (AC-4, AC-30); SessionID, TurnID, and PricingCatalogVersion
// use "" for SQL NULL since they are always either absent or a non-empty
// identifier.
type TaskUsageEvent struct {
	ID             int64
	UsageEventID   string
	TaskID         string
	SessionID      string
	TurnID         string
	AgentProfileID string
	AgentType      string
	Model          string
	Provider       string

	TokensIn          int64
	TokensCachedRead  *int64
	TokensCachedWrite *int64
	TokensOut         *int64
	TokensThought     *int64
	TokensTotal       int64

	CostSubcents int64
	CostSource   string
	Estimated    bool

	RateInputPerMillion       *int64
	RateCachedReadPerMillion  *int64
	RateCachedWritePerMillion *int64
	RateOutputPerMillion      *int64
	PricingCatalogVersion     string

	ContractVersion int
	OccurredAt      time.Time
	CreatedAt       time.Time
}
