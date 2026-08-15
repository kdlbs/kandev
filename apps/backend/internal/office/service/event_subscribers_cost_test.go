package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// fakePricingLookup is a minimal shared.PricingLookup test double. hit
// controls whether LookupForModel reports a hit; version (if non-empty)
// makes the fake also satisfy shared.PricingCatalogVersioner, so tests can
// exercise both the with- and without-versioner cases.
type fakePricingLookup struct {
	hit     bool
	pricing shared.ModelPricing
	version string
}

func (f *fakePricingLookup) LookupForModel(_ context.Context, _ string) (shared.ModelPricing, bool) {
	if !f.hit {
		return shared.ModelPricing{}, false
	}
	return f.pricing, true
}

func (f *fakePricingLookup) CatalogVersion() string { return f.version }

// TestPromptUsage_CacheSplitRecordedWhenNotEstimated confirms the cache
// read/write split reaches storage intact — the P1 defect was that
// tokens_cached_in = read + write was computed and then the split was
// thrown away one line before the INSERT, even though CalculateCostSubcents
// already prices read and write at distinct per-million rates.
func TestPromptUsage_CacheSplitRecordedWhenNotEstimated(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-split")
	insertTestTask(t, svc, "task-split", "ws-1")
	setTestTaskAssignee(t, svc, "task-split", "worker-split")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-split",
		"session_id": "session-split",
		"agent_id":   "claude-acp",
		"model":      "sonnet",
		"usage": map[string]interface{}{
			"input_tokens":        100,
			"cached_read_tokens":  40,
			"cached_write_tokens": 60,
			"output_tokens":       10,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-split"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.TokensCachedRead == nil || *row.TokensCachedRead != 40 {
		t.Errorf("TokensCachedRead = %v, want 40", row.TokensCachedRead)
	}
	if row.TokensCachedWrite == nil || *row.TokensCachedWrite != 60 {
		t.Errorf("TokensCachedWrite = %v, want 60", row.TokensCachedWrite)
	}
	// tokens_cached_in keeps its original sum semantics for existing
	// consumers (e.g. the tree-holds rollup and card 2faa29da).
	if row.TokensCachedIn != 100 {
		t.Errorf("TokensCachedIn = %d, want 100 (read+write, unchanged semantics)", row.TokensCachedIn)
	}
}

// TestPromptUsage_CacheSplitNullWhenEstimated confirms the codex/no-usage-
// frame caveat: when Usage.Estimated is true (context-occupancy synthesis,
// adapter_prompt.go's fallback), the split columns must be NULL, never 0 —
// a zero would falsely claim "no cache activity" for an adapter that never
// reported cache tokens at all. tokens_cached_in still equals read+write
// (both zero here), preserving the existing column's semantics.
func TestPromptUsage_CacheSplitNullWhenEstimated(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-codex-null")
	insertTestTask(t, svc, "task-codex-null", "ws-1")
	setTestTaskAssignee(t, svc, "task-codex-null", "worker-codex-null")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-codex-null",
		"session_id": "session-codex-null",
		"agent_id":   "codex-acp",
		"model":      "gpt-5.4-mini",
		"usage": map[string]interface{}{
			"input_tokens": 350,
			"estimated":    true,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-codex-null"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.TokensCachedRead != nil {
		t.Errorf("TokensCachedRead = %v, want nil (NULL, never 0)", *row.TokensCachedRead)
	}
	if row.TokensCachedWrite != nil {
		t.Errorf("TokensCachedWrite = %v, want nil (NULL, never 0)", *row.TokensCachedWrite)
	}
}

// TestPromptUsage_CostSourceProviderReported confirms Layer A rows are
// tagged provider_reported with no rates recorded (rates only apply to a
// models.dev list-price calculation).
func TestPromptUsage_CostSourceProviderReported(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-src-a")
	insertTestTask(t, svc, "task-src-a", "ws-1")
	setTestTaskAssignee(t, svc, "task-src-a", "worker-src-a")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-src-a",
		"session_id": "session-src-a",
		"agent_id":   "claude-acp",
		"model":      "sonnet",
		"usage": map[string]interface{}{
			"input_tokens":                    100,
			"output_tokens":                   200,
			"provider_reported_cost_subcents": 616,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-src-a"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.CostSource == nil || *row.CostSource != models.CostSourceProviderReported {
		t.Fatalf("CostSource = %v, want %q", row.CostSource, models.CostSourceProviderReported)
	}
	if row.RateInputPerMillion != nil {
		t.Errorf("RateInputPerMillion = %v, want nil on the provider-reported path", *row.RateInputPerMillion)
	}
	if row.PricingCatalogVersion != nil {
		t.Errorf("PricingCatalogVersion = %v, want nil on the provider-reported path", *row.PricingCatalogVersion)
	}
}

func TestPromptUsage_ExplicitZeroProviderCostSkipsPricing(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetPricingLookup(&fakePricingLookup{
		hit:     true,
		pricing: shared.ModelPricing{InputPerMillion: 999999, OutputPerMillion: 999999},
	})

	createTestAgent(t, svc, "ws-1", "worker-zero-provider")
	insertTestTask(t, svc, "task-zero-provider", "ws-1")
	setTestTaskAssignee(t, svc, "task-zero-provider", "worker-zero-provider")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-zero-provider",
		"session_id": "session-zero-provider",
		"agent_id":   "claude-acp",
		"model":      "sonnet",
		"usage": map[string]interface{}{
			"input_tokens":                    100,
			"output_tokens":                   200,
			"provider_reported_cost_subcents": 0,
			"provider_reported_cost_present":  true,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-zero-provider"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.CostSubcents != 0 {
		t.Fatalf("cost_subcents = %d, want 0", row.CostSubcents)
	}
	if row.CostSource == nil || *row.CostSource != models.CostSourceProviderReported {
		t.Fatalf("CostSource = %v, want %q", row.CostSource, models.CostSourceProviderReported)
	}
}

// TestPromptUsage_CostSourceModelsDevList confirms Layer B rows are tagged
// models_dev_list with the four applied rates and the catalogue version
// recorded — the whole point of the provenance field: today `estimated`
// cannot distinguish "priced from a provider amount" from "priced from a
// list-price calculation", which downstream read as "metered" for 671 of
// 673 events until corrected on the dashboard side.
func TestPromptUsage_CostSourceModelsDevList(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetPricingLookup(&fakePricingLookup{
		hit: true,
		pricing: shared.ModelPricing{
			InputPerMillion:       300000,
			CachedReadPerMillion:  30000,
			CachedWritePerMillion: 375000,
			OutputPerMillion:      1500000,
		},
		version: "2026-08-12T00:00:00Z",
	})

	createTestAgent(t, svc, "ws-1", "worker-src-b")
	insertTestTask(t, svc, "task-src-b", "ws-1")
	setTestTaskAssignee(t, svc, "task-src-b", "worker-src-b")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-src-b",
		"session_id": "session-src-b",
		"model":      "claude-sonnet-4-5",
		"usage": map[string]interface{}{
			"input_tokens":  1_000_000,
			"output_tokens": 1_000_000,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-src-b"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.CostSource == nil || *row.CostSource != models.CostSourceModelsDevList {
		t.Fatalf("CostSource = %v, want %q", row.CostSource, models.CostSourceModelsDevList)
	}
	if row.RateInputPerMillion == nil || *row.RateInputPerMillion != 300000 {
		t.Errorf("RateInputPerMillion = %v, want 300000", row.RateInputPerMillion)
	}
	if row.RateOutputPerMillion == nil || *row.RateOutputPerMillion != 1500000 {
		t.Errorf("RateOutputPerMillion = %v, want 1500000", row.RateOutputPerMillion)
	}
	if row.PricingCatalogVersion == nil || *row.PricingCatalogVersion != "2026-08-12T00:00:00Z" {
		t.Errorf("PricingCatalogVersion = %v, want the fake's version", row.PricingCatalogVersion)
	}
	if row.CostContractVersion == nil || *row.CostContractVersion != 2 {
		t.Errorf("CostContractVersion = %v, want 2 (in-band activation point)", row.CostContractVersion)
	}
}

// TestPromptUsage_CostSourceUnpriced confirms the "both layers miss" case
// (no provider-reported cost, no pricing lookup wired) is tagged unpriced,
// not silently left as a bare estimated=true with no source at all. It also
// covers the R2-F4 regression: usage carrying authoritative (non-synthesised)
// token counts must keep Estimated=false even though the row is unpriced —
// cost_source=unpriced alone carries "we could not resolve a price";
// Estimated is strictly "the token counts were synthesised" (see
// costContractVersion's v1→v2 doc comment in prompt_usage_cost.go). Before
// that fix this case incorrectly forced Estimated=true.
func TestPromptUsage_CostSourceUnpriced(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-src-c")
	insertTestTask(t, svc, "task-src-c", "ws-1")
	setTestTaskAssignee(t, svc, "task-src-c", "worker-src-c")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-src-c",
		"session_id": "session-src-c",
		"model":      "butler_a",
		"usage": map[string]interface{}{
			"input_tokens":  100,
			"output_tokens": 200,
			"estimated":     false,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-src-c"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.CostSource == nil || *row.CostSource != models.CostSourceUnpriced {
		t.Fatalf("CostSource = %v, want %q", row.CostSource, models.CostSourceUnpriced)
	}
	if row.Estimated {
		t.Error("Estimated = true, want false: unpriced must not overwrite the adapter's own token-synthesis flag")
	}
}

// TestPromptUsage_TurnIDRecorded confirms turn_id threads all the way from
// the bus payload to the stored row when present.
func TestPromptUsage_TurnIDRecorded(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-turn")
	insertTestTask(t, svc, "task-turn", "ws-1")
	setTestTaskAssignee(t, svc, "task-turn", "worker-turn")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":        "task-turn",
		"session_id":     "session-turn",
		"model":          "sonnet",
		"turn_id":        "turn-abc-123",
		"usage_event_id": "usage-evt-abc-123",
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 5,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-turn"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.TurnID == nil || *row.TurnID != "turn-abc-123" {
		t.Errorf("TurnID = %v, want turn-abc-123", row.TurnID)
	}
	if row.UsageEventID == nil || *row.UsageEventID != "usage-evt-abc-123" {
		t.Errorf("UsageEventID = %v, want usage-evt-abc-123", row.UsageEventID)
	}
}

// TestPromptUsage_DuplicateUsageEventIDIsIdempotent confirms redelivery of
// the same prompt-usage event (identical usage_event_id — e.g. an
// at-least-once event bus retry) does not double-record cost.
func TestPromptUsage_DuplicateUsageEventIDIsIdempotent(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-idem")
	insertTestTask(t, svc, "task-idem", "ws-1")
	setTestTaskAssignee(t, svc, "task-idem", "worker-idem")

	makeEvent := func() *bus.Event {
		return bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
			"task_id":        "task-idem",
			"session_id":     "session-idem",
			"model":          "sonnet",
			"usage_event_id": "usage-evt-dup-1",
			"usage": map[string]interface{}{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		})
	}

	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-idem"), makeEvent()); err != nil {
		t.Fatalf("publish first: %v", err)
	}
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-idem"), makeEvent()); err != nil {
		t.Fatalf("publish redelivery: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost count = %d, want 1 (redelivery must not double-record)", len(costs))
	}
}
