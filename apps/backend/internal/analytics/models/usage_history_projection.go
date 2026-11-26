package models

import "time"

// UsageProjectionMetadata describes the last complete generation. Pending work
// never replaces saved amounts with observations that have not been projected.
type UsageProjectionMetadata struct {
	UsageProjectionState
	Revision     int64
	LastUpdated  *time.Time
	HasHistory   bool
	HasDated     bool
	HasUndated   bool
	HasIntervals bool
	HasNative    bool
	Earliest     *time.Time
	Latest       *time.Time
	HasMore      bool
	NextOffset   int
}

// UsageDailySummary stores known sums beside missing and partial counters.
// Tokens and cost are separate contributions; RateTokens is only a matching,
// complete denominator and never contributes to token totals.
type UsageDailySummary struct {
	ScopeID            string `db:"scope_id"`
	Generation         string `db:"generation"`
	Key                string `db:"summary_key"`
	MetricKind         string `db:"metric_kind"`
	ActorID            string `db:"actor_id"`
	ActorStatus        string `db:"actor_status"`
	ActorKind          string `db:"actor_kind"`
	Source             string `db:"source"`
	Model              string `db:"model"`
	Provider           string `db:"provider"`
	AgentClient        string `db:"agent_client"`
	Semantics          string `db:"semantics"`
	Currency           string `db:"currency"`
	Calendar           string `db:"calendar"`
	Date               string `db:"usage_date"`
	InputTokens        *int64 `db:"input_tokens"`
	OutputTokens       *int64 `db:"output_tokens"`
	CacheReadTokens    *int64 `db:"cache_read_tokens"`
	CacheWriteTokens   *int64 `db:"cache_write_tokens"`
	ReasoningTokens    *int64 `db:"reasoning_tokens"`
	TotalTokens        *int64 `db:"total_tokens"`
	SourceTotalTokens  *int64 `db:"source_total_tokens"`
	Turns              *int64 `db:"turns"`
	CostSubcents       *int64 `db:"cost_subcents"`
	RateTokens         *int64 `db:"rate_tokens"`
	InputMissing       int64  `db:"input_missing"`
	OutputMissing      int64  `db:"output_missing"`
	CacheReadMissing   int64  `db:"cache_read_missing"`
	CacheWriteMissing  int64  `db:"cache_write_missing"`
	ReasoningMissing   int64  `db:"reasoning_missing"`
	TotalMissing       int64  `db:"total_missing"`
	SourceTotalMissing int64  `db:"source_total_missing"`
	TurnsMissing       int64  `db:"turns_missing"`
	CostMissing        int64  `db:"cost_missing"`
	RateMissing        int64  `db:"rate_missing"`
	TokenCount         int64  `db:"token_count"`
	CostCount          int64  `db:"cost_count"`
	PartialCount       int64  `db:"partial_count"`
	EstimatedCount     int64  `db:"estimated_count"`
	StaleCount         int64  `db:"stale_count"`
	CostBasis          string `db:"cost_basis"`
}

func (s UsageDailySummary) EffectiveCostPerMillion() *float64 {
	if s.CostSubcents == nil || s.RateTokens == nil || *s.RateTokens <= 0 || s.RateMissing != 0 ||
		s.CostMissing != 0 || s.PartialCount != 0 || s.CostBasis == UsageCostBasisUnknown || s.CostBasis == UsageCostBasisMixed {
		return nil
	}
	rate := float64(*s.CostSubcents) * 100 / float64(*s.RateTokens)
	return &rate
}
