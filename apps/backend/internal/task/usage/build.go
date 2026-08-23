package usage

import (
	"context"
	"math"
	"time"

	commoncosts "github.com/kandev/kandev/internal/common/costs"
	"github.com/kandev/kandev/internal/task/models"
)

// Cost-source labels recorded on task_usage_events.cost_source (AC-6, AC-7,
// AC-8).
const (
	CostSourceProviderReported = "provider_reported"
	CostSourceModelsDevList    = "models_dev_list"
	CostSourceUnpriced         = "unpriced"
)

// pricingLookupTimeout is AC-8's writer-owned deadline on a pricing call,
// independent of and much shorter than the lookup implementation's own
// HTTP timeout, so a pricing-catalogue outage degrades to an unpriced row
// instead of stalling the single serial worker.
const pricingLookupTimeout = 2 * time.Second

// PricingLookup is the narrow interface the writer declares for pricing
// resolution (AC-26): a single call returning both the rates and the
// catalogue version they came from, so a background catalogue refresh
// between two separate calls can never pair one snapshot's rates with
// another snapshot's version. No internal/office/** type crosses this
// boundary - production wiring adapts whatever concrete lookup it uses
// (e.g. modelsdev.Client) to this shape outside this package.
type PricingLookup interface {
	LookupForModelWithVersion(ctx context.Context, modelID string) (pricing commoncosts.ModelPricing, version string, ok bool)
}

// buildRow constructs a fully-populated task_usage_events row from a
// validated payload (AC-2, AC-5, AC-6..AC-9, AC-23, AC-30, AC-31, AC-35).
// occurredAt is supplied by the caller so it is acquired exactly once, on
// the worker, immediately before the first repository attempt.
func (w *Writer) buildRow(ctx context.Context, p *usageEventPayload, occurredAt time.Time) *models.TaskUsageEvent {
	usage := p.Usage
	if usage == nil {
		usage = &promptUsagePayload{}
	}

	event := &models.TaskUsageEvent{
		UsageEventID:    p.UsageEventID,
		TaskID:          p.TaskID,
		SessionID:       p.SessionID,
		TurnID:          p.TurnID,
		AgentProfileID:  p.AgentProfileID,
		AgentType:       p.AgentType,
		Model:           p.Model,
		Provider:        resolveProvider(p.AgentType, p.AgentID, p.Model),
		Estimated:       usage.Estimated,
		ContractVersion: ContractVersion,
		OccurredAt:      occurredAt,
		CreatedAt:       occurredAt,
	}

	event.TokensIn = usage.InputTokens
	cachedRead := usage.CachedReadTokens
	cachedWrite := usage.CachedWriteTokens
	thought := usage.ThoughtTokens
	event.TokensCachedRead = &cachedRead
	event.TokensCachedWrite = &cachedWrite
	event.TokensThought = &thought
	if usage.OutputTokensPresent {
		out := usage.OutputTokens
		event.TokensOut = &out
	}
	var ok bool
	event.TokensTotal, ok = sumTokenCounts(
		event.TokensIn, cachedRead, cachedWrite, optionalInt64(event.TokensOut), thought,
	)
	if !ok {
		return nil
	}

	w.resolveCost(ctx, event, usage)

	return event
}

// resolveProvider derives event.Provider (AC-31): the first non-empty of
// the CLI-id mapping applied to agent_type, the same mapping applied to
// agent_id, a model-prefix match, then "". Office's resolveProvider applies
// the same two extracted helpers in the same order, so the two writers
// label the same event identically.
func resolveProvider(agentType, agentID, model string) string {
	if provider := commoncosts.ProviderFromCLI(agentType); provider != "" {
		return provider
	}
	if provider := commoncosts.ProviderFromCLI(agentID); provider != "" {
		return provider
	}
	return commoncosts.ProviderForModel(model)
}

// resolveCost implements AC-6/AC-7/AC-8/AC-9's three-way cost-resolution
// ladder: a provider-reported cost is recorded verbatim (including an
// explicit zero); otherwise a models.dev catalogue lookup within
// pricingLookupTimeout computes cost_subcents from a single pricing
// snapshot; otherwise the row is still recorded, unpriced.
func (w *Writer) resolveCost(ctx context.Context, event *models.TaskUsageEvent, usage *promptUsagePayload) {
	if usage.ProviderReportedCostPresent {
		event.CostSubcents = usage.ProviderReportedCostSubcents
		event.CostSource = CostSourceProviderReported
		return
	}
	if w.pricing == nil {
		event.CostSource = CostSourceUnpriced
		return
	}

	lookupCtx, cancel := context.WithTimeout(ctx, pricingLookupTimeout)
	defer cancel()
	pricing, version, ok := w.pricing.LookupForModelWithVersion(lookupCtx, event.Model)
	if !ok {
		event.CostSource = CostSourceUnpriced
		return
	}

	cost, ok := commoncosts.CalculateCostSubcentsChecked(
		event.TokensIn, optionalInt64(event.TokensCachedRead), optionalInt64(event.TokensCachedWrite),
		optionalInt64(event.TokensOut), pricing,
	)
	if !ok {
		event.CostSource = CostSourceUnpriced
		return
	}
	event.CostSubcents = cost
	event.CostSource = CostSourceModelsDevList
	event.RateInputPerMillion = int64Ptr(pricing.InputPerMillion)
	event.RateCachedReadPerMillion = int64Ptr(pricing.CachedReadPerMillion)
	event.RateCachedWritePerMillion = int64Ptr(pricing.CachedWritePerMillion)
	event.RateOutputPerMillion = int64Ptr(pricing.OutputPerMillion)
	event.PricingCatalogVersion = version
}

func optionalInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func sumTokenCounts(values ...int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value < 0 || total > math.MaxInt64-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func int64Ptr(v int64) *int64 {
	return &v
}
