package service

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

// costContractVersion is the in-band activation point for the cache-split /
// cost-provenance / turn-attribution columns (docs/specs/office/costs.md).
// The Rill cost extract has no schema versioning of its own, so a row
// written under a prior contract is distinguished by comparing
// cost_contract_version, not by a date an analyst has to be told out of
// band. Bump only if the contract's meaning changes again.
//
// v1 → v2: on the CostSourceUnpriced path, v1 forced Estimated=true
// regardless of data.Usage.Estimated, conflating "we could not resolve a
// price" with "the token counts themselves were synthesised" — two
// different signals this same contract introduced CostSource specifically
// to keep separate. v2 preserves data.Usage.Estimated verbatim on every
// path (see resolveCostForUsage); cost_source=unpriced alone now carries
// the pricing-failure signal.
const costContractVersion int64 = 2

// costResolution is resolveCostForUsage's output: the priced cost plus
// everything needed to record provenance on the row. Kept separate from
// models.CostEvent so this package's cost-resolution logic doesn't need to
// know about ID/session/task/occurred_at bookkeeping.
type costResolution struct {
	costSubcents   int64
	estimated      bool
	source         models.CostSource
	rates          *shared.ModelPricing // nil unless source == CostSourceModelsDevList
	catalogVersion string
}

// resolveCostForUsage applies the Layer A / Layer B lookup and records
// which layer produced the dollar amount — CostSource, distinct from
// Estimated (a token-synthesis flag, not cost provenance). Layer A wins
// when the adapter forwarded a provider-reported cost sample, including an
// explicit zero
// (claude-acp's usage_update.cost.amount). Layer B (models.dev) is queried
// when a PricingLookup is wired; on miss or when no PricingLookup is
// configured the row is unpriced. Estimated is data.Usage.Estimated
// verbatim on every branch, including unpriced: whether the tokens were
// synthesised and whether a price could be resolved are independent facts,
// and cost_source=unpriced already carries the second one — see
// costContractVersion's v1→v2 doc comment.
func (s *Service) resolveCostForUsage(ctx context.Context, data PromptUsageData) costResolution {
	if data.Usage.ProviderReportedCostPresent || data.Usage.ProviderReportedCostSubcents > 0 {
		return costResolution{
			costSubcents: data.Usage.ProviderReportedCostSubcents,
			estimated:    data.Usage.Estimated,
			source:       models.CostSourceProviderReported,
		}
	}
	if s.pricingLookup == nil || data.Model == "" {
		return costResolution{estimated: data.Usage.Estimated, source: models.CostSourceUnpriced}
	}
	pricing, catalogVersion, ok := s.lookupPricingWithVersion(ctx, data.Model)
	if !ok {
		return costResolution{estimated: data.Usage.Estimated, source: models.CostSourceUnpriced}
	}
	cost := costs.CalculateCostSubcents(
		data.Usage.InputTokens,
		data.Usage.CachedReadTokens,
		data.Usage.CachedWriteTokens,
		data.Usage.OutputTokens,
		costs.ModelPricing{
			InputPerMillion:       pricing.InputPerMillion,
			CachedReadPerMillion:  pricing.CachedReadPerMillion,
			CachedWritePerMillion: pricing.CachedWritePerMillion,
			OutputPerMillion:      pricing.OutputPerMillion,
		},
	)
	return costResolution{
		costSubcents:   cost,
		estimated:      data.Usage.Estimated,
		source:         models.CostSourceModelsDevList,
		rates:          &pricing,
		catalogVersion: catalogVersion,
	}
}

// lookupPricingWithVersion resolves pricing and its catalogue version from
// one atomic snapshot when s.pricingLookup satisfies
// shared.PricingLookupWithVersion, so a concurrent background refresh can
// never pair one catalogue's rates with a different catalogue's version
// identifier on the stored row (docs/specs/office/costs.md). Falls back to two
// separate calls — accepting that narrower race — only for a PricingLookup
// implementation (e.g. a test double) that doesn't support the atomic form;
// CatalogVersion is optional there too, so a non-implementer simply reports
// no version.
func (s *Service) lookupPricingWithVersion(
	ctx context.Context, model string,
) (shared.ModelPricing, string, bool) {
	if withVersion, ok := s.pricingLookup.(shared.PricingLookupWithVersion); ok {
		return withVersion.LookupForModelWithVersion(ctx, model)
	}
	pricing, ok := s.pricingLookup.LookupForModel(ctx, model)
	if !ok {
		return shared.ModelPricing{}, "", false
	}
	var version string
	if versioner, ok := s.pricingLookup.(shared.PricingCatalogVersioner); ok {
		version = versioner.CatalogVersion()
	}
	return pricing, version, true
}

// buildCostEvent assembles the office_cost_events row for a prompt-usage
// update.
//
// The cache split (TokensCachedRead / TokensCachedWrite) is recorded only
// when data.Usage.Estimated is false. codex-acp's ACP bridge (and any
// future adapter with no per-turn usage frame) synthesises InputTokens from
// context-occupancy growth with Estimated=true and never sets the cache
// fields — a NULL split there is honest; 0 would silently claim "no cache
// activity" for an agent that never reported one. TokensCachedIn keeps its
// original read+write sum on every row regardless, so existing consumers
// (the tree-holds rollup, card 2faa29da's task_sessions fix) are
// unaffected.
func buildCostEvent(
	data PromptUsageData, fields *sqlite.TaskExecutionFields, projectID, provider string,
	resolution costResolution,
) *models.CostEvent {
	event := &models.CostEvent{
		SessionID:      data.SessionID,
		TaskID:         data.TaskID,
		AgentProfileID: fields.AssigneeAgentProfileID,
		ProjectID:      projectID,
		Model:          data.Model,
		Provider:       provider,
		TokensIn:       data.Usage.InputTokens,
		TokensCachedIn: data.Usage.CachedReadTokens + data.Usage.CachedWriteTokens,
		TokensOut:      data.Usage.OutputTokens,
		CostSubcents:   resolution.costSubcents,
		Estimated:      resolution.estimated,
		CostSource:     &resolution.source,
		OccurredAt:     time.Now().UTC(),
	}
	contractVersion := costContractVersion
	event.CostContractVersion = &contractVersion

	if !data.Usage.Estimated {
		cachedRead := data.Usage.CachedReadTokens
		cachedWrite := data.Usage.CachedWriteTokens
		event.TokensCachedRead = &cachedRead
		event.TokensCachedWrite = &cachedWrite
	}
	if resolution.rates != nil {
		event.RateInputPerMillion = &resolution.rates.InputPerMillion
		event.RateCachedReadPerMillion = &resolution.rates.CachedReadPerMillion
		event.RateCachedWritePerMillion = &resolution.rates.CachedWritePerMillion
		event.RateOutputPerMillion = &resolution.rates.OutputPerMillion
	}
	if resolution.catalogVersion != "" {
		version := resolution.catalogVersion
		event.PricingCatalogVersion = &version
	}
	if data.TurnID != "" {
		turnID := data.TurnID
		event.TurnID = &turnID
	}
	if data.UsageEventID != "" {
		usageEventID := data.UsageEventID
		event.UsageEventID = &usageEventID
	}
	return event
}
