package usage

import (
	"context"
	"math"
	"testing"
	"testing/synctest"
	"time"

	commoncosts "github.com/kandev/kandev/internal/common/costs"
	"github.com/kandev/kandev/internal/task/models"
)

var fixedOccurredAt = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

// TestBuildRow_TokensTotal_SumsAllFiveClasses pins AC-23: tokens_total is
// computed by the writer, never copied from the payload's own total_tokens.
func TestBuildRow_TokensTotal_SumsAllFiveClasses(t *testing.T) {
	w := &Writer{}
	p := &usageEventPayload{
		UsageEventID: "evt-1", TaskID: "task-1",
		Usage: &promptUsagePayload{
			InputTokens: 100, OutputTokens: 30, OutputTokensPresent: true,
			CachedReadTokens: 25, CachedWriteTokens: 5, ThoughtTokens: 10,
			TotalTokens: 999, // deliberately wrong, to prove it's ignored
		},
	}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)
	if event.TokensTotal != 170 {
		t.Errorf("TokensTotal = %d, want 170 (100+30+25+5+10), payload total_tokens must be ignored", event.TokensTotal)
	}
}

func TestBuildRow_TokensTotalOverflow_ReturnsNil(t *testing.T) {
	w := &Writer{}
	p := &usageEventPayload{
		UsageEventID: "evt-overflow", TaskID: "task-1",
		Usage: &promptUsagePayload{InputTokens: math.MaxInt64, CachedReadTokens: 1},
	}
	if event := w.buildRow(context.Background(), p, fixedOccurredAt); event != nil {
		t.Fatalf("buildRow returned %+v, want nil for an overflowing token total", event)
	}
}

// TestBuildRow_OutputTokensNotPresent_LeavesTokensOutNil pins AC-4/AC-30:
// an unsampled output-token count stores as NULL (nil), not zero, and
// contributes zero to tokens_total either way.
func TestBuildRow_OutputTokensNotPresent_LeavesTokensOutNil(t *testing.T) {
	w := &Writer{}
	p := &usageEventPayload{
		UsageEventID: "evt-1", TaskID: "task-1",
		Usage: &promptUsagePayload{InputTokens: 50, OutputTokensPresent: false},
	}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)
	if event.TokensOut != nil {
		t.Errorf("TokensOut = %v, want nil", event.TokensOut)
	}
	if event.TokensTotal != 50 {
		t.Errorf("TokensTotal = %d, want 50", event.TokensTotal)
	}
}

// TestBuildRow_OccurredAtAndCreatedAt_EqualTheSuppliedInstant pins that
// both timestamps are stamped once, from the caller-supplied occurredAt,
// and are equal on a first write.
func TestBuildRow_OccurredAtAndCreatedAt_EqualTheSuppliedInstant(t *testing.T) {
	w := &Writer{}
	p := &usageEventPayload{UsageEventID: "evt-1", TaskID: "task-1", Usage: &promptUsagePayload{}}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)
	if !event.OccurredAt.Equal(fixedOccurredAt) || !event.CreatedAt.Equal(fixedOccurredAt) {
		t.Errorf("OccurredAt=%v CreatedAt=%v, want both %v", event.OccurredAt, event.CreatedAt, fixedOccurredAt)
	}
}

// TestBuildRow_ContractVersion_IsOne pins AC-5.
func TestBuildRow_ContractVersion_IsOne(t *testing.T) {
	w := &Writer{}
	p := &usageEventPayload{UsageEventID: "evt-1", TaskID: "task-1", Usage: &promptUsagePayload{}}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)
	if event.ContractVersion != 1 {
		t.Errorf("ContractVersion = %d, want 1", event.ContractVersion)
	}
}

// TestBuildRow_CarriesModelAgentTypeAgentProfileIDVerbatim pins AC-35.
func TestBuildRow_CarriesModelAgentTypeAgentProfileIDVerbatim(t *testing.T) {
	w := &Writer{}
	p := &usageEventPayload{
		UsageEventID: "evt-1", TaskID: "task-1",
		Model: "claude-sonnet-5", AgentType: "claude-acp", AgentProfileID: "profile-1",
		Usage: &promptUsagePayload{},
	}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)
	if event.Model != "claude-sonnet-5" || event.AgentType != "claude-acp" || event.AgentProfileID != "profile-1" {
		t.Errorf("event = %+v, fields did not carry through verbatim", event)
	}
}

// TestBuildRow_MissingModelAgentTypeAgentProfileID_DefaultToEmptyString
// pins AC-35's "default to ” only when the payload's own field is empty"
// rule.
func TestBuildRow_MissingModelAgentTypeAgentProfileID_DefaultToEmptyString(t *testing.T) {
	w := &Writer{}
	p := &usageEventPayload{UsageEventID: "evt-1", TaskID: "task-1", Usage: &promptUsagePayload{}}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)
	if event.Model != "" || event.AgentType != "" || event.AgentProfileID != "" {
		t.Errorf("event = %+v, want empty-string defaults", event)
	}
}

// TestResolveProvider pins AC-31's fixed fallback order.
func TestResolveProvider(t *testing.T) {
	tests := []struct {
		name                      string
		agentType, agentID, model string
		want                      string
	}{
		{"agent_type CLI mapping wins first", "claude-acp", "", "gpt-4", "anthropic"},
		{"agent_id CLI mapping used when agent_type doesn't map", "unknown-cli", "codex-acp", "claude-sonnet-5", "openai"},
		{"model prefix used when neither CLI id maps", "unknown-cli", "unknown-cli", "gemini-1.5", "google"},
		{"empty string when nothing matches", "unknown-cli", "unknown-cli", "unknown-model", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveProvider(tt.agentType, tt.agentID, tt.model); got != tt.want {
				t.Errorf("resolveProvider(%q,%q,%q) = %q, want %q", tt.agentType, tt.agentID, tt.model, got, tt.want)
			}
		})
	}
}

// fakePricingLookup is a test double for PricingLookup. If block is true,
// LookupForModelWithVersion blocks until ctx is done and then reports not
// ok, modeling a pricing backend that never itself respects a deadline
// shorter than the writer's own (AC-8's "never returns" scenario).
type fakePricingLookup struct {
	pricing commoncosts.ModelPricing
	version string
	ok      bool
	block   bool
	calls   int
}

func (f *fakePricingLookup) LookupForModelWithVersion(ctx context.Context, _ string) (commoncosts.ModelPricing, string, bool) {
	f.calls++
	if f.block {
		<-ctx.Done()
		return commoncosts.ModelPricing{}, "", false
	}
	return f.pricing, f.version, f.ok
}

// TestResolveCost_ProviderReportedCostPresent_RecordsVerbatimSkipsPricing
// pins AC-6, including the explicit-zero case.
func TestResolveCost_ProviderReportedCostPresent_RecordsVerbatimSkipsPricing(t *testing.T) {
	pricing := &fakePricingLookup{ok: true, pricing: commoncosts.ModelPricing{InputPerMillion: 300}}
	w := &Writer{pricing: pricing}
	p := &usageEventPayload{
		UsageEventID: "evt-1", TaskID: "task-1",
		Usage: &promptUsagePayload{ProviderReportedCostPresent: true, ProviderReportedCostSubcents: 0},
	}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)
	if event.CostSource != CostSourceProviderReported {
		t.Errorf("CostSource = %q, want %q", event.CostSource, CostSourceProviderReported)
	}
	if event.CostSubcents != 0 {
		t.Errorf("CostSubcents = %d, want 0 (explicit provider-reported zero must be recorded, not treated as absent)", event.CostSubcents)
	}
	if pricing.calls != 0 {
		t.Errorf("pricing lookup was called %d times, want 0 (provider-reported cost must skip pricing entirely)", pricing.calls)
	}
}

// TestResolveCost_PricingResolves_ComputesCostAndPopulatesRates pins AC-7,
// AC-9, AC-26: a single lookup call populates cost_subcents, cost_source,
// all four rate columns, and the pricing catalogue version together.
func TestResolveCost_PricingResolves_ComputesCostAndPopulatesRates(t *testing.T) {
	pricing := &fakePricingLookup{
		ok: true, version: "catalogue-v7",
		pricing: commoncosts.ModelPricing{InputPerMillion: 3_000_000, CachedReadPerMillion: 300_000, CachedWritePerMillion: 3_750_000, OutputPerMillion: 15_000_000},
	}
	w := &Writer{pricing: pricing}
	p := &usageEventPayload{
		UsageEventID: "evt-1", TaskID: "task-1",
		Usage: &promptUsagePayload{
			InputTokens: 100, OutputTokens: 30, OutputTokensPresent: true,
			CachedReadTokens: 25, CachedWriteTokens: 5, Estimated: true,
		},
	}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)

	if event.CostSource != CostSourceModelsDevList {
		t.Errorf("CostSource = %q, want %q", event.CostSource, CostSourceModelsDevList)
	}
	wantCost := (int64(100)*3_000_000 + 25*300_000 + 5*3_750_000 + 30*15_000_000) / 1_000_000
	if event.CostSubcents != wantCost {
		t.Errorf("CostSubcents = %d, want %d", event.CostSubcents, wantCost)
	}
	if event.RateInputPerMillion == nil || *event.RateInputPerMillion != 3_000_000 {
		t.Errorf("RateInputPerMillion = %v, want 3000000", event.RateInputPerMillion)
	}
	if event.RateCachedReadPerMillion == nil || *event.RateCachedReadPerMillion != 300_000 {
		t.Errorf("RateCachedReadPerMillion = %v, want 300000", event.RateCachedReadPerMillion)
	}
	if event.RateCachedWritePerMillion == nil || *event.RateCachedWritePerMillion != 3_750_000 {
		t.Errorf("RateCachedWritePerMillion = %v, want 3750000", event.RateCachedWritePerMillion)
	}
	if event.RateOutputPerMillion == nil || *event.RateOutputPerMillion != 15_000_000 {
		t.Errorf("RateOutputPerMillion = %v, want 15000000", event.RateOutputPerMillion)
	}
	if event.PricingCatalogVersion != "catalogue-v7" {
		t.Errorf("PricingCatalogVersion = %q, want %q", event.PricingCatalogVersion, "catalogue-v7")
	}
	if !event.Estimated {
		t.Error("Estimated = false, want true (AC-9: recorded independently of cost_source, straight from the payload)")
	}
	if pricing.calls != 1 {
		t.Errorf("pricing lookup was called %d times, want exactly 1 (a single call must supply both rates and version)", pricing.calls)
	}
}

// TestResolveCost_PricingNotOK_FallsBackToUnpriced pins AC-8: a pricing miss
// still records the row, with cost_subcents=0 and no rate columns.
func TestResolveCost_PricingNotOK_FallsBackToUnpriced(t *testing.T) {
	pricing := &fakePricingLookup{ok: false}
	w := &Writer{pricing: pricing}
	p := &usageEventPayload{UsageEventID: "evt-1", TaskID: "task-1", Usage: &promptUsagePayload{InputTokens: 10}}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)
	if event.CostSource != CostSourceUnpriced {
		t.Errorf("CostSource = %q, want %q", event.CostSource, CostSourceUnpriced)
	}
	if event.CostSubcents != 0 {
		t.Errorf("CostSubcents = %d, want 0", event.CostSubcents)
	}
	if event.RateInputPerMillion != nil {
		t.Errorf("RateInputPerMillion = %v, want nil", event.RateInputPerMillion)
	}
}

func TestResolveCost_OverflowFallsBackToUnpriced(t *testing.T) {
	pricing := &fakePricingLookup{
		ok:      true,
		pricing: commoncosts.ModelPricing{InputPerMillion: math.MaxInt64},
	}
	w := &Writer{pricing: pricing}
	p := &usageEventPayload{
		UsageEventID: "evt-cost-overflow", TaskID: "task-1",
		Usage: &promptUsagePayload{InputTokens: 2},
	}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)
	if event == nil {
		t.Fatal("buildRow returned nil for a cost overflow; token total still fits")
	}
	if event.CostSource != CostSourceUnpriced || event.CostSubcents != 0 {
		t.Fatalf("overflowing cost = (%q, %d), want (unpriced, 0)", event.CostSource, event.CostSubcents)
	}
}

// TestResolveCost_NilPricingLookup_FallsBackToUnpriced pins AC-8's
// no-lookup-wired degradation (relevant before Task 12 wires production
// pricing).
func TestResolveCost_NilPricingLookup_FallsBackToUnpriced(t *testing.T) {
	w := &Writer{pricing: nil}
	p := &usageEventPayload{UsageEventID: "evt-1", TaskID: "task-1", Usage: &promptUsagePayload{InputTokens: 10}}
	event := w.buildRow(context.Background(), p, fixedOccurredAt)
	if event.CostSource != CostSourceUnpriced {
		t.Errorf("CostSource = %q, want %q", event.CostSource, CostSourceUnpriced)
	}
}

// TestResolveCost_PricingNeverReturns_HonorsWriterOwnedDeadline pins AC-8's
// 2-second writer-owned deadline: a pricing lookup that only respects
// context cancellation must not stall buildRow past pricingLookupTimeout.
// Runs inside synctest so the deadline's real time.Timer advances on the
// fake clock instead of costing real wall time.
func TestResolveCost_PricingNeverReturns_HonorsWriterOwnedDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pricing := &fakePricingLookup{block: true}
		w := &Writer{pricing: pricing}
		p := &usageEventPayload{UsageEventID: "evt-1", TaskID: "task-1", Usage: &promptUsagePayload{InputTokens: 10}}

		done := make(chan *models.TaskUsageEvent, 1)
		go func() {
			done <- w.buildRow(context.Background(), p, fixedOccurredAt)
		}()

		event := <-done
		if event.CostSource != CostSourceUnpriced {
			t.Errorf("CostSource = %q, want %q", event.CostSource, CostSourceUnpriced)
		}
	})
}
