package backendapp

import (
	"context"

	commoncosts "github.com/kandev/kandev/internal/common/costs"
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
