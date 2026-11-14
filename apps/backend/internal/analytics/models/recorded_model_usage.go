package models

// RecordedModelUsage groups immutable event observations, including unknown
// model/provider groups. Nullable token fields remain unknown when any event
// in the group lacks that measurement. There is no account identity here.
type RecordedModelUsage struct {
	Model             string `json:"model" db:"model"`
	Provider          string `json:"provider" db:"provider"`
	MeasurementSource string `json:"measurement_source" db:"measurement_source"`
	Completeness      string `json:"usage_completeness" db:"usage_completeness"`
	CostSource        string `json:"cost_source" db:"cost_source"`
	Estimated         bool   `json:"estimated" db:"estimated"`
	EventCount        int    `json:"event_count" db:"event_count"`
	TokensIn          int64  `json:"tokens_in" db:"tokens_in"`
	TokensOut         *int64 `json:"tokens_out" db:"tokens_out"`
	TokensCachedRead  *int64 `json:"tokens_cached_read" db:"tokens_cached_read"`
	TokensCachedWrite *int64 `json:"tokens_cached_write" db:"tokens_cached_write"`
	TokensTotal       int64  `json:"tokens_total" db:"tokens_total"`
	CostSubcents      int64  `json:"cost_subcents" db:"cost_subcents"`
}
