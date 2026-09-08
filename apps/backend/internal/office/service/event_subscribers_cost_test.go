package service_test

import (
	"context"
	"math"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
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

// TestPromptUsage_CacheSplitNullWhenEstimated confirms the context-occupancy
// fallback caveat (adapter_prompt.go's fallbackUsageForNilTypedUsage, used
// when an adapter emits no typed usage frame at all): when the usage sample
// carries no cache counts at all (both fields absent/zero, as that fallback
// always produces), the split columns must be NULL, never 0 — a zero would
// falsely claim "no cache activity" for an adapter that never reported cache
// tokens at all. The gate is cache-data availability, not Usage.Estimated
// (see TestPromptUsage_CacheSplitPersistedWhenEstimatedWithCacheData for the
// codex case: Estimated=true but real cache numbers present). tokens_cached_in
// still equals read+write (both zero here), preserving the existing column's
// semantics.
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

// TestPromptUsage_CacheSplitPersistedWhenEstimatedWithCacheData is a
// regression test for a Review round-1 finding: gating the cache split on
// Usage.Estimated (rather than on whether cache data is actually present)
// silently dropped codex's real cache numbers, because codex-acp's typed
// per-request usage frame sets Estimated=true (it is scoped to the last
// model request of the turn, not the whole turn) while still reporting
// genuine cachedReadTokens/cachedWriteTokens — a live capture showed
// cachedReadTokens=22272 on exactly such a frame. The split must survive
// even though the row is (correctly) marked estimated.
func TestPromptUsage_CacheSplitPersistedWhenEstimatedWithCacheData(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-codex-cache")
	insertTestTask(t, svc, "task-codex-cache", "ws-1")
	setTestTaskAssignee(t, svc, "task-codex-cache", "worker-codex-cache")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-codex-cache",
		"session_id": "session-codex-cache",
		"agent_id":   "codex-acp",
		"model":      "gpt-5.6-terra",
		"usage": map[string]interface{}{
			"input_tokens":        377,
			"cached_read_tokens":  22272,
			"cached_write_tokens": 0,
			"output_tokens":       9,
			"estimated":           true,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-codex-cache"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if !row.Estimated {
		t.Fatalf("Estimated = false, want true (codex's typed frame is per-request, not per-turn)")
	}
	if row.TokensCachedRead == nil || *row.TokensCachedRead != 22272 {
		t.Errorf("TokensCachedRead = %v, want 22272 (real cache data must survive Estimated=true)", row.TokensCachedRead)
	}
	if row.TokensCachedWrite == nil || *row.TokensCachedWrite != 0 {
		t.Errorf("TokensCachedWrite = %v, want 0 (explicit, not nil)", row.TokensCachedWrite)
	}
	if row.TokensCachedIn != 22272 {
		t.Errorf("TokensCachedIn = %d, want 22272 (read+write, unchanged semantics)", row.TokensCachedIn)
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
	if row.CostContractVersion == nil || *row.CostContractVersion != 3 {
		t.Errorf("CostContractVersion = %v, want 3 (in-band activation point)", row.CostContractVersion)
	}
}

func TestPromptUsage_PricingOverflowFallsBackToUnpriced(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetPricingLookup(&fakePricingLookup{
		hit:     true,
		pricing: shared.ModelPricing{InputPerMillion: 3},
	})

	createTestAgent(t, svc, "ws-1", "worker-overflow")
	insertTestTask(t, svc, "task-overflow", "ws-1")
	setTestTaskAssignee(t, svc, "task-overflow", "worker-overflow")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-overflow",
		"session_id": "session-overflow",
		"agent_id":   "claude-acp",
		"model":      "sonnet",
		"usage": map[string]interface{}{
			"input_tokens": math.MaxInt64/2 + 1,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-overflow"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.CostSource == nil || *row.CostSource != models.CostSourceUnpriced || row.CostSubcents != 0 {
		t.Fatalf("cost = (%v, %d), want (unpriced, 0)", row.CostSource, row.CostSubcents)
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
// Estimated remains independent from pricing resolution (see the
// costContractVersion history in prompt_usage_cost.go). Before v2 this case
// incorrectly forced Estimated=true.
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
		t.Error("Estimated = true, want false: unpriced must not overwrite the adapter's usage-authority flag")
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

// TestPromptUsage_SessionAgentProfileWinsOverWorkflowRunner protects the
// immutable attribution boundary: the session's profile owns the usage event,
// even when RunnerProjection now points at a different workflow runner.
func TestPromptUsage_SessionAgentProfileWinsOverWorkflowRunner(t *testing.T) {
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
	if got := costs[0].AgentProfileID; got != "session-runner" {
		t.Errorf("AgentProfileID = %q, want %q (session identity must win)", got, "session-runner")
	}
}

// TestPromptUsage_SessionAgentProfileLookupFailureLogsAndContinues is a
// regression test for a Review round-1 finding: the session-agent-profile
// fallback lookup's error was silently discarded, unlike every other error
// path in handlePromptUsage (each of which records an explicit drop-reason
// counter). A DB failure on that lookup would have silently reproduced the
// exact "unattributed cost event" symptom this card was filed to fix, with
// nothing in the logs to explain why. The fix logs the failure and — since
// attribution is best-effort — still writes the cost event, unattributed,
// rather than dropping it.
func TestPromptUsage_SessionAgentProfileLookupFailureLogsAndContinues(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	svc, eb := newTestServiceWithBusLogger(t, log)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "codex-runner")
	insertTestTask(t, svc, "task-broken-lookup", "ws-1")
	// Deliberately no setTestTaskAssignee call, so RunnerProjection is ""
	// and handlePromptUsage takes the session-fallback branch under test.
	svc.ExecSQL(t, `DROP TABLE task_sessions`)

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-broken-lookup",
		"session_id": "session-broken-lookup",
		"agent_id":   "codex-acp",
		"model":      "gpt-5.6-terra",
		"usage": map[string]interface{}{
			"input_tokens":  100,
			"output_tokens": 10,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-broken-lookup"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if logs.FilterMessage("session agent profile lookup failed").Len() == 0 {
		t.Fatal("session agent profile lookup failure was not logged")
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d) — lookup failure must not drop the cost event", err, len(costs))
	}
	if got := costs[0].AgentProfileID; got != "" {
		t.Errorf("AgentProfileID = %q, want \"\" (fallback lookup failed, no attribution to fall back to)", got)
	}
}

// TestPromptUsage_TokensOutNullWhenUnmeasured confirms a synthesised-usage
// turn (Estimated=true, no output count observed) never writes tokens_out
// as a plain 0 — that would assert a measurement that was never taken, and
// a downstream per-output-token measure would divide by a fake zero-output
// turn instead of seeing "unknown". This is the shape reproduced against
// the live store: real dollars (a provider-reported cost sample) attached
// to a row with no observed output tokens. See costContractVersion's
// v2→v3 doc comment in prompt_usage_cost.go.
func TestPromptUsage_TokensOutNullWhenUnmeasured(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-unmeasured-out")
	insertTestTask(t, svc, "task-unmeasured-out", "ws-1")
	setTestTaskAssignee(t, svc, "task-unmeasured-out", "worker-unmeasured-out")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-unmeasured-out",
		"session_id": "session-unmeasured-out",
		"agent_id":   "claude-acp",
		"model":      "opus",
		"usage": map[string]interface{}{
			"input_tokens":                    767,
			"estimated":                       true,
			"provider_reported_cost_subcents": 76700,
			"provider_reported_cost_present":  true,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-unmeasured-out"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.TokensOut != nil {
		t.Errorf("TokensOut = %v, want nil (NULL, never a fake 0)", *row.TokensOut)
	}
	if row.CostSubcents != 76700 {
		t.Fatalf("cost_subcents = %d, want 76700 (real money on an unmeasured-output row)", row.CostSubcents)
	}
	if row.CostContractVersion == nil || *row.CostContractVersion != 3 {
		t.Errorf("CostContractVersion = %v, want 3 (v2→v3: tokens_out nullability)", row.CostContractVersion)
	}
	// Asserting the negative directly, per the card's regression bullet: no
	// row may present real money against an unmeasured zero.
	if row.CostSubcents > 0 && row.TokensOut != nil && *row.TokensOut == 0 {
		t.Error("row has cost_subcents > 0 and tokens_out = 0: an unmeasured output is masquerading as a measured zero")
	}
}

// TestPromptUsage_TokensOutKeptWhenMeasuredEvenIfEstimated confirms legacy
// compatibility for events that predate OutputTokensPresent. A nonzero output
// count remains observed even when Estimated is true.
func TestPromptUsage_TokensOutKeptWhenMeasuredEvenIfEstimated(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-estimated-measured-out")
	insertTestTask(t, svc, "task-estimated-measured-out", "ws-1")
	setTestTaskAssignee(t, svc, "task-estimated-measured-out", "worker-estimated-measured-out")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-estimated-measured-out",
		"session_id": "session-estimated-measured-out",
		"agent_id":   "codex-acp",
		"model":      "gpt-5.4-mini",
		"usage": map[string]interface{}{
			"input_tokens":  350,
			"output_tokens": 42,
			"estimated":     true,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-estimated-measured-out"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.TokensOut == nil || *row.TokensOut != 42 {
		t.Errorf("TokensOut = %v, want 42 (a measured value must survive even on an estimated turn)", row.TokensOut)
	}
}

// TestPromptUsage_TokensOutKeepsObservedZero confirms that output-token
// presence is independent from its numeric value. An estimated turn can have
// an observed zero, which must remain a non-nil zero in the ledger.
func TestPromptUsage_TokensOutKeepsObservedZero(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-estimated-zero-out")
	insertTestTask(t, svc, "task-estimated-zero-out", "ws-1")
	setTestTaskAssignee(t, svc, "task-estimated-zero-out", "worker-estimated-zero-out")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-estimated-zero-out",
		"session_id": "session-estimated-zero-out",
		"agent_id":   "codex-acp",
		"model":      "gpt-5.4-mini",
		"usage": map[string]interface{}{
			"input_tokens":          350,
			"output_tokens":         0,
			"output_tokens_present": true,
			"estimated":             true,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-estimated-zero-out"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	row := costs[0]
	if row.TokensOut == nil || *row.TokensOut != 0 {
		t.Errorf("TokensOut = %v, want non-nil 0 (the zero was observed)", row.TokensOut)
	}
}

// TestPromptUsage_TokensOutTrustsExplicitMissingState confirms that output
// presence is independent from the broader estimated flag. New normalized
// events must use their explicit presence state instead of value inference.
func TestPromptUsage_TokensOutTrustsExplicitMissingState(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-missing-out")
	insertTestTask(t, svc, "task-missing-out", "ws-1")
	setTestTaskAssignee(t, svc, "task-missing-out", "worker-missing-out")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-missing-out",
		"session_id": "session-missing-out",
		"agent_id":   "future-acp",
		"model":      "future-model",
		"usage": map[string]interface{}{
			"input_tokens":          350,
			"output_tokens":         0,
			"output_tokens_present": false,
			"estimated":             false,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-missing-out"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d)", err, len(costs))
	}
	if costs[0].TokensOut != nil {
		t.Errorf("TokensOut = %v, want nil for explicitly unobserved output", *costs[0].TokensOut)
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

// TestPromptUsage_DoesNotWriteTaskSessionsRollup pins AC-21: the Office cost
// subscriber inserts its own office_cost_events row but SHALL NOT touch
// task_sessions' tokens_in/tokens_cached_in/tokens_out/cost_subcents rollup
// columns — internal/task/usage's writer is the sole writer of those columns
// (AC-10). The session row is seeded with distinctive, nonzero rollup values
// before the event is published; if handlePromptUsage still incremented (or
// otherwise touched) those columns, they would change.
func TestPromptUsage_DoesNotWriteTaskSessionsRollup(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-rollup")
	insertTestTask(t, svc, "task-rollup", "ws-1")
	setTestTaskAssignee(t, svc, "task-rollup", "worker-rollup")
	insertTestTaskSession(t, svc, "session-rollup", "task-rollup", "worker-rollup")
	svc.ExecSQL(t,
		`UPDATE task_sessions SET tokens_in = ?, tokens_cached_in = ?, tokens_out = ?, cost_subcents = ? WHERE id = ?`,
		111, 22, 33, 44, "session-rollup")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-rollup",
		"session_id": "session-rollup",
		"model":      "sonnet",
		"usage": map[string]interface{}{
			"input_tokens":  100,
			"output_tokens": 30,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-rollup"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil || len(costs) != 1 {
		t.Fatalf("list costs: %v (len=%d) - the handler must still have run and inserted its own row", err, len(costs))
	}

	tokensIn, tokensCachedIn, tokensOut, costSubcents := svc.GetTaskSessionRollupForTest(t, "session-rollup")
	if tokensIn != 111 || tokensCachedIn != 22 || tokensOut != 33 || costSubcents != 44 {
		t.Errorf("task_sessions rollup = (%d,%d,%d,%d), want unchanged (111,22,33,44) - Office must not write these columns",
			tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}

// TestPromptUsage_BudgetCheckUsesSessionFallbackProfile is a regression test
// for the companion budget-check gap: buildCostEvent already resolves the
// session fallback into costEvent.AgentProfileID (see
// TestPromptUsage_FallsBackToSessionAgentProfileWhenUnassigned), but the
// post-event CheckBudget call still passed the empty
// fields.AssigneeAgentProfileID. filterApplicablePolicies matches an
// agent-scoped policy against that argument, so on this fallback path an
// over-budget agent's pause_agent policy never matched and the agent was
// never paused, no matter how much it spent.
func TestPromptUsage_BudgetCheckUsesSessionFallbackProfile(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "codex-runner-budget")
	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "codex-runner-budget",
		LimitSubcents:     10,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	insertTestTask(t, svc, "task-budget-fallback", "ws-1")
	// Deliberately no setTestTaskAssignee call: no workflow runner, so the
	// session fallback path is exercised exactly as in
	// TestPromptUsage_FallsBackToSessionAgentProfileWhenUnassigned.
	insertTestTaskSession(t, svc, "session-budget-fallback", "task-budget-fallback", "codex-runner-budget")

	event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
		"task_id":    "task-budget-fallback",
		"session_id": "session-budget-fallback",
		"model":      "sonnet",
		"usage": map[string]interface{}{
			"input_tokens":                    100,
			"output_tokens":                   200,
			"provider_reported_cost_subcents": 1000,
			"provider_reported_cost_present":  true,
		},
	})
	if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject("session-budget-fallback"), event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	agent, err := svc.GetAgentInstance(ctx, "codex-runner-budget")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Status != models.AgentStatusPaused {
		t.Errorf("agent status = %q, want %q (budget policy must apply to the session-fallback profile)",
			agent.Status, models.AgentStatusPaused)
	}
}
