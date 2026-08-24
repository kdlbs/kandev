package service

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

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
// Estimated (a usage-authority flag, not cost provenance). Layer A wins
// when the adapter forwarded a provider-reported cost sample, including an
// explicit zero
// (claude-acp's usage_update.cost.amount). Layer B (models.dev) is queried
// when a PricingLookup is wired; on miss or when no PricingLookup is
// configured the row is unpriced. Estimated is data.Usage.Estimated
// verbatim on every branch, including unpriced: whether the tokens were
// synthesised and whether a price could be resolved are independent facts,
// and cost_source=unpriced already carries the second one — see
// models.CostContractVersion's contract history.
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
	cost, ok := costs.CalculateCostSubcentsChecked(
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
	if !ok {
		return costResolution{estimated: data.Usage.Estimated, source: models.CostSourceUnpriced}
	}
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
// identifier on the stored row (docs/specs/office/requirements/costs.md). Falls back to two
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
// AgentProfileID prefers sessionAgentProfileID — the stable
// task_sessions.agent_profile_id captured when the session ran. It falls back
// to RunnerProjection's workflow-configured runner only for legacy events that
// have no session identity. This prevents a later workflow reassignment from
// moving an earlier session's usage to a different profile.
//
// The cache split (TokensCachedRead / TokensCachedWrite) is recorded only
// when the usage frame actually carries cache data (either field nonzero).
// The context-occupancy fallback (fallbackUsageForNilTypedUsage in
// adapter_prompt.go) never populates either field, so a NULL split there is
// honest — it is a "no data reported" absence, not a claimed zero. Gating on
// Estimated instead would be wrong: codex-acp's per-request typed frame sets
// Estimated=true (it's scoped to the last model request of the turn, not
// the whole turn — see normalizeCodexPromptUsage in dialect_codex.go) but
// does carry real cache numbers, and discarding them would silently NULL a
// value we actually have. TokensCachedIn keeps its original read+write sum
// on every row regardless — a definite total whether or not the split is
// known — so existing consumers (the tree-holds rollup, card 2faa29da's
// task_sessions fix) are unaffected.
//
// TokensOut uses OutputTokensPresent to keep an observed zero distinct from
// an absent sample. For events written before the presence flag existed, a
// non-estimated count or a nonzero count remains observed. The only production
// shape with no output sample is adapter_prompt.go's estimated
// context-occupancy fallback.
func buildCostEvent(
	data PromptUsageData, fields *sqlite.TaskExecutionFields, projectID, provider string,
	resolution costResolution, sessionAgentProfileID string,
) *models.CostEvent {
	agentProfileID := sessionAgentProfileID
	if agentProfileID == "" {
		agentProfileID = fields.AssigneeAgentProfileID
	}
	event := &models.CostEvent{
		SessionID:      data.SessionID,
		TaskID:         data.TaskID,
		AgentProfileID: agentProfileID,
		ProjectID:      projectID,
		Model:          data.Model,
		Provider:       provider,
		TokensIn:       data.Usage.InputTokens,
		TokensCachedIn: data.Usage.CachedReadTokens + data.Usage.CachedWriteTokens,
		CostSubcents:   resolution.costSubcents,
		Estimated:      resolution.estimated,
		CostSource:     &resolution.source,
		OccurredAt:     time.Now().UTC(),
	}
	contractVersion := models.CostContractVersion
	event.CostContractVersion = &contractVersion

	if outputTokensObserved(data.Usage) {
		outputTokens := data.Usage.OutputTokens
		event.TokensOut = &outputTokens
	}

	if data.Usage.CachedReadTokens != 0 || data.Usage.CachedWriteTokens != 0 {
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

func outputTokensObserved(usage UsageTokens) bool {
	if usage.OutputTokensPresent != nil {
		return *usage.OutputTokensPresent
	}
	return !usage.Estimated || usage.OutputTokens != 0
}
