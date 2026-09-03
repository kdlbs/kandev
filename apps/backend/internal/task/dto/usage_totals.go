package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TaskUsageTotalsScope identifies whether a TaskUsageTotalsDTO aggregates an
// entire task or a single session within it (docs/specs/task-cost-ledger/
// spec.md API surface).
type TaskUsageTotalsScope string

const (
	TaskUsageTotalsScopeTask    TaskUsageTotalsScope = "task"
	TaskUsageTotalsScopeSession TaskUsageTotalsScope = "session"
)

// TaskUsageTotalsDTO is the exact JSON shape both usage-totals HTTP routes
// return (AC-18, AC-19, AC-20). FirstEventAt/LastEventAt serialize to JSON
// null, never omitted, when the scope has no contributing rows.
type TaskUsageTotalsDTO struct {
	Scope                TaskUsageTotalsScope `json:"scope"`
	ScopeID              string               `json:"scope_id"`
	TokensIn             int64                `json:"tokens_in"`
	TokensCachedRead     int64                `json:"tokens_cached_read"`
	TokensCachedWrite    int64                `json:"tokens_cached_write"`
	TokensOut            int64                `json:"tokens_out"`
	TokensThought        int64                `json:"tokens_thought"`
	TokensTotal          int64                `json:"tokens_total"`
	CostSubcents         int64                `json:"cost_subcents"`
	EventCount           int64                `json:"event_count"`
	EstimatedEventCount  int64                `json:"estimated_event_count"`
	UnpricedEventCount   int64                `json:"unpriced_event_count"`
	OutputTokensComplete bool                 `json:"output_tokens_complete"`
	FirstEventAt         *time.Time           `json:"first_event_at"`
	LastEventAt          *time.Time           `json:"last_event_at"`
}

// ToTaskUsageTotalsDTO combines a repository-layer aggregate with the scope
// and scope ID the HTTP route resolved it for.
func ToTaskUsageTotalsDTO(scope TaskUsageTotalsScope, scopeID string, totals *models.TaskUsageTotals) TaskUsageTotalsDTO {
	return TaskUsageTotalsDTO{
		Scope:                scope,
		ScopeID:              scopeID,
		TokensIn:             totals.TokensIn,
		TokensCachedRead:     totals.TokensCachedRead,
		TokensCachedWrite:    totals.TokensCachedWrite,
		TokensOut:            totals.TokensOut,
		TokensThought:        totals.TokensThought,
		TokensTotal:          totals.TokensTotal,
		CostSubcents:         totals.CostSubcents,
		EventCount:           totals.EventCount,
		EstimatedEventCount:  totals.EstimatedEventCount,
		UnpricedEventCount:   totals.UnpricedEventCount,
		OutputTokensComplete: totals.OutputTokensComplete,
		FirstEventAt:         totals.FirstEventAt,
		LastEventAt:          totals.LastEventAt,
	}
}
