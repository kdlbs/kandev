package dto

import (
	"sort"
	"strconv"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	usageCompletenessExact   = "exact"
	usageCompletenessPartial = "partial"
)

type UsageTokenBreakdownDTO struct {
	InputTokens       int64   `json:"input_tokens"`
	CachedReadTokens  int64   `json:"cached_read_tokens"`
	CachedWriteTokens int64   `json:"cached_write_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	ThoughtTokens     int64   `json:"thought_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	OutputComplete    bool    `json:"output_complete"`
	EventCount        int     `json:"event_count"`
	UnpricedCount     int     `json:"unpriced_count"`
	CostSubcents      *string `json:"cost_subcents"`
	Overflow          bool    `json:"overflow"`
	costSubcents      int64
	costSeen          bool
	costOverflow      bool
}

type UsageResponseDTO struct {
	UsageEventID             string  `json:"usage_event_id"`
	ProviderResponseID       string  `json:"provider_response_id,omitempty"`
	ProviderThreadID         string  `json:"provider_thread_id,omitempty"`
	ProviderTurnID           string  `json:"provider_turn_id,omitempty"`
	Scope                    string  `json:"scope,omitempty"`
	Source                   string  `json:"source,omitempty"`
	Completeness             string  `json:"completeness,omitempty"`
	Model                    string  `json:"model,omitempty"`
	Provider                 string  `json:"provider,omitempty"`
	InputTokens              int64   `json:"input_tokens"`
	CachedReadTokens         *int64  `json:"cached_read_tokens,omitempty"`
	CachedWriteTokens        *int64  `json:"cached_write_tokens,omitempty"`
	OutputTokens             *int64  `json:"output_tokens,omitempty"`
	ThoughtTokens            *int64  `json:"thought_tokens,omitempty"`
	ReasoningOutputTokens    *int64  `json:"reasoning_output_tokens,omitempty"`
	ReportedCacheWriteTokens *int64  `json:"reported_cache_write_tokens,omitempty"`
	ReportedTotalTokens      *int64  `json:"reported_total_tokens,omitempty"`
	TotalTokens              int64   `json:"total_tokens"`
	CostSubcents             *string `json:"cost_subcents,omitempty"`
	CostSource               string  `json:"cost_source"`
	Estimated                bool    `json:"estimated"`
	OccurredAt               string  `json:"occurred_at"`
}

type UsageTurnDTO struct {
	TurnID       string                 `json:"turn_id"`
	Completeness string                 `json:"completeness"`
	Direct       UsageTokenBreakdownDTO `json:"direct"`
	Child        UsageTokenBreakdownDTO `json:"child"`
	Total        UsageTokenBreakdownDTO `json:"total"`
	CostSources  []string               `json:"cost_sources"`
	LastResponse *UsageResponseDTO      `json:"last_response,omitempty"`
	Responses    []UsageResponseDTO     `json:"responses,omitempty"`
}

type UsageTurnsPageDTO struct {
	SessionID  string         `json:"session_id"`
	Turns      []UsageTurnDTO `json:"turns"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

func ToUsageTurnDTO(rows models.TaskUsageTurnEvents, includeResponses bool) UsageTurnDTO {
	turn := UsageTurnDTO{TurnID: rows.TurnID, Completeness: "unknown", CostSources: []string{}}
	turn.Direct.OutputComplete = true
	turn.Child.OutputComplete = true
	turn.Total.OutputComplete = true
	costSources := make(map[string]struct{})
	completeness := usageTurnCompleteness{}
	for _, row := range rows.Events {
		if row == nil {
			continue
		}
		appendUsageTurnRow(&turn, row, includeResponses, costSources, &completeness)
	}
	switch {
	case turn.Direct.Overflow || turn.Child.Overflow || turn.Total.Overflow:
		turn.Completeness = usageCompletenessPartial
	case completeness.estimated:
		turn.Completeness = "estimated"
	case completeness.partial:
		turn.Completeness = usageCompletenessPartial
	case completeness.exact:
		turn.Completeness = usageCompletenessExact
	}
	for source := range costSources {
		turn.CostSources = append(turn.CostSources, source)
	}
	sort.Strings(turn.CostSources)
	finalizeBucket(&turn.Direct)
	finalizeBucket(&turn.Child)
	finalizeBucket(&turn.Total)
	return turn
}

type usageTurnCompleteness struct {
	estimated bool
	exact     bool
	partial   bool
}

func appendUsageTurnRow(
	turn *UsageTurnDTO,
	row *models.TaskUsageEvent,
	includeResponses bool,
	costSources map[string]struct{},
	completeness *usageTurnCompleteness,
) {
	item := ToUsageResponseDTO(row)
	completeness.estimated = completeness.estimated || row.Estimated || row.UsageCompleteness == "estimated"
	completeness.exact = completeness.exact || row.UsageCompleteness == usageCompletenessExact
	completeness.partial = completeness.partial || row.UsageCompleteness == usageCompletenessPartial ||
		(row.NativeScope != "" && row.UsageCompleteness == "")
	if row.CostSource != "" {
		costSources[row.CostSource] = struct{}{}
	}
	bucket := &turn.Direct
	if row.NativeScope == "child" {
		bucket = &turn.Child
	}
	addUsageRow(bucket, row)
	addUsageRow(&turn.Total, row)
	turn.LastResponse = &item
	if includeResponses {
		turn.Responses = append(turn.Responses, item)
	}
}

func addUsageRow(bucket *UsageTokenBreakdownDTO, row *models.TaskUsageEvent) {
	bucket.EventCount++
	addTokenCount(bucket, &bucket.InputTokens, row.TokensIn)
	addTokenCount(bucket, &bucket.CachedReadTokens, int64Value(row.TokensCachedRead))
	addTokenCount(bucket, &bucket.CachedWriteTokens, int64Value(row.TokensCachedWrite))
	addTokenCount(bucket, &bucket.OutputTokens, int64Value(row.TokensOut))
	addTokenCount(bucket, &bucket.ThoughtTokens, int64Value(row.TokensThought))
	addTokenCount(bucket, &bucket.TotalTokens, row.TokensTotal)
	if row.TokensOut == nil {
		bucket.OutputComplete = false
	}
	if row.CostSource == "unpriced" {
		bucket.UnpricedCount++
	} else {
		total, ok := addInt64(bucket.costSubcents, row.CostSubcents)
		if ok {
			bucket.costSubcents = total
			bucket.costSeen = true
		} else {
			bucket.costOverflow = true
		}
	}
}

func addTokenCount(bucket *UsageTokenBreakdownDTO, total *int64, value int64) {
	if sum, ok := addInt64(*total, value); ok {
		*total = sum
		return
	}
	bucket.Overflow = true
	if value > 0 {
		*total = maxInt64
	} else {
		*total = minInt64
	}
}

func finalizeBucket(bucket *UsageTokenBreakdownDTO) {
	if bucket.EventCount == 0 || bucket.UnpricedCount > 0 || bucket.costOverflow || !bucket.costSeen {
		return
	}
	cost := strconv.FormatInt(bucket.costSubcents, 10)
	bucket.CostSubcents = &cost
}

func ToUsageResponseDTO(row *models.TaskUsageEvent) UsageResponseDTO {
	response := UsageResponseDTO{
		UsageEventID: row.UsageEventID, ProviderResponseID: row.ProviderResponseID,
		ProviderThreadID: row.ProviderThreadID, ProviderTurnID: row.ProviderTurnID,
		Scope: row.NativeScope, Source: row.MeasurementSource, Completeness: row.UsageCompleteness,
		Model: row.Model, Provider: row.Provider, InputTokens: row.TokensIn,
		CachedReadTokens: row.TokensCachedRead, CachedWriteTokens: row.TokensCachedWrite,
		OutputTokens: row.TokensOut, ThoughtTokens: row.TokensThought,
		ReasoningOutputTokens: row.ReasoningOutputTokens, ReportedCacheWriteTokens: row.ReportedCacheWriteTokens,
		ReportedTotalTokens: row.ReportedTotalTokens, TotalTokens: row.TokensTotal,
		CostSource: row.CostSource, Estimated: row.Estimated, OccurredAt: row.OccurredAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
	if row.CostSource != "unpriced" {
		cost := strconv.FormatInt(row.CostSubcents, 10)
		response.CostSubcents = &cost
	}
	return response
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func addInt64(a, b int64) (int64, bool) {
	if b > 0 && a > maxInt64-b || b < 0 && a < minInt64-b {
		return 0, false
	}
	return a + b, true
}

const maxInt64 = int64(^uint64(0) >> 1)
const minInt64 = -maxInt64 - 1
