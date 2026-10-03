package models

import "time"

// SessionUsageMeasurement is one source-aware cumulative or dated usage
// observation. A nil token or cost field means that the source did not report
// that value. It is different from a reported zero.
type SessionUsageMeasurement struct {
	ID                string
	WorkspaceID       string
	TaskID            string
	SessionID         string
	TranscriptID      string
	Source            string
	SourceRecordID    string
	UsageIdentity     string
	Model             string
	Provider          string
	SourceVersion     string
	Revision          int64
	PayloadDigest     string
	ObservedAt        time.Time
	CollectedAt       time.Time
	InputTokens       *int64
	OutputTokens      *int64
	CacheReadTokens   *int64
	CacheWriteTokens  *int64
	ReasoningTokens   *int64
	TotalTokens       *int64
	Turns             *int64
	CostSubcents      *int64
	Currency          string
	CostBasis         string
	Coverage          string
	CostCoverage      string
	SourceTimezone    string
	AttributionStatus string
	CoverageStart     *time.Time
	CoverageEnd       *time.Time
	SourceDate        string
	CoverageKey       string
	Estimated         bool
	Stale             bool
}

// UsageMeasurement is kept as a concise alias for plugin and service callers.
type UsageMeasurement = SessionUsageMeasurement

const (
	UsageCostBasisReported  = "reported"
	UsageCostBasisEstimated = "estimated"
	UsageCostBasisUnknown   = "unknown"
	UsageCostBasisMixed     = "mixed"

	UsageCoverageComplete = "complete"
	UsageCoveragePartial  = "partial"
	UsageCoverageMissing  = "missing"

	UsageAttributionAttributed = "attributed"
	UsageAttributionAmbiguous  = "ambiguous"
	UsageAttributionUnknown    = "unknown"
)

// SessionUsageWriteStatus describes how a cumulative observation was handled.
type SessionUsageWriteStatus string

const (
	UsageWriteApplied   SessionUsageWriteStatus = "applied"
	UsageWriteUnchanged SessionUsageWriteStatus = "unchanged"
	UsageWriteStale     SessionUsageWriteStatus = "stale"
	UsageWriteConflict  SessionUsageWriteStatus = "conflict"
	UsageWriteInvalid   SessionUsageWriteStatus = "invalid"
)

type SessionUsageUpsertResult struct {
	SourceRecordID string                   `json:"source_record_id"`
	UsageIdentity  string                   `json:"usage_identity"`
	Model          string                   `json:"model"`
	Provider       string                   `json:"provider"`
	Status         SessionUsageWriteStatus  `json:"status"`
	Measurement    *SessionUsageMeasurement `json:"measurement,omitempty"`
	Error          string                   `json:"error,omitempty"`
}

// SessionUsageFilter is used by Host reads and the native token usage page.
// Start is inclusive and End is exclusive. A range query uses dated buckets;
// undated lifetime snapshots are returned only when IncludeUndated is true.
type SessionUsageFilter struct {
	WorkspaceID string
	// Source is an internal service-side narrowing key. Browser callers leave
	// it empty; plugin Host reads set it from the authenticated plugin id so a
	// plugin can only read its own saved measurements.
	Source         string
	SessionIDs     []string
	TaskIDs        []string
	Model          string
	Provider       string
	Start          *time.Time
	End            *time.Time
	Timezone       string
	GroupBy        string
	SortBy         string
	SortDirection  string
	IncludeUndated bool
	Limit          int
	Offset         int
	Canonical      bool
}

// UsageTotals contains nullable totals so missing source categories stay
// visible to callers. CostSubcents is in 1/10,000 of a US dollar, matching the
// native task usage ledger.
type UsageTotals struct {
	InputTokens             *int64   `json:"input_tokens"`
	OutputTokens            *int64   `json:"output_tokens"`
	CacheReadTokens         *int64   `json:"cache_read_tokens"`
	CacheWriteTokens        *int64   `json:"cache_write_tokens"`
	ReasoningTokens         *int64   `json:"reasoning_tokens"`
	TotalTokens             *int64   `json:"total_tokens"`
	Turns                   *int64   `json:"turns"`
	CostSubcents            *int64   `json:"cost_subcents"`
	Currency                string   `json:"currency,omitempty"`
	CostBasis               string   `json:"cost_basis"`
	CostCoverage            string   `json:"cost_coverage"`
	EffectiveCostPerMillion *float64 `json:"effective_cost_per_million"`
	Estimated               bool     `json:"estimated"`
	Partial                 bool     `json:"partial"`
	Undated                 bool     `json:"undated"`
}

// TokenUsageRow is a complete filtered result row. Rows are grouped by the
// query's GroupBy value and are sorted before pagination.
type TokenUsageRow struct {
	Key                     string   `json:"key"`
	Period                  string   `json:"period,omitempty"`
	Model                   string   `json:"model,omitempty"`
	Provider                string   `json:"provider,omitempty"`
	TaskID                  string   `json:"task_id,omitempty"`
	SessionID               string   `json:"session_id,omitempty"`
	Source                  string   `json:"source,omitempty"`
	InputTokens             *int64   `json:"input_tokens"`
	OutputTokens            *int64   `json:"output_tokens"`
	CacheReadTokens         *int64   `json:"cache_read_tokens"`
	CacheWriteTokens        *int64   `json:"cache_write_tokens"`
	ReasoningTokens         *int64   `json:"reasoning_tokens"`
	TotalTokens             *int64   `json:"total_tokens"`
	Turns                   *int64   `json:"turns"`
	CostSubcents            *int64   `json:"cost_subcents"`
	Currency                string   `json:"currency,omitempty"`
	CostBasis               string   `json:"cost_basis"`
	CostCoverage            string   `json:"cost_coverage"`
	EffectiveCostPerMillion *float64 `json:"effective_cost_per_million"`
	Coverage                string   `json:"coverage"`
	Estimated               bool     `json:"estimated"`
	Stale                   bool     `json:"stale"`
	Undated                 bool     `json:"undated"`
}

type TokenUsageReport struct {
	Summary          UsageTotals     `json:"summary"`
	Rows             []TokenUsageRow `json:"rows"`
	Totals           UsageTotals     `json:"totals"`
	TotalRows        int             `json:"total_rows"`
	HasMore          bool            `json:"has_more"`
	HasHistory       bool            `json:"has_history"`
	HasRangeData     bool            `json:"has_range_data"`
	HasNativeHistory bool            `json:"has_native_history"`
	DatedCoverage    bool            `json:"dated_coverage"`
	UndatedCoverage  bool            `json:"undated_coverage"`
	Providers        []string        `json:"providers"`
	LastUpdated      *time.Time      `json:"last_updated,omitempty"`
}

func Int64(value int64) *int64 { return &value }
