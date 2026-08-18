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

// TestPromptUsage_ThoughtTokensDoNotAffectCost is a regression test for the
// triage correction on the card: reasoning_output_tokens is a SUBSET of
// output_tokens (OpenAI's own accounting: last_total = last_in + last_out
// held across all 22 Tetris-benchmark rows even though reasoning was
// nonzero), not an addend. Folding ThoughtTokens into billable output would
// have double-counted and inflated that turn's cost by ~22%. Pins the cost
// for an identical usage sample with and without ThoughtTokens set.
func TestPromptUsage_ThoughtTokensDoNotAffectCost(t *testing.T) {
	pricing := &fakePricingLookup{
		hit: true,
		pricing: shared.ModelPricing{
			InputPerMillion:  300000,
			OutputPerMillion: 1500000,
		},
	}

	costFor := func(taskID, sessionID string, thoughtTokens int) int64 {
		svc, eb := newTestServiceWithBus(t)
		svc.SetPricingLookup(pricing)
		ctx := context.Background()
		createTestAgent(t, svc, "ws-1", "worker-thought")
		insertTestTask(t, svc, taskID, "ws-1")
		setTestTaskAssignee(t, svc, taskID, "worker-thought")

		usage := map[string]interface{}{
			"input_tokens":  37616,
			"output_tokens": 410,
		}
		if thoughtTokens > 0 {
			usage["thought_tokens"] = thoughtTokens
		}
		event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
			"task_id":    taskID,
			"session_id": sessionID,
			"model":      "gpt-5.6-terra",
			"usage":      usage,
		})
		if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject(sessionID), event); err != nil {
			t.Fatalf("publish: %v", err)
		}
		costs, err := svc.ListCostEvents(ctx, "ws-1")
		if err != nil || len(costs) != 1 {
			t.Fatalf("list costs: %v (len=%d)", err, len(costs))
		}
		return costs[0].CostSubcents
	}

	withoutReasoning := costFor("task-thought-a", "session-thought-a", 0)
	withReasoning := costFor("task-thought-b", "session-thought-b", 238)
	if withoutReasoning != withReasoning {
		t.Fatalf(
			"cost changed with ThoughtTokens present: without=%d with=%d, want equal (reasoning tokens are a subset of output, not billable separately)",
			withoutReasoning, withReasoning,
		)
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

// TestPromptUsage_FallsBackToSessionAgentProfileWhenUnassigned is a
// regression test for the codex cost-attribution gap: a task on a Kanban
// step with no pinned runner (or, in general, no workflow_step_participants
// 'runner' row) resolves RunnerProjection to "", so every cost event for it
// attributed to no agent profile at all — measured at 421/639 opus and
// 186/640 sonnet cost events store-wide. The session that actually ran the
// turn still knows: task_sessions.agent_profile_id is populated. buildCostEvent
// should fall back to it only when the workflow projection is blank.
func TestPromptUsage_FallsBackToSessionAgentProfileWhenUnassigned(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "codex-runner")
	insertTestTask(t, svc, "task-no-runner", "ws-1")
	// Deliberately no setTestTaskAssignee call: the workflow step has no
	// pinned runner, so RunnerProjection resolves to "".
	insertTestTaskSession(t, svc, "session-no-runner", "task-no-runner", "codex-runner")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-no-runner",
		"session_id": "session-no-runner",
		"model":      "gpt-5.6-terra",
		"usage": map[string]interface{}{
			"input_tokens":  100,
			"output_tokens": 10,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-no-runner"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	if got := costs[0].AgentProfileID; got != "codex-runner" {
		t.Errorf("AgentProfileID = %q, want %q (session fallback)", got, "codex-runner")
	}
}

// TestPromptUsage_WorkflowRunnerWinsOverSessionAgentProfile confirms the
// session fallback strictly fills blanks: when RunnerProjection already
// resolves an assignee, the session's own agent_profile_id (which could
// differ, e.g. after a mid-task agent swap) must never override it.
func TestPromptUsage_WorkflowRunnerWinsOverSessionAgentProfile(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "workflow-runner")
	createTestAgent(t, svc, "ws-1", "session-runner")
	insertTestTask(t, svc, "task-both-runners", "ws-1")
	setTestTaskAssignee(t, svc, "task-both-runners", "workflow-runner")
	insertTestTaskSession(t, svc, "session-both-runners", "task-both-runners", "session-runner")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-both-runners",
		"session_id": "session-both-runners",
		"model":      "sonnet",
		"usage": map[string]interface{}{
			"input_tokens":  100,
			"output_tokens": 10,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-both-runners"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	if got := costs[0].AgentProfileID; got != "workflow-runner" {
		t.Errorf("AgentProfileID = %q, want %q (workflow projection must win)", got, "workflow-runner")
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
