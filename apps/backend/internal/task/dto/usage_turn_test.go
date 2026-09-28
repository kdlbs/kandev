package dto

import (
	"math"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestToUsageTurnDTOSeparatesDirectChildAndReasoningSubset(t *testing.T) {
	cached := int64(600)
	output := int64(200)
	reasoning := int64(80)
	rows := models.TaskUsageTurnEvents{TurnID: "turn-1", Events: []*models.TaskUsageEvent{
		{
			UsageEventID: "evt-root", NativeScope: "direct", MeasurementSource: "response", UsageCompleteness: "exact",
			ProviderResponseID: "response-root", TokensIn: 400, TokensCachedRead: &cached, TokensOut: &output,
			ReasoningOutputTokens: &reasoning, TokensTotal: 1200, CostSource: "unpriced", OccurredAt: time.Unix(1, 0),
		},
		{
			UsageEventID: "evt-child", NativeScope: "child", MeasurementSource: "response", UsageCompleteness: "exact",
			TokensIn: 10, TokensTotal: 10, TokensOut: &output, CostSubcents: 25, CostSource: "models_dev_list", OccurredAt: time.Unix(2, 0),
		},
	}}
	got := ToUsageTurnDTO(rows, true)
	if got.TurnID != "turn-1" || got.Completeness != "exact" || got.Direct.TotalTokens != 1200 || got.Child.TotalTokens != 10 || got.Total.TotalTokens != 1210 {
		t.Fatalf("usage turn totals = %#v, want direct 1200, child 10, total 1210", got)
	}
	if got.Direct.ThoughtTokens != 0 || got.Responses[0].ReasoningOutputTokens == nil || *got.Responses[0].ReasoningOutputTokens != 80 {
		t.Fatalf("reasoning subset = %#v, want separate detail without changing thought total", got)
	}
	if got.Total.CostSubcents != nil || got.Total.UnpricedCount != 1 {
		t.Fatalf("mixed priced/unpriced total = %#v, want unavailable total price", got.Total)
	}
	if got.LastResponse == nil || got.LastResponse.UsageEventID != "evt-child" {
		t.Fatalf("last response = %#v, want the latest observed event", got.LastResponse)
	}
}

func TestToUsageTurnDTOMarksTokenAggregateOverflow(t *testing.T) {
	got := ToUsageTurnDTO(models.TaskUsageTurnEvents{TurnID: "turn-overflow", Events: []*models.TaskUsageEvent{
		{UsageEventID: "evt-one", NativeScope: "direct", TokensIn: math.MaxInt64, TokensOut: ptrInt64(0), TokensTotal: math.MaxInt64},
		{UsageEventID: "evt-two", NativeScope: "direct", TokensIn: 1, TokensOut: ptrInt64(0), TokensTotal: 1},
	}}, false)
	if got.Completeness != "partial" || !got.Direct.Overflow || !got.Total.Overflow || got.Direct.InputTokens != math.MaxInt64 {
		t.Fatalf("overflow turn = %#v, want partial saturated totals", got)
	}
}

func ptrInt64(value int64) *int64 { return &value }

func TestToUsageTurnDTOReportsZeroCostAndNoResponseRows(t *testing.T) {
	zero := int64(0)
	got := ToUsageTurnDTO(models.TaskUsageTurnEvents{TurnID: "turn-zero", Events: []*models.TaskUsageEvent{{
		UsageEventID: "evt-zero", NativeScope: "direct", MeasurementSource: "response", UsageCompleteness: "exact",
		TokensOut: &zero, CostSource: "provider_reported", CostSubcents: 0,
	}}}, false)
	if got.Total.CostSubcents == nil || *got.Total.CostSubcents != "0" {
		t.Fatalf("zero cost = %v, want a present zero", got.Total.CostSubcents)
	}
	if len(got.Responses) != 0 || got.LastResponse == nil {
		t.Fatalf("response details = %#v and last response %#v, want summary-only with last response", got.Responses, got.LastResponse)
	}
}
