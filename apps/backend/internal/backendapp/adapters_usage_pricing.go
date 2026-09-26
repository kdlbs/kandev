package backendapp

import (
	"context"

	commoncosts "github.com/kandev/kandev/internal/common/costs"
	"github.com/kandev/kandev/internal/office/costs/modelsdev"
	officeshared "github.com/kandev/kandev/internal/office/shared"
)

// modelsDevLookup is the narrow slice of *modelsdev.Client (internal/office/
// costs/modelsdev) this adapter needs, defined locally so it can be
// exercised with a stub in tests without constructing a real Client.
type modelsDevLookup interface {
	LookupForModelWithVersion(ctx context.Context, modelID string) (officeshared.ModelPricing, string, bool)
}

// usagePricingAdapter adapts *modelsdev.Client's office/shared.ModelPricing-
// returning LookupForModelWithVersion to internal/task/usage's own
// PricingLookup interface (docs/specs/task-cost-ledger/spec.md AC-26): the
// ledger writer's package boundary must never import internal/office/**, so
// this conversion lives in the composition root instead.
type usagePricingAdapter struct {
	lookup modelsDevLookup
}

func (a usagePricingAdapter) LookupForModelWithVersion(ctx context.Context, modelID string) (commoncosts.ModelPricing, string, bool) {
	pricing, version, ok := a.lookup.LookupForModelWithVersion(ctx, modelID)
	return commoncosts.ModelPricing{
		InputPerMillion:       pricing.InputPerMillion,
		CachedReadPerMillion:  pricing.CachedReadPerMillion,
		CachedWritePerMillion: pricing.CachedWritePerMillion,
		OutputPerMillion:      pricing.OutputPerMillion,
	}, version, ok
}

type conversationForkModelInfoLookup interface {
	LookupModelInfo(ctx context.Context, modelID string) (modelsdev.ModelInfo, bool)
}

type conversationForkModelLimitAdapter struct {
	lookup conversationForkModelInfoLookup
}

func (a conversationForkModelLimitAdapter) LookupConversationForkContextLimit(ctx context.Context, modelID string) (int64, bool) {
	if a.lookup == nil {
		return 0, false
	}
	info, ok := a.lookup.LookupModelInfo(ctx, modelID)
	return info.ContextWindow, ok && info.ContextWindow > 0
}
